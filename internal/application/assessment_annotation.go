package application

import (
	"context"

	"bioacoustic-release-hub/internal/domain"
)

func (s *Service) AssessSignal(ctx context.Context, cmd AssessCommand) (Result, error) {
	if err := domain.RequireRole(cmd.Role, domain.RoleAnnotator); err != nil {
		return Result{}, err
	}
	return s.run(ctx, cmd.Metadata, func(_ context.Context, snapshot *domain.Snapshot) (Result, string, error) {
		if err := domain.EnsureMutable(snapshot.Dataset); err != nil {
			return Result{}, "", err
		}
		if _, err := findSample(*snapshot, cmd.SampleID); err != nil {
			return Result{}, "", err
		}
		now := s.now().UTC()
		if previous := latestAssessment(*snapshot, cmd.SampleID); previous != nil && !now.After(previous.AssessedAt) {
			now = previous.AssessedAt.Add(1)
		}
		assessment, err := domain.AssessSignal(domain.AssessSignalInput{ID: s.newID("assessment"), DatasetID: cmd.DatasetID, SampleID: cmd.SampleID, RuleVersion: snapshot.Dataset.QualityRuleVersion, SignalToNoiseDB: cmd.SignalToNoiseDB, ClippingRatio: cmd.ClippingRatio, SilenceRatio: cmd.SilenceRatio, AssessedBy: cmd.Actor}, now)
		if err != nil {
			return Result{}, "", err
		}
		snapshot.Assessments = append(snapshot.Assessments, assessment)
		domain.Advance(&snapshot.Dataset, snapshot.Dataset.Status, now)
		recompute(snapshot)
		return Result{Dataset: snapshot.Dataset, Data: assessment}, "signal.assessed", nil
	})
}

func (s *Service) AssessSignals(ctx context.Context, cmd BatchAssessCommand) (Result, error) {
	if err := domain.RequireRole(cmd.Role, domain.RoleAnnotator); err != nil {
		return Result{}, err
	}
	if len(cmd.Items) == 0 || len(cmd.Items) > 100 {
		return Result{}, domain.NewError(domain.CodeInvalid, "每批 items 数量必须处于 1 到 100")
	}
	return s.run(ctx, cmd.Metadata, func(_ context.Context, snapshot *domain.Snapshot) (Result, string, error) {
		if err := domain.EnsureMutable(snapshot.Dataset); err != nil {
			return Result{}, "", err
		}
		seen := make(map[string]bool, len(cmd.Items))
		for _, item := range cmd.Items {
			if item.SampleID == "" {
				return Result{}, "", domain.NewError(domain.CodeInvalid, "sampleId 不能为空")
			}
			if seen[item.SampleID] {
				return Result{}, "", domain.NewError(domain.CodeInvalid, "批次包含重复样本 %s", item.SampleID)
			}
			seen[item.SampleID] = true
			if _, err := findSample(*snapshot, item.SampleID); err != nil {
				return Result{}, "", err
			}
		}
		now := s.now().UTC()
		assessments := make([]domain.SignalAssessment, 0, len(cmd.Items))
		for _, item := range cmd.Items {
			assessmentTime := now
			if previous := latestAssessment(*snapshot, item.SampleID); previous != nil && !assessmentTime.After(previous.AssessedAt) {
				assessmentTime = previous.AssessedAt.Add(1)
			}
			assessment, err := domain.AssessSignal(domain.AssessSignalInput{
				ID: s.newID("assessment"), DatasetID: cmd.DatasetID, SampleID: item.SampleID,
				RuleVersion: snapshot.Dataset.QualityRuleVersion, SignalToNoiseDB: item.SignalToNoiseDB,
				ClippingRatio: item.ClippingRatio, SilenceRatio: item.SilenceRatio, AssessedBy: cmd.Actor,
			}, assessmentTime)
			if err != nil {
				return Result{}, "", err
			}
			assessments = append(assessments, assessment)
		}
		snapshot.Assessments = append(snapshot.Assessments, assessments...)
		domain.Advance(&snapshot.Dataset, snapshot.Dataset.Status, now)
		recompute(snapshot)
		return Result{Dataset: snapshot.Dataset, Data: map[string]any{"items": assessments}}, "signals.batch_assessed", nil
	})
}

func (s *Service) SubmitAnnotation(ctx context.Context, cmd AnnotateCommand) (Result, error) {
	if err := domain.RequireRole(cmd.Role, domain.RoleAnnotator); err != nil {
		return Result{}, err
	}
	return s.run(ctx, cmd.Metadata, func(_ context.Context, snapshot *domain.Snapshot) (Result, string, error) {
		if err := domain.EnsureMutable(snapshot.Dataset); err != nil {
			return Result{}, "", err
		}
		sample, err := findSample(*snapshot, cmd.SampleID)
		if err != nil {
			return Result{}, "", err
		}
		assessment := latestAssessment(*snapshot, cmd.SampleID)
		if assessment == nil {
			return Result{}, "", domain.NewError(domain.CodePrecondition, "提交标注前必须完成信号检查")
		}
		if assessment.Outcome == domain.SignalBlocked {
			return Result{}, "", domain.NewError(domain.CodePrecondition, "阻断信号检查必须复验通过后才能标注")
		}
		previous := latestAnnotation(*snapshot, cmd.SampleID)
		revisionNo := 1
		var previousID string
		if previous != nil {
			revisionNo = previous.RevisionNo + 1
			previousID = previous.ID
			if cmd.SupersedesID == "" {
				cmd.SupersedesID = previous.ID
			}
		}
		annotation, err := domain.NewAnnotation(domain.AnnotationInput{ID: s.newID("annotation"), DatasetID: cmd.DatasetID, SampleID: cmd.SampleID, RevisionNo: revisionNo, Segments: cmd.Segments, SpeciesCode: cmd.SpeciesCode, CallType: cmd.CallType, Confidence: cmd.Confidence, EvidenceNote: cmd.EvidenceNote, SupersedesID: cmd.SupersedesID, SubmittedBy: cmd.Actor}, sample, s.now())
		if err != nil {
			return Result{}, "", err
		}
		if previous != nil && annotation.SupersedesID != previousID {
			return Result{}, "", domain.NewError(domain.CodeInvalid, "新修订必须 supersede 当前标注 %s", previousID)
		}
		resolvedReturned := false
		for i := range snapshot.Issues {
			issue := &snapshot.Issues[i]
			if issue.SampleID == cmd.SampleID && issue.Status == domain.IssueReturned && issue.AnnotationRevisionID == annotation.SupersedesID {
				if err := domain.ResolveReturnedIssue(issue, annotation); err != nil {
					return Result{}, "", err
				}
				resolvedReturned = true
			}
		}
		rule, _ := domain.RuleFor(snapshot.Dataset.QualityRuleVersion)
		var conflictBase *domain.AnnotationRevision
		if !resolvedReturned {
			conflictBase = previous
		}
		issues := domain.DetectAnnotationIssues(annotation, conflictBase, rule, func() string { return s.newID("issue") })
		snapshot.Annotations = append(snapshot.Annotations, annotation)
		snapshot.Issues = append(snapshot.Issues, issues...)
		domain.Advance(&snapshot.Dataset, snapshot.Dataset.Status, s.now())
		recompute(snapshot)
		return Result{Dataset: snapshot.Dataset, Data: map[string]any{"annotation": annotation, "issues": issues}}, "annotation.submitted", nil
	})
}

package application

import (
	"context"
	"sort"
	"strings"

	"bioacoustic-release-hub/internal/domain"
)

func (s *Service) GetDataset(ctx context.Context, id string) (DatasetView, error) {
	snapshot, err := s.repo.Load(ctx, id)
	if err != nil {
		return DatasetView{}, err
	}
	_, assessed, annotated, open, returned := statusCounts(snapshot)
	return DatasetView{Dataset: snapshot.Dataset, SampleCount: len(snapshot.Samples), AssessmentCount: assessed, AnnotationCount: annotated, OpenIssueCount: open + returned, Credential: snapshot.Credential}, nil
}
func (s *Service) ListSamples(ctx context.Context, id string, page domain.Page) ([]domain.RecordingSample, error) {
	return s.repo.ListSamples(ctx, id, page)
}
func (s *Service) ListIssues(ctx context.Context, id string, page domain.Page) ([]domain.ReviewIssue, error) {
	queue, err := s.QueryIssues(ctx, id, IssueFilter{}, page)
	return queue.Items, err
}
func (s *Service) Timeline(ctx context.Context, id string, page domain.Page) ([]domain.AuditEvent, error) {
	return s.repo.Timeline(ctx, id, page)
}
func (s *Service) GetCredential(ctx context.Context, id string) (domain.ReleaseCredential, error) {
	return s.repo.FindCredential(ctx, id)
}
func (s *Service) VerifyCredential(ctx context.Context, code string) (domain.ReleaseCredential, bool, error) {
	credential, err := s.repo.FindCredential(ctx, code)
	if err != nil {
		return credential, false, err
	}
	return credential, domain.VerifyCredential(credential), nil
}

func normalizeApplicationPage(page domain.Page) (int, int) {
	limit := page.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	offset := page.Offset
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func (s *Service) QueryIssues(ctx context.Context, id string, filter IssueFilter, page domain.Page) (IssueQueue, error) {
	validStatus := map[string]bool{"": true, "open": true, "returned": true, "closed": true}
	validKind := map[string]bool{"": true, "low_confidence": true, "label_conflict": true}
	validSeverity := map[string]bool{"": true, "blocking": true}
	if !validStatus[filter.Status] {
		return IssueQueue{}, domain.NewError(domain.CodeInvalid, "未知 status %q", filter.Status)
	}
	if !validKind[filter.Kind] {
		return IssueQueue{}, domain.NewError(domain.CodeInvalid, "未知 kind %q", filter.Kind)
	}
	if !validSeverity[filter.Severity] {
		return IssueQueue{}, domain.NewError(domain.CodeInvalid, "未知 severity %q", filter.Severity)
	}
	if strings.TrimSpace(filter.SampleID) != filter.SampleID {
		return IssueQueue{}, domain.NewError(domain.CodeInvalid, "sampleId 不能包含首尾空白")
	}
	limit, offset := normalizeApplicationPage(page)
	cacheKey := issueCacheKey{datasetID: id, limit: limit, offset: offset}
	if queue, ok := s.cachedIssueQueue(cacheKey); ok {
		return queue, nil
	}
	result, err := s.repo.QueryIssues(ctx, id, domain.IssueFilter{Status: filter.Status, Kind: filter.Kind, Severity: filter.Severity, SampleID: filter.SampleID}, page)
	if err != nil {
		return IssueQueue{}, err
	}
	queue := IssueQueue{Items: result.Items, Total: result.Total, StatusSummary: result.StatusSummary, KindSummary: result.KindSummary}
	s.rememberIssueQueue(cacheKey, queue)
	return queue, nil
}

func (s *Service) FreezeReadiness(ctx context.Context, id string) (domain.FreezeReadiness, error) {
	snapshot, err := s.loadConsistent(ctx, id)
	if err != nil {
		return domain.FreezeReadiness{}, err
	}
	return domain.DiagnoseFreezeReadiness(snapshot)
}

func (s *Service) AnnotationHistory(ctx context.Context, datasetID, sampleID string, page domain.Page) (AnnotationHistory, error) {
	snapshot, err := s.loadConsistent(ctx, datasetID)
	if err != nil {
		return AnnotationHistory{}, err
	}
	sample, err := findSample(snapshot, sampleID)
	if err != nil {
		return AnnotationHistory{}, err
	}
	revisions := make([]domain.AnnotationRevision, 0)
	for _, revision := range snapshot.Annotations {
		if revision.SampleID == sampleID {
			revisions = append(revisions, revision)
		}
	}
	if err := domain.ValidateAnnotationHistory(sample, revisions, snapshot.Issues); err != nil {
		return AnnotationHistory{}, err
	}
	sort.Slice(revisions, func(i, j int) bool { return revisions[i].RevisionNo < revisions[j].RevisionNo })
	total := len(revisions)
	limit, offset := normalizeApplicationPage(page)
	selected := revisions
	if offset >= total {
		selected = []domain.AnnotationRevision{}
	} else {
		end := offset + limit
		if end > total {
			end = total
		}
		selected = selected[offset:end]
	}
	items := make([]AnnotationHistoryItem, 0, len(selected))
	for _, revision := range selected {
		item := AnnotationHistoryItem{AnnotationRevision: revision, Current: revision.RevisionNo == total, Issues: []AnnotationIssueView{}}
		for _, issue := range snapshot.Issues {
			if issue.AnnotationRevisionID == revision.ID {
				item.Issues = append(item.Issues, AnnotationIssueView{IssueID: issue.ID, Status: issue.Status, ExpertDecision: issue.ExpertDecision, ResolutionRevisionID: issue.ResolutionRevisionID})
			}
		}
		sort.Slice(item.Issues, func(i, j int) bool { return item.Issues[i].IssueID < item.Issues[j].IssueID })
		items = append(items, item)
	}
	return AnnotationHistory{DatasetID: datasetID, Sample: sample, Items: items, Total: total}, nil
}

func (s *Service) FrozenItems(ctx context.Context, id string, page domain.Page) (FrozenItemsView, error) {
	snapshot, err := s.loadConsistent(ctx, id)
	if err != nil {
		return FrozenItemsView{}, err
	}
	d := snapshot.Dataset
	if d.Status != domain.StatusFrozen && d.Status != domain.StatusReleased {
		return FrozenItemsView{}, domain.WithState(domain.NewError(domain.CodePrecondition, "候选尚未冻结"), d)
	}
	items := append([]domain.FrozenItem(nil), snapshot.FrozenItems...)
	sort.Slice(items, func(i, j int) bool { return items[i].SampleID < items[j].SampleID })
	if err := validateFrozenReferences(snapshot, items); err != nil {
		return FrozenItemsView{}, err
	}
	digest, err := domain.FrozenManifestDigest(items)
	if err != nil {
		return FrozenItemsView{}, err
	}
	digestValid := digest == d.ManifestDigest
	if d.Status == domain.StatusReleased {
		if snapshot.Credential == nil || snapshot.Credential.FrozenRevision != d.FrozenRevision || snapshot.Credential.ManifestDigest != d.ManifestDigest || snapshot.Credential.SampleCount != len(items) || !digestValid {
			return FrozenItemsView{}, domain.NewError(domain.CodeIntegrity, "放行凭据与冻结清单不一致")
		}
	}
	total := len(items)
	limit, offset := normalizeApplicationPage(page)
	if offset >= total {
		items = []domain.FrozenItem{}
	} else {
		end := offset + limit
		if end > total {
			end = total
		}
		items = items[offset:end]
	}
	return FrozenItemsView{DatasetID: id, Status: d.Status, Items: items, Total: total, FrozenRevision: d.FrozenRevision, ManifestDigest: d.ManifestDigest, DigestValid: digestValid}, nil
}

func validateFrozenReferences(snapshot domain.Snapshot, items []domain.FrozenItem) error {
	samples := make(map[string]domain.RecordingSample, len(snapshot.Samples))
	assessments := make(map[string]domain.SignalAssessment, len(snapshot.Assessments))
	annotations := make(map[string]domain.AnnotationRevision, len(snapshot.Annotations))
	for _, item := range snapshot.Samples {
		samples[item.ID] = item
	}
	for _, item := range snapshot.Assessments {
		assessments[item.ID] = item
	}
	for _, item := range snapshot.Annotations {
		annotations[item.ID] = item
	}
	for _, item := range items {
		sample, sampleOK := samples[item.SampleID]
		assessment, assessmentOK := assessments[item.AssessmentID]
		annotation, annotationOK := annotations[item.AnnotationRevisionID]
		if !sampleOK || sample.DatasetID != snapshot.Dataset.ID || sample.SHA256 != item.SHA256 || !assessmentOK || assessment.SampleID != item.SampleID || assessment.DatasetID != snapshot.Dataset.ID || !annotationOK || annotation.SampleID != item.SampleID || annotation.DatasetID != snapshot.Dataset.ID {
			return domain.NewError(domain.CodeIntegrity, "冻结项 %s 的实体引用不一致", item.SampleID)
		}
	}
	return nil
}

package domain

import "sort"

type ReadinessCounts struct {
	SampleTotal          int            `json:"sampleTotal"`
	QualifiedAssessments int            `json:"qualifiedAssessments"`
	CurrentAnnotations   int            `json:"currentAnnotations"`
	BlockedAssessments   int            `json:"blockedAssessments"`
	IssuesByStatus       map[string]int `json:"issuesByStatus"`
}

type ReadinessBlocker struct {
	SampleID     string      `json:"sampleId,omitempty"`
	Kind         string      `json:"kind"`
	AssessmentID string      `json:"assessmentId,omitempty"`
	IssueID      string      `json:"issueId,omitempty"`
	IssueStatus  IssueStatus `json:"issueStatus,omitempty"`
}

type FreezeReadiness struct {
	DatasetID      string             `json:"datasetId"`
	Revision       int64              `json:"revision"`
	Status         DatasetStatus      `json:"status"`
	Ready          bool               `json:"ready"`
	FreezeRequired bool               `json:"freezeRequired"`
	AlreadyFrozen  bool               `json:"alreadyFrozen"`
	Counts         ReadinessCounts    `json:"counts"`
	Blockers       []ReadinessBlocker `json:"blockers"`
	SampleCount    int                `json:"sampleCount,omitempty"`
	ManifestDigest string             `json:"manifestDigest,omitempty"`
	FrozenRevision int64              `json:"frozenRevision,omitempty"`
}

func DiagnoseFreezeReadiness(snapshot Snapshot) (FreezeReadiness, error) {
	d := snapshot.Dataset
	result := FreezeReadiness{
		DatasetID: d.ID, Revision: d.Revision, Status: d.Status, FreezeRequired: true,
		Counts:   ReadinessCounts{SampleTotal: len(snapshot.Samples), IssuesByStatus: map[string]int{"open": 0, "returned": 0, "closed": 0}},
		Blockers: []ReadinessBlocker{},
	}
	alreadyFrozen := d.Status == StatusFrozen || d.Status == StatusReleased
	latestAssessment := make(map[string]SignalAssessment)
	for _, assessment := range snapshot.Assessments {
		current, ok := latestAssessment[assessment.SampleID]
		if !ok || assessment.AssessedAt.After(current.AssessedAt) || (assessment.AssessedAt.Equal(current.AssessedAt) && assessment.ID > current.ID) {
			latestAssessment[assessment.SampleID] = assessment
		}
	}
	latestAnnotation := make(map[string]AnnotationRevision)
	for _, annotation := range snapshot.Annotations {
		current, ok := latestAnnotation[annotation.SampleID]
		if !ok || annotation.RevisionNo > current.RevisionNo {
			latestAnnotation[annotation.SampleID] = annotation
		}
	}
	issuesBySample := make(map[string][]ReviewIssue)
	for _, issue := range snapshot.Issues {
		result.Counts.IssuesByStatus[string(issue.Status)]++
		if issue.Status != IssueClosed {
			issuesBySample[issue.SampleID] = append(issuesBySample[issue.SampleID], issue)
		}
	}
	samples := append([]RecordingSample(nil), snapshot.Samples...)
	sort.Slice(samples, func(i, j int) bool { return samples[i].ID < samples[j].ID })
	if len(samples) == 0 && !alreadyFrozen {
		result.Blockers = append(result.Blockers, ReadinessBlocker{Kind: "empty_dataset"})
	}
	for _, sample := range samples {
		assessment, assessed := latestAssessment[sample.ID]
		switch {
		case !assessed:
			if !alreadyFrozen {
				result.Blockers = append(result.Blockers, ReadinessBlocker{SampleID: sample.ID, Kind: "missing_assessment"})
			}
		case assessment.Outcome == SignalBlocked:
			result.Counts.BlockedAssessments++
			if !alreadyFrozen {
				result.Blockers = append(result.Blockers, ReadinessBlocker{SampleID: sample.ID, Kind: "blocked_assessment", AssessmentID: assessment.ID})
			}
		default:
			result.Counts.QualifiedAssessments++
		}
		if _, ok := latestAnnotation[sample.ID]; !ok {
			if !alreadyFrozen {
				result.Blockers = append(result.Blockers, ReadinessBlocker{SampleID: sample.ID, Kind: "missing_annotation"})
			}
		} else {
			result.Counts.CurrentAnnotations++
		}
		issues := issuesBySample[sample.ID]
		sort.Slice(issues, func(i, j int) bool { return issues[i].ID < issues[j].ID })
		if !alreadyFrozen {
			for _, issue := range issues {
				result.Blockers = append(result.Blockers, ReadinessBlocker{SampleID: sample.ID, Kind: "unclosed_issue", IssueID: issue.ID, IssueStatus: issue.Status})
			}
		}
	}
	if alreadyFrozen {
		result.Ready = true
		result.FreezeRequired = false
		result.AlreadyFrozen = true
		result.FrozenRevision = d.FrozenRevision
		result.ManifestDigest = d.ManifestDigest
		result.SampleCount = len(snapshot.FrozenItems)
		return result, nil
	}
	if len(result.Blockers) != 0 {
		return result, nil
	}
	items, digest, err := BuildManifest(snapshot.Samples, snapshot.Assessments, snapshot.Annotations)
	if err != nil {
		return FreezeReadiness{}, err
	}
	result.Ready = true
	result.SampleCount = len(items)
	result.ManifestDigest = digest
	return result, nil
}

func ValidateAnnotationHistory(sample RecordingSample, revisions []AnnotationRevision, issues []ReviewIssue) error {
	ordered := append([]AnnotationRevision(nil), revisions...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].RevisionNo < ordered[j].RevisionNo })
	byID := make(map[string]AnnotationRevision, len(ordered))
	for i, revision := range ordered {
		if revision.DatasetID != sample.DatasetID || revision.SampleID != sample.ID || revision.RevisionNo != i+1 {
			return NewError(CodeIntegrity, "样本 %s 的标注修订号断链", sample.ID)
		}
		if i == 0 && revision.SupersedesID != "" {
			return NewError(CodeIntegrity, "样本 %s 的首个标注修订不能 supersede 其他修订", sample.ID)
		}
		if i > 0 && revision.SupersedesID != ordered[i-1].ID {
			return NewError(CodeIntegrity, "样本 %s 的标注修订 %s 未直接替代前一版本", sample.ID, revision.ID)
		}
		byID[revision.ID] = revision
	}
	for _, issue := range issues {
		if issue.SampleID != sample.ID {
			continue
		}
		if issue.DatasetID != sample.DatasetID {
			return NewError(CodeIntegrity, "审查问题 %s 与样本不属于同一数据集", issue.ID)
		}
		referenced, ok := byID[issue.AnnotationRevisionID]
		if !ok || referenced.SampleID != issue.SampleID {
			return NewError(CodeIntegrity, "审查问题 %s 引用了无效标注修订", issue.ID)
		}
		if issue.ResolutionRevisionID != "" {
			resolution, ok := byID[issue.ResolutionRevisionID]
			if !ok || resolution.SampleID != issue.SampleID {
				return NewError(CodeIntegrity, "审查问题 %s 的整改修订引用无效", issue.ID)
			}
		}
	}
	return nil
}

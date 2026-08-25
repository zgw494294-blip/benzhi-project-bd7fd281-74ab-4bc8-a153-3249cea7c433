package domain

import (
	"strings"
	"time"
)

type AnnotationInput struct {
	ID           string
	DatasetID    string
	SampleID     string
	RevisionNo   int
	Segments     []Segment
	SpeciesCode  string
	CallType     string
	Confidence   float64
	EvidenceNote string
	SupersedesID string
	SubmittedBy  string
}

func NewAnnotation(in AnnotationInput, sample RecordingSample, now time.Time) (AnnotationRevision, error) {
	if in.SampleID != sample.ID || in.DatasetID != sample.DatasetID {
		return AnnotationRevision{}, NewError(CodeInvalid, "标注与样本不属于同一数据集")
	}
	if in.RevisionNo < 1 || len(in.Segments) == 0 {
		return AnnotationRevision{}, NewError(CodeInvalid, "标注修订号和片段不能为空")
	}
	if strings.TrimSpace(in.SpeciesCode) == "" || strings.TrimSpace(in.CallType) == "" || strings.TrimSpace(in.EvidenceNote) == "" {
		return AnnotationRevision{}, NewError(CodeInvalid, "speciesCode、callType 和 evidenceNote 不能为空")
	}
	if in.Confidence < 0 || in.Confidence > 1 {
		return AnnotationRevision{}, NewError(CodeInvalid, "confidence 必须处于 0 到 1")
	}
	segments := append([]Segment(nil), in.Segments...)
	for i, segment := range segments {
		if segment.StartMs < 0 || segment.EndMs <= segment.StartMs || segment.EndMs > sample.DurationMs {
			return AnnotationRevision{}, NewError(CodeInvalid, "片段 %d 超出录音时间范围", i)
		}
		for j := 0; j < i; j++ {
			if segments[j].StartMs < segment.EndMs && segment.StartMs < segments[j].EndMs {
				return AnnotationRevision{}, NewError(CodeConflict, "标注片段 %d 与 %d 重叠", j, i)
			}
		}
	}
	return AnnotationRevision{ID: in.ID, DatasetID: in.DatasetID, SampleID: in.SampleID, RevisionNo: in.RevisionNo, Segments: segments, SpeciesCode: strings.TrimSpace(in.SpeciesCode), CallType: strings.TrimSpace(in.CallType), Confidence: in.Confidence, EvidenceNote: strings.TrimSpace(in.EvidenceNote), SupersedesID: in.SupersedesID, SubmittedBy: in.SubmittedBy, SubmittedAt: now.UTC()}, nil
}

func DetectAnnotationIssues(annotation AnnotationRevision, previous *AnnotationRevision, rule QualityRule, id func() string) []ReviewIssue {
	issues := make([]ReviewIssue, 0, 2)
	if annotation.Confidence < rule.MinimumConfidence {
		issues = append(issues, ReviewIssue{ID: id(), DatasetID: annotation.DatasetID, SampleID: annotation.SampleID, AnnotationRevisionID: annotation.ID, Kind: "low_confidence", Severity: "blocking", Status: IssueOpen})
	}
	if previous != nil && (previous.SpeciesCode != annotation.SpeciesCode || previous.CallType != annotation.CallType) {
		issues = append(issues, ReviewIssue{ID: id(), DatasetID: annotation.DatasetID, SampleID: annotation.SampleID, AnnotationRevisionID: annotation.ID, Kind: "label_conflict", Severity: "blocking", Status: IssueOpen})
	}
	return issues
}

func DecideIssue(issue *ReviewIssue, decision IssueDecision, note, expert string, now time.Time) error {
	if issue.Status != IssueOpen {
		return NewError(CodeConflict, "仅 open 问题可以裁决")
	}
	if decision != DecisionConfirm && decision != DecisionOverride && decision != DecisionReturn {
		return NewError(CodeInvalid, "未知专家决定 %q", decision)
	}
	if strings.TrimSpace(note) == "" {
		return NewError(CodeInvalid, "专家裁决说明不能为空")
	}
	issue.ExpertDecision = decision
	issue.DecisionNote = strings.TrimSpace(note)
	issue.ReviewedBy = expert
	t := now.UTC()
	issue.ReviewedAt = &t
	if decision == DecisionReturn {
		issue.Status = IssueReturned
	} else {
		issue.Status = IssueClosed
	}
	return nil
}

func ResolveReturnedIssue(issue *ReviewIssue, resolution AnnotationRevision) error {
	if issue.Status != IssueReturned {
		return NewError(CodeConflict, "仅 returned 问题可以整改关闭")
	}
	if resolution.SampleID != issue.SampleID || resolution.ID == issue.AnnotationRevisionID || resolution.SupersedesID != issue.AnnotationRevisionID {
		return NewError(CodeInvalid, "整改必须引用新的标注修订并 supersede 原问题修订")
	}
	issue.ResolutionRevisionID = resolution.ID
	issue.Status = IssueClosed
	return nil
}

package domain

import (
	"strings"
	"time"
)

type CreateDatasetInput struct {
	ID                 string
	Title              string
	ResearchGoal       string
	TargetTaxa         []string
	RecordingRegion    string
	QualityRuleVersion string
	CreatedBy          string
}

func NewDataset(in CreateDatasetInput, now time.Time) (Dataset, error) {
	if strings.TrimSpace(in.ID) == "" || strings.TrimSpace(in.Title) == "" {
		return Dataset{}, NewError(CodeInvalid, "数据集 id 和 title 不能为空")
	}
	if strings.TrimSpace(in.ResearchGoal) == "" || strings.TrimSpace(in.RecordingRegion) == "" {
		return Dataset{}, NewError(CodeInvalid, "researchGoal 和 recordingRegion 不能为空")
	}
	if len(in.TargetTaxa) == 0 {
		return Dataset{}, NewError(CodeInvalid, "targetTaxa 至少包含一个目标类群")
	}
	if in.QualityRuleVersion != "bio-v1" {
		return Dataset{}, NewError(CodeInvalid, "不支持质量规则版本 %q", in.QualityRuleVersion)
	}
	return Dataset{
		ID: in.ID, Title: strings.TrimSpace(in.Title), ResearchGoal: strings.TrimSpace(in.ResearchGoal),
		TargetTaxa: append([]string(nil), in.TargetTaxa...), RecordingRegion: strings.TrimSpace(in.RecordingRegion),
		QualityRuleVersion: in.QualityRuleVersion, Status: StatusDraft, Revision: 1,
		CreatedBy: in.CreatedBy, CreatedAt: now.UTC(), UpdatedAt: now.UTC(),
	}, nil
}

func CheckRevision(d Dataset, expected int64) error {
	if expected <= 0 {
		return WithState(NewError(CodeInvalid, "revision 必须为正整数"), d)
	}
	if d.Revision != expected {
		return WithState(NewError(CodeStaleRevision, "revision 已过期：期望 %d，当前 %d", expected, d.Revision), d)
	}
	return nil
}

func EnsureMutable(d Dataset) error {
	if d.Status == StatusFrozen || d.Status == StatusReleased {
		return WithState(NewError(CodeFrozen, "候选集冻结后禁止修改纳入内容"), d)
	}
	return nil
}

func Advance(d *Dataset, status DatasetStatus, now time.Time) {
	d.Revision++
	d.Status = status
	d.UpdatedAt = now.UTC()
}

func RecomputeStatus(d *Dataset, sampleCount, assessedCount, annotatedCount, openIssues, returnedIssues int) {
	if d.Status == StatusFrozen || d.Status == StatusReleased {
		return
	}
	switch {
	case sampleCount == 0:
		d.Status = StatusDraft
	case assessedCount < sampleCount:
		d.Status = StatusDraft
	case annotatedCount < sampleCount:
		d.Status = StatusScreened
	case openIssues > 0 || returnedIssues > 0:
		d.Status = StatusUnderReview
	default:
		d.Status = StatusReady
	}
}

func Freeze(d *Dataset, digest string, now time.Time) error {
	if d.Status != StatusReady {
		return WithState(NewError(CodePrecondition, "仅 ready 状态可以冻结"), *d)
	}
	Advance(d, StatusFrozen, now)
	d.FrozenRevision = d.Revision
	d.ManifestDigest = digest
	return nil
}

func RevokeFreeze(d *Dataset, now time.Time) error {
	if d.Status != StatusFrozen {
		return WithState(NewError(CodePrecondition, "仅 frozen 状态可以撤销候选"), *d)
	}
	d.ManifestDigest = ""
	d.FrozenRevision = 0
	Advance(d, StatusReady, now)
	return nil
}

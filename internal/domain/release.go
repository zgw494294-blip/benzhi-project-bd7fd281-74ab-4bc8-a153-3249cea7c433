package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
)

func BuildManifest(samples []RecordingSample, assessments []SignalAssessment, annotations []AnnotationRevision) ([]FrozenItem, string, error) {
	assessmentBySample := make(map[string]SignalAssessment)
	for _, a := range assessments {
		assessmentBySample[a.SampleID] = a
	}
	annotationBySample := make(map[string]AnnotationRevision)
	for _, a := range annotations {
		if current, ok := annotationBySample[a.SampleID]; !ok || a.RevisionNo > current.RevisionNo {
			annotationBySample[a.SampleID] = a
		}
	}
	items := make([]FrozenItem, 0, len(samples))
	for _, sample := range samples {
		assessment, ok := assessmentBySample[sample.ID]
		if !ok {
			return nil, "", NewError(CodePrecondition, "样本 %s 尚未完成信号检查", sample.ID)
		}
		if assessment.Outcome == SignalBlocked {
			return nil, "", NewError(CodePrecondition, "样本 %s 信号检查被阻断", sample.ID)
		}
		annotation, ok := annotationBySample[sample.ID]
		if !ok {
			return nil, "", NewError(CodePrecondition, "样本 %s 尚未标注", sample.ID)
		}
		items = append(items, FrozenItem{SampleID: sample.ID, SHA256: sample.SHA256, AssessmentID: assessment.ID, AnnotationRevisionID: annotation.ID})
	}
	if len(items) == 0 {
		return nil, "", NewError(CodePrecondition, "空数据集不能冻结")
	}
	sort.Slice(items, func(i, j int) bool { return items[i].SampleID < items[j].SampleID })
	digest, err := FrozenManifestDigest(items)
	if err != nil {
		return nil, "", err
	}
	return items, digest, nil
}

// FrozenManifestDigest 使用与冻结命令相同的规范顺序复核持久化候选清单。
func FrozenManifestDigest(items []FrozenItem) (string, error) {
	ordered := append([]FrozenItem(nil), items...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].SampleID < ordered[j].SampleID })
	h := sha256.New()
	for i, item := range ordered {
		if strings.TrimSpace(item.SampleID) == "" || strings.TrimSpace(item.SHA256) == "" || strings.TrimSpace(item.AssessmentID) == "" || strings.TrimSpace(item.AnnotationRevisionID) == "" {
			return "", NewError(CodeIntegrity, "冻结项字段不完整")
		}
		if i > 0 && ordered[i-1].SampleID == item.SampleID {
			return "", NewError(CodeIntegrity, "冻结清单包含重复样本 %s", item.SampleID)
		}
		fmt.Fprintf(h, "%s\x00%s\x00%s\x00%s\n", item.SampleID, item.SHA256, item.AssessmentID, item.AnnotationRevisionID)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func NewCredential(id string, d Dataset, sampleCount int, approvedBy string, now time.Time) (ReleaseCredential, error) {
	if d.Status != StatusFrozen || d.ManifestDigest == "" {
		return ReleaseCredential{}, WithState(NewError(CodePrecondition, "仅 frozen 候选可以批准"), d)
	}
	if strings.TrimSpace(approvedBy) == "" {
		return ReleaseCredential{}, NewError(CodeInvalid, "approvedBy 不能为空")
	}
	c := ReleaseCredential{ID: id, DatasetID: d.ID, FrozenRevision: d.FrozenRevision, ManifestDigest: d.ManifestDigest, SampleCount: sampleCount, QualityRuleVersion: d.QualityRuleVersion, ApprovedBy: approvedBy, ApprovedAt: now.UTC()}
	c.VerificationCode = CredentialCode(c)
	return c, nil
}

func CredentialCode(c ReleaseCredential) string {
	plain := fmt.Sprintf("%s|%d|%s|%d|%s|%s|%s", c.DatasetID, c.FrozenRevision, c.ManifestDigest, c.SampleCount, c.QualityRuleVersion, c.ApprovedBy, c.ApprovedAt.UTC().Format(time.RFC3339Nano))
	sum := sha256.Sum256([]byte(plain))
	return strings.ToUpper(hex.EncodeToString(sum[:16]))
}

func VerifyCredential(c ReleaseCredential) bool {
	return c.VerificationCode != "" && c.VerificationCode == CredentialCode(c)
}

package domain

import (
	"fmt"
	"math"
	"strings"
	"time"
)

type QualityRule struct {
	Version           string
	MinimumSNRDB      float64
	WarningSNRDB      float64
	MaximumClipping   float64
	WarningClipping   float64
	MaximumSilence    float64
	WarningSilence    float64
	MinimumConfidence float64
}

func RuleFor(version string) (QualityRule, error) {
	if version != "bio-v1" {
		return QualityRule{}, NewError(CodeInvalid, "未知质量规则版本 %q", version)
	}
	return QualityRule{Version: version, MinimumSNRDB: 8, WarningSNRDB: 14, MaximumClipping: 0.08, WarningClipping: 0.03, MaximumSilence: 0.9, WarningSilence: 0.7, MinimumConfidence: 0.75}, nil
}

type AssessSignalInput struct {
	ID              string
	DatasetID       string
	SampleID        string
	RuleVersion     string
	SignalToNoiseDB float64
	ClippingRatio   float64
	SilenceRatio    float64
	AssessedBy      string
}

func AssessSignal(in AssessSignalInput, now time.Time) (SignalAssessment, error) {
	rule, err := RuleFor(in.RuleVersion)
	if err != nil {
		return SignalAssessment{}, err
	}
	if math.IsNaN(in.SignalToNoiseDB) || math.IsInf(in.SignalToNoiseDB, 0) || in.SignalToNoiseDB < -20 || in.SignalToNoiseDB > 120 {
		return SignalAssessment{}, NewError(CodeInvalid, "signalToNoiseDb 超出允许范围")
	}
	if math.IsNaN(in.ClippingRatio) || math.IsInf(in.ClippingRatio, 0) || math.IsNaN(in.SilenceRatio) || math.IsInf(in.SilenceRatio, 0) || in.ClippingRatio < 0 || in.ClippingRatio > 1 || in.SilenceRatio < 0 || in.SilenceRatio > 1 {
		return SignalAssessment{}, NewError(CodeInvalid, "比例指标必须处于 0 到 1")
	}
	outcome := SignalPass
	reasons := make([]string, 0, 3)
	blocked := false
	if in.SignalToNoiseDB < rule.MinimumSNRDB {
		blocked = true
		reasons = append(reasons, fmt.Sprintf("信噪比 %.2f dB 低于阻断阈值 %.2f", in.SignalToNoiseDB, rule.MinimumSNRDB))
	} else if in.SignalToNoiseDB < rule.WarningSNRDB {
		outcome = SignalWarning
		reasons = append(reasons, "信噪比接近下限")
	}
	if in.ClippingRatio > rule.MaximumClipping {
		blocked = true
		reasons = append(reasons, "削波率超过阻断阈值")
	} else if in.ClippingRatio > rule.WarningClipping && outcome == SignalPass {
		outcome = SignalWarning
		reasons = append(reasons, "削波率达到警告阈值")
	}
	if in.SilenceRatio > rule.MaximumSilence {
		blocked = true
		reasons = append(reasons, "静音比例超过阻断阈值")
	} else if in.SilenceRatio > rule.WarningSilence && outcome == SignalPass {
		outcome = SignalWarning
		reasons = append(reasons, "静音比例达到警告阈值")
	}
	if blocked {
		outcome = SignalBlocked
	}
	if len(reasons) == 0 {
		reasons = append(reasons, "全部信号指标满足规则")
	}
	return SignalAssessment{ID: in.ID, DatasetID: in.DatasetID, SampleID: in.SampleID, RuleVersion: rule.Version, SignalToNoiseDB: in.SignalToNoiseDB, ClippingRatio: in.ClippingRatio, SilenceRatio: in.SilenceRatio, Outcome: outcome, Reasons: reasons, AssessedBy: strings.TrimSpace(in.AssessedBy), AssessedAt: now.UTC()}, nil
}

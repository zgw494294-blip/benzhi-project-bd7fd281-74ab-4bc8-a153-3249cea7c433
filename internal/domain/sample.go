package domain

import (
	"regexp"
	"strings"
	"time"
)

var sha256Pattern = regexp.MustCompile(`^[a-fA-F0-9]{64}$`)

type SampleInput struct {
	ID            string
	DatasetID     string
	SourceRef     string
	CapturedAt    time.Time
	LatitudeBand  string
	LongitudeBand string
	SampleRateHz  int
	Channels      int
	DurationMs    int64
	SHA256        string
	RegisteredBy  string
}

func NewSample(in SampleInput, dataset Dataset, now time.Time) (RecordingSample, error) {
	if in.DatasetID != dataset.ID {
		return RecordingSample{}, NewError(CodeInvalid, "样本 datasetId 不匹配")
	}
	if strings.TrimSpace(in.ID) == "" || strings.TrimSpace(in.SourceRef) == "" {
		return RecordingSample{}, NewError(CodeInvalid, "样本 id 和 sourceRef 不能为空")
	}
	if in.CapturedAt.IsZero() || in.CapturedAt.After(now.Add(5*time.Minute)) || in.CapturedAt.Before(time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)) {
		return RecordingSample{}, NewError(CodeInvalid, "capturedAt 超出允许范围")
	}
	if in.SampleRateHz < 8000 || in.SampleRateHz > 384000 {
		return RecordingSample{}, NewError(CodeInvalid, "sampleRateHz 必须处于 8000 到 384000")
	}
	if in.Channels < 1 || in.Channels > 8 {
		return RecordingSample{}, NewError(CodeInvalid, "channels 必须处于 1 到 8")
	}
	if in.DurationMs < 100 || in.DurationMs > 24*60*60*1000 {
		return RecordingSample{}, NewError(CodeInvalid, "durationMs 必须处于 100 到 86400000")
	}
	if !sha256Pattern.MatchString(in.SHA256) {
		return RecordingSample{}, NewError(CodeInvalid, "sha256 必须是 64 位十六进制内容摘要")
	}
	if strings.TrimSpace(in.LatitudeBand) == "" || strings.TrimSpace(in.LongitudeBand) == "" {
		return RecordingSample{}, NewError(CodeInvalid, "位置分带不能为空")
	}
	return RecordingSample{ID: in.ID, DatasetID: in.DatasetID, SourceRef: strings.TrimSpace(in.SourceRef), CapturedAt: in.CapturedAt.UTC(), LatitudeBand: in.LatitudeBand, LongitudeBand: in.LongitudeBand, SampleRateHz: in.SampleRateHz, Channels: in.Channels, DurationMs: in.DurationMs, SHA256: strings.ToLower(in.SHA256), RegisteredBy: in.RegisteredBy, RegisteredAt: now.UTC()}, nil
}

package application

import (
	"context"
	"encoding/json"

	"bioacoustic-release-hub/internal/domain"
)

func (s *Service) CreateDataset(ctx context.Context, cmd CreateCommand) (Result, error) {
	if err := domain.RequireRole(cmd.Role, domain.RoleManager); err != nil {
		return Result{}, err
	}
	if cmd.RequestID == "" || cmd.Actor == "" {
		return Result{}, domain.NewError(domain.CodeInvalid, "X-Request-Id 和 X-Actor 不能为空")
	}
	if cmd.ID == "" {
		cmd.ID = s.newID("ds")
	}
	value, err := s.mailboxes.forDataset(cmd.ID).submit(ctx, func(inner context.Context) (any, error) {
		if raw, ok, err := s.repo.IdempotentResult(inner, cmd.ID, cmd.RequestID); err != nil {
			return nil, err
		} else if ok {
			var r Result
			if err := json.Unmarshal(raw, &r); err != nil {
				return nil, err
			}
			r.Idempotent = true
			return r, nil
		}
		now := s.now().UTC()
		dataset, err := domain.NewDataset(domain.CreateDatasetInput{ID: cmd.ID, Title: cmd.Title, ResearchGoal: cmd.ResearchGoal, TargetTaxa: cmd.TargetTaxa, RecordingRegion: cmd.RecordingRegion, QualityRuleVersion: cmd.QualityRuleVersion, CreatedBy: cmd.Actor}, now)
		if err != nil {
			return nil, err
		}
		result := Result{Dataset: dataset}
		raw, _ := json.Marshal(result)
		event := domain.AuditEvent{DatasetID: dataset.ID, Revision: dataset.Revision, EventType: "dataset.created", Actor: cmd.Actor, RequestID: cmd.RequestID, Details: map[string]any{"title": dataset.Title}, OccurredAt: now}
		if err := s.repo.Create(inner, domain.Snapshot{Dataset: dataset}, event, cmd.RequestID, raw); err != nil {
			return nil, err
		}
		return result, nil
	})
	if err != nil {
		return Result{}, err
	}
	return value.(Result), nil
}

func (s *Service) RegisterSamples(ctx context.Context, cmd RegisterCommand) (Result, error) {
	if err := domain.RequireRole(cmd.Role, domain.RoleManager); err != nil {
		return Result{}, err
	}
	if len(cmd.Samples) == 0 || len(cmd.Samples) > 100 {
		return Result{}, domain.NewError(domain.CodeInvalid, "每批 samples 数量必须处于 1 到 100")
	}
	return s.run(ctx, cmd.Metadata, func(_ context.Context, snapshot *domain.Snapshot) (Result, string, error) {
		if err := domain.EnsureMutable(snapshot.Dataset); err != nil {
			return Result{}, "", err
		}
		seenID, seenSource, seenHash := map[string]bool{}, map[string]bool{}, map[string]bool{}
		for _, x := range snapshot.Samples {
			seenID[x.ID] = true
			seenSource[x.SourceRef] = true
			seenHash[x.SHA256] = true
		}
		added := make([]domain.RecordingSample, 0, len(cmd.Samples))
		now := s.now().UTC()
		for _, draft := range cmd.Samples {
			if draft.ID == "" {
				draft.ID = s.newID("sample")
			}
			if seenID[draft.ID] || seenSource[draft.SourceRef] || seenHash[draft.SHA256] {
				return Result{}, "", domain.NewError(domain.CodeConflict, "样本 id、sourceRef 或 sha256 重复")
			}
			sample, err := domain.NewSample(domain.SampleInput{ID: draft.ID, DatasetID: cmd.DatasetID, SourceRef: draft.SourceRef, CapturedAt: draft.CapturedAt, LatitudeBand: draft.LatitudeBand, LongitudeBand: draft.LongitudeBand, SampleRateHz: draft.SampleRateHz, Channels: draft.Channels, DurationMs: draft.DurationMs, SHA256: draft.SHA256, RegisteredBy: cmd.Actor}, snapshot.Dataset, now)
			if err != nil {
				return Result{}, "", err
			}
			seenID[sample.ID] = true
			seenSource[sample.SourceRef] = true
			seenHash[sample.SHA256] = true
			added = append(added, sample)
		}
		snapshot.Samples = append(snapshot.Samples, added...)
		domain.Advance(&snapshot.Dataset, snapshot.Dataset.Status, now)
		recompute(snapshot)
		return Result{Dataset: snapshot.Dataset, Data: added}, "samples.registered", nil
	})
}

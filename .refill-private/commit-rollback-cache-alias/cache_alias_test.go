package commitrollbackcachealias_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"bioacoustic-release-hub/internal/application"
	"bioacoustic-release-hub/internal/domain"
	"bioacoustic-release-hub/internal/store/sqlite"
)

type failCommitRepository struct {
	domain.Repository
	failNext bool
}

func (r *failCommitRepository) Commit(ctx context.Context, snapshot domain.Snapshot, expectedRevision int64, event domain.AuditEvent, requestID string, response json.RawMessage) error {
	if r.failNext {
		r.failNext = false
		return errors.New("forced commit failure")
	}
	return r.Repository.Commit(ctx, snapshot, expectedRevision, event, requestID, response)
}

func TestCommitFailureDoesNotPoisonSnapshotCache(t *testing.T) {
	ctx := context.Background()
	store, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	repo := &failCommitRepository{Repository: store}
	service := application.NewService(repo)

	created, err := service.CreateDataset(ctx, application.CreateCommand{
		RequestID: "create", Role: domain.RoleManager, Actor: "manager", ID: "cache-alias-dataset",
		Title: "林鸟复核", ResearchGoal: "验证裁决提交失败后的重试", TargetTaxa: []string{"aves"},
		RecordingRegion: "华东", QualityRuleVersion: "bio-v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	registered, err := service.RegisterSamples(ctx, application.RegisterCommand{
		Metadata: application.Metadata{DatasetID: "cache-alias-dataset", RequestID: "register", Revision: created.Dataset.Revision, Role: domain.RoleManager, Actor: "manager"},
		Samples: []application.SampleDraft{{
			ID: "sample-1", SourceRef: "field/cache-alias.wav", CapturedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
			LatitudeBand: "N30-N31", LongitudeBand: "E120-E121", SampleRateHz: 48000, Channels: 1, DurationMs: 4000,
			SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	assessed, err := service.AssessSignal(ctx, application.AssessCommand{
		Metadata: application.Metadata{DatasetID: "cache-alias-dataset", RequestID: "assess", Revision: registered.Dataset.Revision, Role: domain.RoleAnnotator, Actor: "annotator"},
		SampleID: "sample-1", SignalToNoiseDB: 20, ClippingRatio: 0.001, SilenceRatio: 0.1,
	})
	if err != nil {
		t.Fatal(err)
	}
	annotated, err := service.SubmitAnnotation(ctx, application.AnnotateCommand{
		Metadata: application.Metadata{DatasetID: "cache-alias-dataset", RequestID: "annotate", Revision: assessed.Dataset.Revision, Role: domain.RoleAnnotator, Actor: "annotator"},
		SampleID: "sample-1", Segments: []domain.Segment{{StartMs: 10, EndMs: 2000}}, SpeciesCode: "UNKNOWN",
		CallType: "contact", Confidence: 0.5, EvidenceNote: "置信度不足，等待专家复核",
	})
	if err != nil {
		t.Fatal(err)
	}
	issues, err := service.ListIssues(ctx, "cache-alias-dataset", domain.Page{})
	if err != nil || len(issues) != 1 {
		t.Fatalf("issues=%+v err=%v", issues, err)
	}

	repo.failNext = true
	failedCommand := application.DecideCommand{
		Metadata: application.Metadata{DatasetID: "cache-alias-dataset", RequestID: "decision-failed", Revision: annotated.Dataset.Revision, Role: domain.RoleExpert, Actor: "expert"},
		IssueID:  issues[0].ID, Decision: domain.DecisionConfirm, Note: "确认当前标注可用",
	}
	if _, err := service.DecideIssue(ctx, failedCommand); err == nil || err.Error() != "forced commit failure" {
		t.Fatalf("首次裁决应在 Commit 失败，err=%v", err)
	}
	persisted, err := store.ListIssues(ctx, "cache-alias-dataset", domain.Page{})
	if err != nil || len(persisted) != 1 || persisted[0].Status != domain.IssueOpen {
		t.Fatalf("事务回滚后仓储应保留 open 问题，issues=%+v err=%v", persisted, err)
	}

	retry := failedCommand
	retry.RequestID = "decision-retry"
	if _, err := service.DecideIssue(ctx, retry); err != nil {
		t.Fatalf("同 revision 重试应从已回滚的仓储状态重新裁决，err=%v", err)
	}
}

package replaycacherace_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"bioacoustic-release-hub/internal/application"
	"bioacoustic-release-hub/internal/domain"
)

type commitBarrierRepository struct {
	commits chan struct{}
	release chan struct{}
}

func (r *commitBarrierRepository) Create(context.Context, domain.Snapshot, domain.AuditEvent, string, json.RawMessage) error {
	return nil
}

func (r *commitBarrierRepository) Load(_ context.Context, datasetID string) (domain.Snapshot, error) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	return domain.Snapshot{Dataset: domain.Dataset{
		ID:                 datasetID,
		Title:              "并发缓存复现",
		ResearchGoal:       "验证跨数据集写后重放",
		TargetTaxa:         []string{"aves"},
		RecordingRegion:    "华东",
		QualityRuleVersion: "bio-v1",
		Status:             domain.StatusDraft,
		Revision:           1,
		CreatedBy:          "manager",
		CreatedAt:          now,
		UpdatedAt:          now,
	}}, nil
}

func (r *commitBarrierRepository) Commit(context.Context, domain.Snapshot, int64, domain.AuditEvent, string, json.RawMessage) error {
	r.commits <- struct{}{}
	<-r.release
	return nil
}

func (r *commitBarrierRepository) IdempotentResult(context.Context, string, string) (json.RawMessage, bool, error) {
	return nil, false, nil
}

func (r *commitBarrierRepository) ListSamples(context.Context, string, domain.Page) ([]domain.RecordingSample, error) {
	return nil, nil
}

func (r *commitBarrierRepository) ListIssues(context.Context, string, domain.Page) ([]domain.ReviewIssue, error) {
	return nil, nil
}

func (r *commitBarrierRepository) QueryIssues(context.Context, string, domain.IssueFilter, domain.Page) (domain.IssueQueryResult, error) {
	return domain.IssueQueryResult{}, nil
}

func (r *commitBarrierRepository) Timeline(context.Context, string, domain.Page) ([]domain.AuditEvent, error) {
	return nil, nil
}

func (r *commitBarrierRepository) FindCredential(context.Context, string) (domain.ReleaseCredential, error) {
	return domain.ReleaseCredential{}, nil
}

func (r *commitBarrierRepository) VerifyIntegrity(context.Context) error { return nil }
func (r *commitBarrierRepository) Close() error                          { return nil }

func TestConcurrentDatasetsRaceOnReplayCache(t *testing.T) {
	repo := &commitBarrierRepository{commits: make(chan struct{}, 2), release: make(chan struct{})}
	service := application.NewService(repo)
	start := make(chan struct{})
	errors := make(chan error, 2)
	var workers sync.WaitGroup
	for index, datasetID := range []string{"dataset-a", "dataset-b"} {
		workers.Add(1)
		go func(index int, datasetID string) {
			defer workers.Done()
			<-start
			_, err := service.RegisterSamples(context.Background(), application.RegisterCommand{
				Metadata: application.Metadata{DatasetID: datasetID, RequestID: fmt.Sprintf("request-%d", index), Revision: 1, Role: domain.RoleManager, Actor: "manager"},
				Samples: []application.SampleDraft{{
					ID: fmt.Sprintf("sample-%d", index), SourceRef: fmt.Sprintf("field/%d.wav", index),
					CapturedAt: time.Date(2026, 1, 1, 0, 0, index, 0, time.UTC), LatitudeBand: "N30-N31", LongitudeBand: "E120-E121",
					SampleRateHz: 48000, Channels: 1, DurationMs: 1000,
					SHA256: fmt.Sprintf("%064x", index+1),
				}},
			})
			errors <- err
		}(index, datasetID)
	}
	close(start)
	<-repo.commits
	<-repo.commits
	close(repo.release)
	workers.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatalf("并发写命令失败: %v", err)
		}
	}
}

package contextcancelcommit_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"bioacoustic-release-hub/internal/application"
	"bioacoustic-release-hub/internal/domain"
)

type controlledRepository struct {
	snapshot       domain.Snapshot
	commitStarted  chan struct{}
	releaseCommit  chan struct{}
	commitFinished chan struct{}
	committed      bool
}

func newControlledRepository() *controlledRepository {
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	return &controlledRepository{
		snapshot: domain.Snapshot{Dataset: domain.Dataset{
			ID: "ds_context", Title: "context lifecycle", ResearchGoal: "verify cancellation",
			TargetTaxa: []string{"aves"}, RecordingRegion: "test-region", QualityRuleVersion: "bio-v1",
			Status: domain.StatusDraft, Revision: 1, CreatedBy: "manager", CreatedAt: now, UpdatedAt: now,
		}},
		commitStarted: make(chan struct{}), releaseCommit: make(chan struct{}), commitFinished: make(chan struct{}),
	}
}

func (r *controlledRepository) Create(context.Context, domain.Snapshot, domain.AuditEvent, string, json.RawMessage) error {
	return errors.New("unexpected Create")
}

func (r *controlledRepository) Load(context.Context, string) (domain.Snapshot, error) {
	return r.snapshot, nil
}

func (r *controlledRepository) Commit(ctx context.Context, snapshot domain.Snapshot, _ int64, _ domain.AuditEvent, _ string, _ json.RawMessage) error {
	close(r.commitStarted)
	defer close(r.commitFinished)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-r.releaseCommit:
		r.snapshot = snapshot
		r.committed = true
		return nil
	}
}

func (r *controlledRepository) IdempotentResult(context.Context, string, string) (json.RawMessage, bool, error) {
	return nil, false, nil
}

func (r *controlledRepository) ListSamples(context.Context, string, domain.Page) ([]domain.RecordingSample, error) {
	return nil, errors.New("unexpected ListSamples")
}

func (r *controlledRepository) ListIssues(context.Context, string, domain.Page) ([]domain.ReviewIssue, error) {
	return nil, errors.New("unexpected ListIssues")
}

func (r *controlledRepository) QueryIssues(context.Context, string, domain.IssueFilter, domain.Page) (domain.IssueQueryResult, error) {
	return domain.IssueQueryResult{}, errors.New("unexpected QueryIssues")
}

func (r *controlledRepository) Timeline(context.Context, string, domain.Page) ([]domain.AuditEvent, error) {
	return nil, errors.New("unexpected Timeline")
}

func (r *controlledRepository) FindCredential(context.Context, string) (domain.ReleaseCredential, error) {
	return domain.ReleaseCredential{}, errors.New("unexpected FindCredential")
}

func (r *controlledRepository) VerifyIntegrity(context.Context) error { return nil }
func (r *controlledRepository) Close() error                          { return nil }

func TestCanceledQueuedCommandDoesNotCommit(t *testing.T) {
	repo := newControlledRepository()
	service := application.NewService(repo)
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := service.RegisterSamples(ctx, application.RegisterCommand{
			Metadata: application.Metadata{DatasetID: "ds_context", RequestID: "req-cancel", Revision: 1, Role: domain.RoleManager, Actor: "manager"},
			Samples: []application.SampleDraft{{
				ID: "sample_context", SourceRef: "recorder://context", CapturedAt: time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC),
				LatitudeBand: "N30", LongitudeBand: "E120", SampleRateHz: 48000, Channels: 1, DurationMs: 1000,
				SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			}},
		})
		result <- err
	}()

	<-repo.commitStarted
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled request returned %v, want context.Canceled", err)
	}
	close(repo.releaseCommit)
	<-repo.commitFinished

	if repo.committed {
		t.Fatalf("canceled command was committed after its caller observed context.Canceled")
	}
}

package query_coalesced_context_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"bioacoustic-release-hub/internal/application"
	"bioacoustic-release-hub/internal/domain"
)

type repositoryStub struct{}

func (repositoryStub) Create(context.Context, domain.Snapshot, domain.AuditEvent, string, json.RawMessage) error {
	panic("unexpected Create")
}
func (repositoryStub) Commit(context.Context, domain.Snapshot, int64, domain.AuditEvent, string, json.RawMessage) error {
	panic("unexpected Commit")
}
func (repositoryStub) IdempotentResult(context.Context, string, string) (json.RawMessage, bool, error) {
	panic("unexpected IdempotentResult")
}
func (repositoryStub) ListSamples(context.Context, string, domain.Page) ([]domain.RecordingSample, error) {
	panic("unexpected ListSamples")
}
func (repositoryStub) ListIssues(context.Context, string, domain.Page) ([]domain.ReviewIssue, error) {
	panic("unexpected ListIssues")
}
func (repositoryStub) QueryIssues(context.Context, string, domain.IssueFilter, domain.Page) (domain.IssueQueryResult, error) {
	panic("unexpected QueryIssues")
}
func (repositoryStub) Timeline(context.Context, string, domain.Page) ([]domain.AuditEvent, error) {
	panic("unexpected Timeline")
}
func (repositoryStub) FindCredential(context.Context, string) (domain.ReleaseCredential, error) {
	panic("unexpected FindCredential")
}
func (repositoryStub) VerifyIntegrity(context.Context) error { return nil }
func (repositoryStub) Close() error                          { return nil }

type blockingRepository struct {
	repositoryStub
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (r *blockingRepository) Load(ctx context.Context, datasetID string) (domain.Snapshot, error) {
	r.once.Do(func() { close(r.entered) })
	select {
	case <-r.release:
		return domain.Snapshot{Dataset: domain.Dataset{ID: datasetID, Status: domain.StatusDraft}}, nil
	case <-ctx.Done():
		return domain.Snapshot{}, ctx.Err()
	}
}

type observedContext struct {
	context.Context
	observed chan struct{}
	done     chan struct{}
	once     sync.Once
}

func (c *observedContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.observed) })
	return c.done
}

func TestActiveWaiterSurvivesCoalescedLeaderCancellation(t *testing.T) {
	repo := &blockingRepository{entered: make(chan struct{}), release: make(chan struct{})}
	service := application.NewService(repo)
	leaderContext, cancelLeader := context.WithCancel(context.Background())
	leaderResult := make(chan error, 1)
	go func() {
		_, err := service.GetDataset(leaderContext, "dataset-1")
		leaderResult <- err
	}()
	<-repo.entered

	waiterJoined := make(chan struct{})
	waiterContext := &observedContext{
		Context:  context.Background(),
		observed: waiterJoined,
		done:     make(chan struct{}),
	}
	waiterResult := make(chan error, 1)
	go func() {
		_, err := service.GetDataset(waiterContext, "dataset-1")
		waiterResult <- err
	}()
	<-waiterJoined

	cancelLeader()
	if err := <-leaderResult; !errors.Is(err, context.Canceled) {
		t.Fatalf("leader cancellation returned %v", err)
	}
	close(repo.release)
	if err := <-waiterResult; err != nil {
		t.Fatalf("active waiter inherited canceled shared load: %v", err)
	}
}

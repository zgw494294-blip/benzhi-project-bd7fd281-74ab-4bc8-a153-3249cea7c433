package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"bioacoustic-release-hub/internal/domain"
)

type Service struct {
	repo          domain.Repository
	mailboxes     *mailboxRegistry
	now           func() time.Time
	newID         func(string) string
	snapshotMu    sync.RWMutex
	snapshotCache map[string]domain.Snapshot
}

func NewService(repo domain.Repository) *Service {
	return &Service{
		repo: repo, mailboxes: newMailboxRegistry(64), now: time.Now, newID: randomID,
		snapshotCache: make(map[string]domain.Snapshot),
	}
}

func randomID(prefix string) string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return prefix + "_" + hex.EncodeToString(b[:])
}

func validateMetadata(meta Metadata) error {
	if meta.DatasetID == "" {
		return domain.NewError(domain.CodeInvalid, "datasetId 不能为空")
	}
	if meta.RequestID == "" || len(meta.RequestID) > 128 {
		return domain.NewError(domain.CodeInvalid, "X-Request-Id 不能为空且不能超过 128 字符")
	}
	if meta.Actor == "" || len(meta.Actor) > 128 {
		return domain.NewError(domain.CodeInvalid, "X-Actor 不能为空且不能超过 128 字符")
	}
	return nil
}

func (s *Service) loadSnapshot(ctx context.Context, datasetID string) (domain.Snapshot, error) {
	s.snapshotMu.RLock()
	snapshot, ok := s.snapshotCache[datasetID]
	s.snapshotMu.RUnlock()
	if ok {
		return snapshot, nil
	}
	snapshot, err := s.repo.Load(ctx, datasetID)
	if err != nil {
		return domain.Snapshot{}, err
	}
	s.rememberSnapshot(snapshot)
	return snapshot, nil
}

func (s *Service) rememberSnapshot(snapshot domain.Snapshot) {
	s.snapshotMu.Lock()
	s.snapshotCache[snapshot.Dataset.ID] = snapshot
	s.snapshotMu.Unlock()
}

func (s *Service) run(ctx context.Context, meta Metadata, fn func(context.Context, *domain.Snapshot) (Result, string, error)) (Result, error) {
	if err := validateMetadata(meta); err != nil {
		return Result{}, err
	}
	value, err := s.mailboxes.forDataset(meta.DatasetID).submit(ctx, func(inner context.Context) (any, error) {
		if raw, ok, err := s.repo.IdempotentResult(inner, meta.DatasetID, meta.RequestID); err != nil {
			return nil, err
		} else if ok {
			var result Result
			if err := json.Unmarshal(raw, &result); err != nil {
				return nil, fmt.Errorf("解码幂等响应: %w", err)
			}
			result.Idempotent = true
			return result, nil
		}
		snapshot, err := s.loadSnapshot(inner, meta.DatasetID)
		if err != nil {
			return nil, err
		}
		if err := domain.CheckRevision(snapshot.Dataset, meta.Revision); err != nil {
			return nil, err
		}
		expected := snapshot.Dataset.Revision
		result, eventType, err := fn(inner, &snapshot)
		if err != nil {
			return nil, err
		}
		raw, err := json.Marshal(result)
		if err != nil {
			return nil, err
		}
		details := map[string]any{"status": result.Dataset.Status}
		if result.Data != nil {
			details["result"] = result.Data
		}
		event := domain.AuditEvent{DatasetID: meta.DatasetID, Revision: result.Dataset.Revision, EventType: eventType, Actor: meta.Actor, RequestID: meta.RequestID, Details: details, OccurredAt: s.now().UTC()}
		if err := s.repo.Commit(inner, snapshot, expected, event, meta.RequestID, raw); err != nil {
			return nil, err
		}
		s.rememberSnapshot(snapshot)
		return result, nil
	})
	if err != nil {
		return Result{}, err
	}
	return value.(Result), nil
}

func (s *Service) loadConsistent(ctx context.Context, datasetID string) (domain.Snapshot, error) {
	value, err := s.mailboxes.forDataset(datasetID).submit(ctx, func(inner context.Context) (any, error) {
		return s.loadSnapshot(inner, datasetID)
	})
	if err != nil {
		return domain.Snapshot{}, err
	}
	return value.(domain.Snapshot), nil
}

func findSample(snapshot domain.Snapshot, id string) (domain.RecordingSample, error) {
	for _, sample := range snapshot.Samples {
		if sample.ID == id {
			return sample, nil
		}
	}
	return domain.RecordingSample{}, domain.NewError(domain.CodeNotFound, "样本 %s 不存在", id)
}
func latestAssessment(snapshot domain.Snapshot, sampleID string) *domain.SignalAssessment {
	var found *domain.SignalAssessment
	for i := range snapshot.Assessments {
		a := &snapshot.Assessments[i]
		if a.SampleID == sampleID && (found == nil || a.AssessedAt.After(found.AssessedAt)) {
			found = a
		}
	}
	return found
}
func latestAnnotation(snapshot domain.Snapshot, sampleID string) *domain.AnnotationRevision {
	var found *domain.AnnotationRevision
	for i := range snapshot.Annotations {
		a := &snapshot.Annotations[i]
		if a.SampleID == sampleID && (found == nil || a.RevisionNo > found.RevisionNo) {
			found = a
		}
	}
	return found
}
func statusCounts(snapshot domain.Snapshot) (int, int, int, int, int) {
	assessed, annotated, open, returned := 0, 0, 0, 0
	for _, sample := range snapshot.Samples {
		if latestAssessment(snapshot, sample.ID) != nil {
			assessed++
		}
		if latestAnnotation(snapshot, sample.ID) != nil {
			annotated++
		}
	}
	for _, issue := range snapshot.Issues {
		if issue.Status == domain.IssueOpen {
			open++
		}
		if issue.Status == domain.IssueReturned {
			returned++
		}
	}
	return len(snapshot.Samples), assessed, annotated, open, returned
}
func recompute(snapshot *domain.Snapshot) {
	a, b, c, d, e := statusCounts(*snapshot)
	domain.RecomputeStatus(&snapshot.Dataset, a, b, c, d, e)
}

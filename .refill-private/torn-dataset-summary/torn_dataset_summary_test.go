package torn_dataset_summary_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"bioacoustic-release-hub/internal/application"
	"bioacoustic-release-hub/internal/domain"
	httptransport "bioacoustic-release-hub/internal/transport/http"
)

type stagedRepository struct {
	domain.Repository
	loadCaptured chan struct{}
	commitDone   chan struct{}
	captureOnce  sync.Once
	oldSnapshot  domain.Snapshot
	newSamples   []domain.RecordingSample
	newIssues    domain.IssueQueryResult
}

func newStagedRepository(oldSnapshot domain.Snapshot, newSamples []domain.RecordingSample, newIssues domain.IssueQueryResult) *stagedRepository {
	return &stagedRepository{
		loadCaptured: make(chan struct{}),
		commitDone:   make(chan struct{}),
		oldSnapshot:  oldSnapshot,
		newSamples:   newSamples,
		newIssues:    newIssues,
	}
}

func (r *stagedRepository) Load(context.Context, string) (domain.Snapshot, error) {
	r.captureOnce.Do(func() { close(r.loadCaptured) })
	<-r.commitDone
	return r.oldSnapshot, nil
}

func (r *stagedRepository) ListSamples(context.Context, string, domain.Page) ([]domain.RecordingSample, error) {
	return r.newSamples, nil
}

func (r *stagedRepository) QueryIssues(context.Context, string, domain.IssueFilter, domain.Page) (domain.IssueQueryResult, error) {
	return r.newIssues, nil
}

func requestDatasetView(t *testing.T, repo *stagedRepository) struct {
	Dataset struct {
		Revision int64 `json:"revision"`
	} `json:"dataset"`
	SampleCount     int `json:"sampleCount"`
	AnnotationCount int `json:"annotationCount"`
	OpenIssueCount  int `json:"openIssueCount"`
} {
	t.Helper()
	handler := httptransport.NewHandler(application.NewService(repo))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/datasets/dataset-torn-view", nil)
	recorder := httptest.NewRecorder()
	requestDone := make(chan struct{})

	go func() {
		handler.ServeHTTP(recorder, request)
		close(requestDone)
	}()

	<-repo.loadCaptured
	close(repo.commitDone)
	<-requestDone

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected HTTP status %d: %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Data struct {
			Dataset struct {
				Revision int64 `json:"revision"`
			} `json:"dataset"`
			SampleCount     int `json:"sampleCount"`
			AnnotationCount int `json:"annotationCount"`
			OpenIssueCount  int `json:"openIssueCount"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return response.Data
}

func TestDatasetSummaryUsesOneRepositorySnapshot(t *testing.T) {
	t.Run("register commit", func(t *testing.T) {
		old := domain.Snapshot{Dataset: domain.Dataset{ID: "dataset-torn-view", Status: domain.StatusDraft, Revision: 1}}
		newSamples := []domain.RecordingSample{{ID: "sample-after-commit", DatasetID: old.Dataset.ID}}
		view := requestDatasetView(t, newStagedRepository(old, newSamples, domain.IssueQueryResult{}))
		if view.Dataset.Revision != 1 || view.SampleCount != 0 {
			t.Errorf("revision 1 was torn with the registered sample: revision=%d sampleCount=%d", view.Dataset.Revision, view.SampleCount)
		}
	})

	t.Run("annotation commit", func(t *testing.T) {
		old := domain.Snapshot{
			Dataset:     domain.Dataset{ID: "dataset-torn-view", Status: domain.StatusScreened, Revision: 3},
			Samples:     []domain.RecordingSample{{ID: "sample-existing", DatasetID: "dataset-torn-view"}},
			Assessments: []domain.SignalAssessment{{ID: "assessment-existing", DatasetID: "dataset-torn-view", SampleID: "sample-existing"}},
		}
		newIssues := domain.IssueQueryResult{StatusSummary: map[string]int{string(domain.IssueOpen): 1}}
		view := requestDatasetView(t, newStagedRepository(old, old.Samples, newIssues))
		if view.Dataset.Revision != 3 || view.AnnotationCount != 0 || view.OpenIssueCount != 0 {
			t.Errorf("revision 3 was torn with the new annotation issue: revision=%d annotationCount=%d openIssueCount=%d",
				view.Dataset.Revision, view.AnnotationCount, view.OpenIssueCount)
		}
	})
}

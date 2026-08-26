package issue_filter_cache_isolation_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"bioacoustic-release-hub/internal/application"
	"bioacoustic-release-hub/internal/domain"
	httptransport "bioacoustic-release-hub/internal/transport/http"
)

type filterRepository struct{}

func (filterRepository) QueryIssues(_ context.Context, datasetID string, filter domain.IssueFilter, _ domain.Page) (domain.IssueQueryResult, error) {
	status := domain.IssueStatus(filter.Status)
	return domain.IssueQueryResult{
		Items:         []domain.ReviewIssue{{ID: "issue-" + filter.Status, DatasetID: datasetID, Status: status}},
		Total:         1,
		StatusSummary: map[string]int{filter.Status: 1},
		KindSummary:   map[string]int{"low_confidence": 1},
	}, nil
}

func (filterRepository) Create(context.Context, domain.Snapshot, domain.AuditEvent, string, json.RawMessage) error {
	return errors.New("unexpected Create")
}
func (filterRepository) Load(context.Context, string) (domain.Snapshot, error) {
	return domain.Snapshot{}, errors.New("unexpected Load")
}
func (filterRepository) Commit(context.Context, domain.Snapshot, int64, domain.AuditEvent, string, json.RawMessage) error {
	return errors.New("unexpected Commit")
}
func (filterRepository) IdempotentResult(context.Context, string, string) (json.RawMessage, bool, error) {
	return nil, false, errors.New("unexpected IdempotentResult")
}
func (filterRepository) ListSamples(context.Context, string, domain.Page) ([]domain.RecordingSample, error) {
	return nil, errors.New("unexpected ListSamples")
}
func (filterRepository) ListIssues(context.Context, string, domain.Page) ([]domain.ReviewIssue, error) {
	return nil, errors.New("unexpected ListIssues")
}
func (filterRepository) Timeline(context.Context, string, domain.Page) ([]domain.AuditEvent, error) {
	return nil, errors.New("unexpected Timeline")
}
func (filterRepository) FindCredential(context.Context, string) (domain.ReleaseCredential, error) {
	return domain.ReleaseCredential{}, errors.New("unexpected FindCredential")
}
func (filterRepository) VerifyIntegrity(context.Context) error { return nil }
func (filterRepository) Close() error                          { return nil }

func TestIssueQueryCacheSeparatesFilters(t *testing.T) {
	handler := httptransport.NewHandler(application.NewService(filterRepository{}))
	query := func(status string) application.IssueQueue {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/datasets/dataset/issues?status="+status+"&limit=10&offset=0", nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		if response.Code != http.StatusOK {
			t.Fatalf("status=%s 返回 HTTP %d: %s", status, response.Code, response.Body.String())
		}
		var payload struct {
			Data application.IssueQueue `json:"data"`
		}
		if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
			t.Fatalf("解码 status=%s 响应: %v", status, err)
		}
		return payload.Data
	}

	open := query("open")
	if len(open.Items) != 1 || open.Items[0].Status != domain.IssueOpen {
		t.Fatalf("首次 open 查询返回错误结果: %+v", open.Items)
	}
	closed := query("closed")
	if len(closed.Items) != 1 || closed.Items[0].Status != domain.IssueClosed {
		t.Fatalf("closed 查询复用了其他过滤条件的缓存: %+v", closed.Items)
	}
}

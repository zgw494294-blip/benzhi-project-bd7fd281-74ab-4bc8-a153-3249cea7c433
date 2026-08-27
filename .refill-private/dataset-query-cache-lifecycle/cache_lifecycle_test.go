package datasetquerycachelifecycle_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"bioacoustic-release-hub/internal/application"
	"bioacoustic-release-hub/internal/store/sqlite"
	httptransport "bioacoustic-release-hub/internal/transport/http"
)

type datasetEnvelope struct {
	Data struct {
		Dataset struct {
			Revision int64 `json:"revision"`
		} `json:"dataset"`
		SampleCount int `json:"sampleCount"`
	} `json:"data"`
}

func performRequest(handler http.Handler, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func createDataset(t *testing.T, handler http.Handler, id, requestID string) {
	t.Helper()
	body := `{"id":"` + id + `","title":"缓存生命周期验证","researchGoal":"验证数据集查询跨写入后的可见性","targetTaxa":["aves"],"recordingRegion":"华东","qualityRuleVersion":"bio-v1"}`
	response := performRequest(handler, http.MethodPost, "/api/v1/datasets", body, map[string]string{
		"Content-Type": "application/json",
		"X-Role":       "manager",
		"X-Actor":      "manager",
		"X-Request-Id": requestID,
	})
	if response.Code != http.StatusCreated {
		t.Fatalf("创建数据集 %s 失败: status=%d body=%s", id, response.Code, response.Body.String())
	}
}

func TestDatasetQueryCacheTracksLifecycle(t *testing.T) {
	repository, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	handler := httptransport.NewHandler(application.NewService(repository))

	missing := performRequest(handler, http.MethodGet, "/api/v1/datasets/created-later", "", nil)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("预热负缓存时 status=%d", missing.Code)
	}
	createDataset(t, handler, "created-later", "create-later")
	afterCreate := performRequest(handler, http.MethodGet, "/api/v1/datasets/created-later", "", nil)
	negativeCacheCleared := afterCreate.Code == http.StatusOK

	createDataset(t, handler, "active-dataset", "create-active")
	initial := performRequest(handler, http.MethodGet, "/api/v1/datasets/active-dataset", "", nil)
	if initial.Code != http.StatusOK {
		t.Fatalf("预热数据集缓存时 status=%d body=%s", initial.Code, initial.Body.String())
	}
	registerBody := `{"samples":[{"id":"sample-1","sourceRef":"field/sample-1.wav","capturedAt":"2026-01-01T00:00:00Z","latitudeBand":"N30-N31","longitudeBand":"E120-E121","sampleRateHz":48000,"channels":1,"durationMs":4000,"sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}]}`
	registered := performRequest(handler, http.MethodPost, "/api/v1/datasets/active-dataset/samples", registerBody, map[string]string{
		"Content-Type":      "application/json",
		"If-Match-Revision": "1",
		"X-Role":            "manager",
		"X-Actor":           "manager",
		"X-Request-Id":      "register-sample",
	})
	if registered.Code != http.StatusOK {
		t.Fatalf("登记样本失败: status=%d body=%s", registered.Code, registered.Body.String())
	}
	afterWrite := performRequest(handler, http.MethodGet, "/api/v1/datasets/active-dataset", "", nil)
	var view datasetEnvelope
	if afterWrite.Code == http.StatusOK {
		if err := json.Unmarshal(afterWrite.Body.Bytes(), &view); err != nil {
			t.Fatal(err)
		}
	}
	positiveCacheRefreshed := afterWrite.Code == http.StatusOK && view.Data.Dataset.Revision == 2 && view.Data.SampleCount == 1

	if !negativeCacheCleared || !positiveCacheRefreshed {
		t.Fatalf("数据集查询缓存未跟随生命周期更新: afterCreate=%d afterWriteRevision=%d sampleCount=%d", afterCreate.Code, view.Data.Dataset.Revision, view.Data.SampleCount)
	}
}

package missingdatasetcollections

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"bioacoustic-release-hub/internal/application"
	"bioacoustic-release-hub/internal/store/sqlite"
	httptransport "bioacoustic-release-hub/internal/transport/http"
)

func TestMissingDatasetCollectionEndpointsReturnNotFound(t *testing.T) {
	store, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	handler := httptransport.NewHandler(application.NewService(store))

	samples := httptest.NewRecorder()
	handler.ServeHTTP(samples, httptest.NewRequest(http.MethodGet, "/api/v1/datasets/missing/samples", nil))
	timeline := httptest.NewRecorder()
	handler.ServeHTTP(timeline, httptest.NewRequest(http.MethodGet, "/api/v1/datasets/missing/timeline", nil))
	if samples.Code != http.StatusNotFound || timeline.Code != http.StatusNotFound {
		t.Fatalf("TestMissingDatasetCollectionEndpointsReturnNotFound: samples=%d timeline=%d", samples.Code, timeline.Code)
	}
}

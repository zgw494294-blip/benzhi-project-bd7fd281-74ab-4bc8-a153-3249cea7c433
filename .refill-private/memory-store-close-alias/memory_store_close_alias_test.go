package memory_store_close_alias_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"bioacoustic-release-hub/internal/application"
	"bioacoustic-release-hub/internal/domain"
	"bioacoustic-release-hub/internal/store/sqlite"
	httptransport "bioacoustic-release-hub/internal/transport/http"
)

func TestClosingOneMemoryStoreDoesNotInvalidateAnother(t *testing.T) {
	first, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	second, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = second.Close() })

	service := application.NewService(second)
	created, err := service.CreateDataset(context.Background(), application.CreateCommand{
		RequestID:          "create-secondary",
		Role:               domain.RoleManager,
		Actor:              "manager",
		ID:                 "secondary-dataset",
		Title:              "独立内存仓储",
		ResearchGoal:       "验证仓储关闭所有权",
		TargetTaxa:         []string{"aves"},
		RecordingRegion:    "华东",
		QualityRuleVersion: "bio-v1",
	})
	if err != nil {
		t.Fatalf("初始化第二个仓储: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("关闭第一个仓储: %v", err)
	}

	handler := httptransport.NewHandler(service)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/datasets/"+created.Dataset.ID, nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("关闭另一调用方的仓储后 GET 数据集: status=%d body=%s", response.Code, response.Body.String())
	}
}

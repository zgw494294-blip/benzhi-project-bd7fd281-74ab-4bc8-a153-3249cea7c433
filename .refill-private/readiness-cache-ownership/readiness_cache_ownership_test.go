package readiness_cache_ownership_test

import (
	"context"
	"testing"

	"bioacoustic-release-hub/internal/application"
	"bioacoustic-release-hub/internal/domain"
	"bioacoustic-release-hub/internal/store/sqlite"
)

func TestReadinessCacheDoesNotExposeMutableState(t *testing.T) {
	repo, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })

	service := application.NewService(repo)
	_, err = service.CreateDataset(context.Background(), application.CreateCommand{
		RequestID:          "create-readiness-cache-fixture",
		Role:               domain.RoleManager,
		Actor:              "manager",
		ID:                 "readiness-cache-fixture",
		Title:              "就绪度缓存所有权复现",
		ResearchGoal:       "验证查询返回值不会反向污染服务缓存",
		TargetTaxa:         []string{"aves"},
		RecordingRegion:    "华东",
		QualityRuleVersion: "bio-v1",
	})
	if err != nil {
		t.Fatal(err)
	}

	first, err := service.FreezeReadiness(context.Background(), "readiness-cache-fixture")
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Blockers) != 1 || first.Blockers[0].Kind != "empty_dataset" {
		t.Fatalf("unexpected fixture readiness: %+v", first)
	}

	first.Counts.IssuesByStatus["open"] = 41
	first.Blockers[0].Kind = "caller-poisoned"

	second, err := service.FreezeReadiness(context.Background(), "readiness-cache-fixture")
	if err != nil {
		t.Fatal(err)
	}
	if second.Counts.IssuesByStatus["open"] != 0 || len(second.Blockers) != 1 || second.Blockers[0].Kind != "empty_dataset" {
		t.Fatalf("cached readiness was mutated through a prior result: %+v", second)
	}
}

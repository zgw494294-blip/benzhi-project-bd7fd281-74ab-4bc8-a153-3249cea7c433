package application

import (
	"context"
	"testing"
	"time"

	"bioacoustic-release-hub/internal/domain"
	"bioacoustic-release-hub/internal/store/sqlite"
)

func newExtensionFixture(t *testing.T, sampleIDs ...string) (*Service, *sqlite.Store, int64) {
	t.Helper()
	repo, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	service := NewService(repo)
	created, err := service.CreateDataset(context.Background(), CreateCommand{RequestID: "create", Role: domain.RoleManager, Actor: "manager", ID: "dataset", Title: "批量声纹", ResearchGoal: "验证扩展流程", TargetTaxa: []string{"aves"}, RecordingRegion: "华东", QualityRuleVersion: "bio-v1"})
	if err != nil {
		t.Fatal(err)
	}
	drafts := make([]SampleDraft, 0, len(sampleIDs))
	for i, id := range sampleIDs {
		hash := make([]byte, 64)
		for j := range hash {
			hash[j] = byte('a' + i)
		}
		drafts = append(drafts, SampleDraft{ID: id, SourceRef: "field/" + id + ".wav", CapturedAt: time.Date(2026, 1, i+1, 0, 0, 0, 0, time.UTC), LatitudeBand: "N30-N31", LongitudeBand: "E120-E121", SampleRateHz: 48000, Channels: 1, DurationMs: 4000, SHA256: string(hash)})
	}
	registered, err := service.RegisterSamples(context.Background(), RegisterCommand{Metadata: Metadata{DatasetID: "dataset", RequestID: "register", Revision: created.Dataset.Revision, Role: domain.RoleManager, Actor: "manager"}, Samples: drafts})
	if err != nil {
		t.Fatal(err)
	}
	return service, repo, registered.Dataset.Revision
}

func TestBatchAssessmentsAreAtomicAndIdempotent(t *testing.T) {
	ctx := context.Background()
	service, repo, revision := newExtensionFixture(t, "pass", "warning", "blocked")
	cmd := BatchAssessCommand{Metadata: Metadata{DatasetID: "dataset", RequestID: "batch-1", Revision: revision, Role: domain.RoleAnnotator, Actor: "annotator"}, Items: []AssessmentDraft{
		{SampleID: "pass", SignalToNoiseDB: 20, ClippingRatio: .01, SilenceRatio: .2},
		{SampleID: "warning", SignalToNoiseDB: 12, ClippingRatio: .01, SilenceRatio: .2},
		{SampleID: "blocked", SignalToNoiseDB: 7, ClippingRatio: .01, SilenceRatio: .2},
	}}
	result, err := service.AssessSignals(ctx, cmd)
	if err != nil {
		t.Fatal(err)
	}
	if result.Dataset.Revision != revision+1 {
		t.Fatalf("批次 revision=%d", result.Dataset.Revision)
	}
	data := result.Data.(map[string]any)
	assessments := data["items"].([]domain.SignalAssessment)
	for i, want := range []domain.SignalOutcome{domain.SignalPass, domain.SignalWarning, domain.SignalBlocked} {
		if assessments[i].Outcome != want || len(assessments[i].Reasons) == 0 {
			t.Fatalf("assessment[%d]=%+v", i, assessments[i])
		}
	}
	repeated, err := service.AssessSignals(ctx, cmd)
	if err != nil || !repeated.Idempotent || repeated.Dataset.Revision != result.Dataset.Revision {
		t.Fatalf("幂等结果=%+v err=%v", repeated, err)
	}
	snapshot, err := repo.Load(ctx, "dataset")
	if err != nil || len(snapshot.Assessments) != 3 {
		t.Fatalf("assessments=%d err=%v", len(snapshot.Assessments), err)
	}
	_, err = service.AssessSignals(ctx, BatchAssessCommand{Metadata: Metadata{DatasetID: "dataset", RequestID: "batch-invalid", Revision: result.Dataset.Revision, Role: domain.RoleAnnotator, Actor: "annotator"}, Items: []AssessmentDraft{{SampleID: "pass", SignalToNoiseDB: 22, ClippingRatio: .01, SilenceRatio: .2}, {SampleID: "missing", SignalToNoiseDB: 22, ClippingRatio: .01, SilenceRatio: .2}}})
	if !domain.IsCode(err, domain.CodeNotFound) {
		t.Fatalf("expected NOT_FOUND, got %v", err)
	}
	snapshot, _ = repo.Load(ctx, "dataset")
	if len(snapshot.Assessments) != 3 || snapshot.Dataset.Revision != result.Dataset.Revision {
		t.Fatalf("非法批次留下部分写入: assessments=%d revision=%d", len(snapshot.Assessments), snapshot.Dataset.Revision)
	}
	_, err = service.AssessSignals(ctx, BatchAssessCommand{Metadata: Metadata{DatasetID: "dataset", RequestID: "batch-invalid-ratio", Revision: result.Dataset.Revision, Role: domain.RoleAnnotator, Actor: "annotator"}, Items: []AssessmentDraft{{SampleID: "pass", SignalToNoiseDB: 22, ClippingRatio: 1.1, SilenceRatio: .2}, {SampleID: "warning", SignalToNoiseDB: 22, ClippingRatio: .01, SilenceRatio: .2}}})
	if !domain.IsCode(err, domain.CodeInvalid) {
		t.Fatalf("expected INVALID_ARGUMENT, got %v", err)
	}
	snapshot, _ = repo.Load(ctx, "dataset")
	if len(snapshot.Assessments) != 3 || snapshot.Dataset.Revision != result.Dataset.Revision {
		t.Fatal("越界指标批次留下了部分写入")
	}
	rechecked, err := service.AssessSignals(ctx, BatchAssessCommand{Metadata: Metadata{DatasetID: "dataset", RequestID: "batch-recheck", Revision: result.Dataset.Revision, Role: domain.RoleAnnotator, Actor: "annotator"}, Items: []AssessmentDraft{{SampleID: "blocked", SignalToNoiseDB: 24, ClippingRatio: .01, SilenceRatio: .2}}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.SubmitAnnotation(ctx, AnnotateCommand{Metadata: Metadata{DatasetID: "dataset", RequestID: "annotate-blocked", Revision: rechecked.Dataset.Revision, Role: domain.RoleAnnotator, Actor: "annotator"}, SampleID: "blocked", Segments: []domain.Segment{{StartMs: 10, EndMs: 500}}, SpeciesCode: "PASSER", CallType: "contact", Confidence: .9, EvidenceNote: "复验通过"})
	if err != nil {
		t.Fatalf("复验通过后仍无法标注: %v", err)
	}
}

func TestReadinessHistoryAndFrozenManifestQueries(t *testing.T) {
	ctx := context.Background()
	service, _, revision := newExtensionFixture(t, "s1", "s2")
	assessed, err := service.AssessSignals(ctx, BatchAssessCommand{Metadata: Metadata{DatasetID: "dataset", RequestID: "assess", Revision: revision, Role: domain.RoleAnnotator, Actor: "annotator"}, Items: []AssessmentDraft{{SampleID: "s1", SignalToNoiseDB: 20, ClippingRatio: .01, SilenceRatio: .2}, {SampleID: "s2", SignalToNoiseDB: 20, ClippingRatio: .01, SilenceRatio: .2}}})
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.SubmitAnnotation(ctx, AnnotateCommand{Metadata: Metadata{DatasetID: "dataset", RequestID: "annotate-1", Revision: assessed.Dataset.Revision, Role: domain.RoleAnnotator, Actor: "annotator"}, SampleID: "s1", Segments: []domain.Segment{{StartMs: 10, EndMs: 500}}, SpeciesCode: "UNKNOWN", CallType: "contact", Confidence: .5, EvidenceNote: "证据不足"})
	if err != nil {
		t.Fatal(err)
	}
	before, err := service.Timeline(ctx, "dataset", domain.Page{})
	if err != nil {
		t.Fatal(err)
	}
	diagnostic, err := service.FreezeReadiness(ctx, "dataset")
	if err != nil {
		t.Fatal(err)
	}
	if diagnostic.Ready || len(diagnostic.Blockers) != 2 {
		t.Fatalf("diagnostic=%+v", diagnostic)
	}
	after, _ := service.Timeline(ctx, "dataset", domain.Page{})
	view, _ := service.GetDataset(ctx, "dataset")
	if len(after) != len(before) || view.Dataset.Revision != first.Dataset.Revision {
		t.Fatal("就绪度查询修改了聚合或时间线")
	}
	queue, err := service.QueryIssues(ctx, "dataset", IssueFilter{Status: "open", Kind: "low_confidence", Severity: "blocking", SampleID: "s1"}, domain.Page{Limit: 1})
	if err != nil || queue.Total != 1 || queue.StatusSummary["open"] != 1 || len(queue.Items) != 1 {
		t.Fatalf("queue=%+v err=%v", queue, err)
	}
	if _, err := service.QueryIssues(ctx, "dataset", IssueFilter{Status: "unknown"}, domain.Page{}); !domain.IsCode(err, domain.CodeInvalid) {
		t.Fatalf("未知筛选未被拒绝: %v", err)
	}
	issue := queue.Items[0]
	decided, err := service.DecideIssue(ctx, DecideCommand{Metadata: Metadata{DatasetID: "dataset", RequestID: "return", Revision: first.Dataset.Revision, Role: domain.RoleExpert, Actor: "expert"}, IssueID: issue.ID, Decision: domain.DecisionReturn, Note: "重新核验物种"})
	if err != nil {
		t.Fatal(err)
	}
	remediated, err := service.SubmitAnnotation(ctx, AnnotateCommand{Metadata: Metadata{DatasetID: "dataset", RequestID: "annotate-2", Revision: decided.Dataset.Revision, Role: domain.RoleAnnotator, Actor: "annotator"}, SampleID: "s1", Segments: []domain.Segment{{StartMs: 20, EndMs: 600}}, SpeciesCode: "PASSER", CallType: "contact", Confidence: .95, EvidenceNote: "补充频谱证据", SupersedesID: issue.AnnotationRevisionID})
	if err != nil {
		t.Fatal(err)
	}
	history, err := service.AnnotationHistory(ctx, "dataset", "s1", domain.Page{Limit: 10})
	if err != nil || history.Total != 2 || history.Items[0].Current || !history.Items[1].Current || history.Items[0].Issues[0].Status != domain.IssueClosed || history.Items[0].Issues[0].ResolutionRevisionID != history.Items[1].ID {
		t.Fatalf("history=%+v err=%v", history, err)
	}
	second, err := service.SubmitAnnotation(ctx, AnnotateCommand{Metadata: Metadata{DatasetID: "dataset", RequestID: "annotate-s2", Revision: remediated.Dataset.Revision, Role: domain.RoleAnnotator, Actor: "annotator"}, SampleID: "s2", Segments: []domain.Segment{{StartMs: 30, EndMs: 700}}, SpeciesCode: "PASSER", CallType: "contact", Confidence: .95, EvidenceNote: "证据完整"})
	if err != nil {
		t.Fatal(err)
	}
	ready, err := service.FreezeReadiness(ctx, "dataset")
	if err != nil || !ready.Ready || ready.ManifestDigest == "" || ready.SampleCount != 2 {
		t.Fatalf("ready=%+v err=%v", ready, err)
	}
	frozen, err := service.Freeze(ctx, FreezeCommand{Metadata: Metadata{DatasetID: "dataset", RequestID: "freeze", Revision: second.Dataset.Revision, Role: domain.RoleLead, Actor: "lead"}})
	if err != nil || frozen.Dataset.ManifestDigest != ready.ManifestDigest {
		t.Fatalf("frozen=%+v err=%v", frozen, err)
	}
	page1, err := service.FrozenItems(ctx, "dataset", domain.Page{Limit: 1})
	page2, err2 := service.FrozenItems(ctx, "dataset", domain.Page{Limit: 1, Offset: 1})
	if err != nil || err2 != nil || !page1.DigestValid || page1.Total != 2 || len(page1.Items) != 1 || len(page2.Items) != 1 || page1.Items[0].SampleID == page2.Items[0].SampleID {
		t.Fatalf("pages=%+v %+v err=%v/%v", page1, page2, err, err2)
	}
	revoked, err := service.Revoke(ctx, RevokeCommand{Metadata: Metadata{DatasetID: "dataset", RequestID: "revoke", Revision: frozen.Dataset.Revision, Role: domain.RoleLead, Actor: "lead"}, Reason: "复核撤销清单可见性"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.FrozenItems(ctx, "dataset", domain.Page{}); !domain.IsCode(err, domain.CodePrecondition) {
		t.Fatalf("撤销后仍暴露冻结清单: %v", err)
	}
	frozen, err = service.Freeze(ctx, FreezeCommand{Metadata: Metadata{DatasetID: "dataset", RequestID: "refreeze", Revision: revoked.Dataset.Revision, Role: domain.RoleLead, Actor: "lead"}})
	if err != nil || frozen.Dataset.ManifestDigest != ready.ManifestDigest {
		t.Fatalf("重新冻结=%+v err=%v", frozen, err)
	}
	released, err := service.Approve(ctx, ApproveCommand{Metadata: Metadata{DatasetID: "dataset", RequestID: "approve", Revision: frozen.Dataset.Revision, Role: domain.RoleLead, Actor: "lead"}})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := service.FrozenItems(ctx, "dataset", domain.Page{})
	if err != nil || manifest.Status != domain.StatusReleased || manifest.FrozenRevision != released.Dataset.FrozenRevision {
		t.Fatalf("released manifest=%+v err=%v", manifest, err)
	}
}

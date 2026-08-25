package application

import (
	"context"
	"testing"
	"time"

	"bioacoustic-release-hub/internal/domain"
	"bioacoustic-release-hub/internal/store/sqlite"
)

func TestReturnedIssueRemediationAndRelease(t *testing.T) {
	ctx := context.Background()
	repo, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	service := NewService(repo)
	created, err := service.CreateDataset(ctx, CreateCommand{RequestID: "r1", Role: domain.RoleManager, Actor: "manager", ID: "dataset", Title: "林鸟", ResearchGoal: "建立研究样本", TargetTaxa: []string{"aves"}, RecordingRegion: "华东", QualityRuleVersion: "bio-v1"})
	if err != nil {
		t.Fatal(err)
	}
	revision := created.Dataset.Revision
	registered, err := service.RegisterSamples(ctx, RegisterCommand{Metadata: Metadata{DatasetID: "dataset", RequestID: "r2", Revision: revision, Role: domain.RoleManager, Actor: "manager"}, Samples: []SampleDraft{{ID: "sample", SourceRef: "field/a.wav", CapturedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), LatitudeBand: "N30-N31", LongitudeBand: "E120-E121", SampleRateHz: 48000, Channels: 1, DurationMs: 4000, SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}})
	if err != nil {
		t.Fatal(err)
	}
	revision = registered.Dataset.Revision
	assessed, err := service.AssessSignal(ctx, AssessCommand{Metadata: Metadata{DatasetID: "dataset", RequestID: "r3", Revision: revision, Role: domain.RoleAnnotator, Actor: "annotator"}, SampleID: "sample", SignalToNoiseDB: 20, ClippingRatio: .001, SilenceRatio: .1})
	if err != nil {
		t.Fatal(err)
	}
	revision = assessed.Dataset.Revision
	annotated, err := service.SubmitAnnotation(ctx, AnnotateCommand{Metadata: Metadata{DatasetID: "dataset", RequestID: "r4", Revision: revision, Role: domain.RoleAnnotator, Actor: "annotator"}, SampleID: "sample", Segments: []domain.Segment{{StartMs: 10, EndMs: 2000}}, SpeciesCode: "UNKNOWN", CallType: "contact", Confidence: .5, EvidenceNote: "存在背景噪声"})
	if err != nil {
		t.Fatal(err)
	}
	revision = annotated.Dataset.Revision
	issues, err := service.ListIssues(ctx, "dataset", domain.Page{})
	if err != nil || len(issues) != 1 {
		t.Fatalf("issues=%+v err=%v", issues, err)
	}
	decided, err := service.DecideIssue(ctx, DecideCommand{Metadata: Metadata{DatasetID: "dataset", RequestID: "r5", Revision: revision, Role: domain.RoleExpert, Actor: "expert"}, IssueID: issues[0].ID, Decision: domain.DecisionReturn, Note: "物种证据不足，需重新标注"})
	if err != nil {
		t.Fatal(err)
	}
	revision = decided.Dataset.Revision
	remediated, err := service.SubmitAnnotation(ctx, AnnotateCommand{Metadata: Metadata{DatasetID: "dataset", RequestID: "r6", Revision: revision, Role: domain.RoleAnnotator, Actor: "annotator"}, SampleID: "sample", Segments: []domain.Segment{{StartMs: 20, EndMs: 1900}}, SpeciesCode: "PASSER_MONTANUS", CallType: "contact", Confidence: .94, EvidenceNote: "依据谐波结构完成复核", SupersedesID: issues[0].AnnotationRevisionID})
	if err != nil {
		t.Fatal(err)
	}
	if remediated.Dataset.Status != domain.StatusReady {
		t.Fatalf("status=%s", remediated.Dataset.Status)
	}
	revision = remediated.Dataset.Revision
	frozen, err := service.Freeze(ctx, FreezeCommand{Metadata: Metadata{DatasetID: "dataset", RequestID: "r7", Revision: revision, Role: domain.RoleLead, Actor: "lead"}})
	if err != nil {
		t.Fatal(err)
	}
	released, err := service.Approve(ctx, ApproveCommand{Metadata: Metadata{DatasetID: "dataset", RequestID: "r8", Revision: frozen.Dataset.Revision, Role: domain.RoleLead, Actor: "lead"}})
	if err != nil || released.Dataset.Status != domain.StatusReleased {
		t.Fatalf("released=%+v err=%v", released, err)
	}
	credential, err := service.GetCredential(ctx, "dataset")
	if err != nil || !domain.VerifyCredential(credential) {
		t.Fatalf("credential=%+v err=%v", credential, err)
	}
	timeline, err := service.Timeline(ctx, "dataset", domain.Page{})
	if err != nil || len(timeline) != 8 {
		t.Fatalf("timeline count=%d err=%v", len(timeline), err)
	}
}

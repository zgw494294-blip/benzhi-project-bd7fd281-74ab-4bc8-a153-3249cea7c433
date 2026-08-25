package domain

import (
	"math"
	"testing"
	"time"
)

func TestQualityAndCredential(t *testing.T) {
	a, err := AssessSignal(AssessSignalInput{ID: "a", DatasetID: "d", SampleID: "s", RuleVersion: "bio-v1", SignalToNoiseDB: 20, ClippingRatio: 0.01, SilenceRatio: 0.2, AssessedBy: "u"}, time.Now())
	if err != nil || a.Outcome != SignalPass {
		t.Fatalf("assessment=%+v err=%v", a, err)
	}
	d := Dataset{ID: "d", Status: StatusFrozen, FrozenRevision: 3, ManifestDigest: "abc", QualityRuleVersion: "bio-v1"}
	c, err := NewCredential("c", d, 1, "lead", time.Now())
	if err != nil || !VerifyCredential(c) {
		t.Fatalf("credential invalid: %v", err)
	}
}

func TestQualityRejectsNonFiniteMetrics(t *testing.T) {
	_, err := AssessSignal(AssessSignalInput{ID: "a", DatasetID: "d", SampleID: "s", RuleVersion: "bio-v1", SignalToNoiseDB: math.NaN(), ClippingRatio: 0.01, SilenceRatio: 0.2, AssessedBy: "u"}, time.Now())
	if !IsCode(err, CodeInvalid) {
		t.Fatalf("expected invalid, got %v", err)
	}
}

func TestAnnotationRejectsOverlap(t *testing.T) {
	sample := RecordingSample{ID: "s", DatasetID: "d", DurationMs: 1000}
	_, err := NewAnnotation(AnnotationInput{ID: "a", DatasetID: "d", SampleID: "s", RevisionNo: 1, Segments: []Segment{{StartMs: 1, EndMs: 500}, {StartMs: 400, EndMs: 700}}, SpeciesCode: "x", CallType: "x", Confidence: .9, EvidenceNote: "x"}, sample, time.Now())
	if !IsCode(err, CodeConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
}

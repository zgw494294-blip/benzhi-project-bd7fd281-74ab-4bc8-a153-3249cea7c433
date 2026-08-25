package application

import (
	"time"

	"bioacoustic-release-hub/internal/domain"
)

type Metadata struct {
	DatasetID string
	RequestID string
	Revision  int64
	Role      domain.Role
	Actor     string
}

type CreateCommand struct {
	RequestID          string
	Role               domain.Role
	Actor              string
	ID                 string   `json:"id"`
	Title              string   `json:"title"`
	ResearchGoal       string   `json:"researchGoal"`
	TargetTaxa         []string `json:"targetTaxa"`
	RecordingRegion    string   `json:"recordingRegion"`
	QualityRuleVersion string   `json:"qualityRuleVersion"`
}

type SampleDraft struct {
	ID            string    `json:"id"`
	SourceRef     string    `json:"sourceRef"`
	CapturedAt    time.Time `json:"capturedAt"`
	LatitudeBand  string    `json:"latitudeBand"`
	LongitudeBand string    `json:"longitudeBand"`
	SampleRateHz  int       `json:"sampleRateHz"`
	Channels      int       `json:"channels"`
	DurationMs    int64     `json:"durationMs"`
	SHA256        string    `json:"sha256"`
}

type RegisterCommand struct {
	Metadata
	Samples []SampleDraft `json:"samples"`
}
type AssessCommand struct {
	Metadata
	SampleID        string  `json:"sampleId"`
	SignalToNoiseDB float64 `json:"signalToNoiseDb"`
	ClippingRatio   float64 `json:"clippingRatio"`
	SilenceRatio    float64 `json:"silenceRatio"`
}
type AssessmentDraft struct {
	SampleID        string  `json:"sampleId"`
	SignalToNoiseDB float64 `json:"signalToNoiseDb"`
	ClippingRatio   float64 `json:"clippingRatio"`
	SilenceRatio    float64 `json:"silenceRatio"`
}
type BatchAssessCommand struct {
	Metadata
	Items []AssessmentDraft `json:"items"`
}
type AnnotateCommand struct {
	Metadata
	SampleID     string           `json:"sampleId"`
	Segments     []domain.Segment `json:"segments"`
	SpeciesCode  string           `json:"speciesCode"`
	CallType     string           `json:"callType"`
	Confidence   float64          `json:"confidence"`
	EvidenceNote string           `json:"evidenceNote"`
	SupersedesID string           `json:"supersedesId"`
}
type DecideCommand struct {
	Metadata
	IssueID  string               `json:"issueId"`
	Decision domain.IssueDecision `json:"decision"`
	Note     string               `json:"note"`
}
type FreezeCommand struct{ Metadata }
type RevokeCommand struct {
	Metadata
	Reason string `json:"reason"`
}
type ApproveCommand struct{ Metadata }

type Result struct {
	Dataset    domain.Dataset `json:"dataset"`
	Data       any            `json:"data,omitempty"`
	Idempotent bool           `json:"idempotent,omitempty"`
}

type DatasetView struct {
	Dataset         domain.Dataset            `json:"dataset"`
	SampleCount     int                       `json:"sampleCount"`
	AssessmentCount int                       `json:"assessmentCount"`
	AnnotationCount int                       `json:"annotationCount"`
	OpenIssueCount  int                       `json:"openIssueCount"`
	Credential      *domain.ReleaseCredential `json:"credential,omitempty"`
}

type IssueFilter struct {
	Status   string
	Kind     string
	Severity string
	SampleID string
}

type IssueQueue struct {
	Items         []domain.ReviewIssue `json:"items"`
	Total         int                  `json:"total"`
	StatusSummary map[string]int       `json:"statusSummary"`
	KindSummary   map[string]int       `json:"kindSummary"`
}

type AnnotationIssueView struct {
	IssueID              string               `json:"issueId"`
	Status               domain.IssueStatus   `json:"status"`
	ExpertDecision       domain.IssueDecision `json:"expertDecision,omitempty"`
	ResolutionRevisionID string               `json:"resolutionRevisionId,omitempty"`
}

type AnnotationHistoryItem struct {
	domain.AnnotationRevision
	Current bool                  `json:"current"`
	Issues  []AnnotationIssueView `json:"issues"`
}

type AnnotationHistory struct {
	DatasetID string                  `json:"datasetId"`
	Sample    domain.RecordingSample  `json:"sample"`
	Items     []AnnotationHistoryItem `json:"items"`
	Total     int                     `json:"total"`
}

type FrozenItemsView struct {
	DatasetID      string               `json:"datasetId"`
	Status         domain.DatasetStatus `json:"status"`
	Items          []domain.FrozenItem  `json:"items"`
	Total          int                  `json:"total"`
	FrozenRevision int64                `json:"frozenRevision"`
	ManifestDigest string               `json:"manifestDigest"`
	DigestValid    bool                 `json:"digestValid"`
}

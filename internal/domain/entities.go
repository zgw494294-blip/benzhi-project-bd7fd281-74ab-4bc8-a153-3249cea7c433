package domain

import "time"

type DatasetStatus string

const (
	StatusDraft       DatasetStatus = "draft"
	StatusScreened    DatasetStatus = "screened"
	StatusAnnotated   DatasetStatus = "annotated"
	StatusUnderReview DatasetStatus = "under_review"
	StatusReady       DatasetStatus = "ready"
	StatusFrozen      DatasetStatus = "frozen"
	StatusReleased    DatasetStatus = "released"
)

type Dataset struct {
	ID                 string        `json:"id"`
	Title              string        `json:"title"`
	ResearchGoal       string        `json:"researchGoal"`
	TargetTaxa         []string      `json:"targetTaxa"`
	RecordingRegion    string        `json:"recordingRegion"`
	QualityRuleVersion string        `json:"qualityRuleVersion"`
	Status             DatasetStatus `json:"status"`
	Revision           int64         `json:"revision"`
	CreatedBy          string        `json:"createdBy"`
	CreatedAt          time.Time     `json:"createdAt"`
	UpdatedAt          time.Time     `json:"updatedAt"`
	FrozenRevision     int64         `json:"frozenRevision,omitempty"`
	ManifestDigest     string        `json:"manifestDigest,omitempty"`
}

type RecordingSample struct {
	ID            string    `json:"id"`
	DatasetID     string    `json:"datasetId"`
	SourceRef     string    `json:"sourceRef"`
	CapturedAt    time.Time `json:"capturedAt"`
	LatitudeBand  string    `json:"latitudeBand"`
	LongitudeBand string    `json:"longitudeBand"`
	SampleRateHz  int       `json:"sampleRateHz"`
	Channels      int       `json:"channels"`
	DurationMs    int64     `json:"durationMs"`
	SHA256        string    `json:"sha256"`
	RegisteredBy  string    `json:"registeredBy"`
	RegisteredAt  time.Time `json:"registeredAt"`
}

type SignalOutcome string

const (
	SignalPass    SignalOutcome = "pass"
	SignalWarning SignalOutcome = "warning"
	SignalBlocked SignalOutcome = "blocked"
)

type SignalAssessment struct {
	ID              string        `json:"id"`
	DatasetID       string        `json:"datasetId"`
	SampleID        string        `json:"sampleId"`
	RuleVersion     string        `json:"ruleVersion"`
	SignalToNoiseDB float64       `json:"signalToNoiseDb"`
	ClippingRatio   float64       `json:"clippingRatio"`
	SilenceRatio    float64       `json:"silenceRatio"`
	Outcome         SignalOutcome `json:"outcome"`
	Reasons         []string      `json:"reasons"`
	AssessedBy      string        `json:"assessedBy"`
	AssessedAt      time.Time     `json:"assessedAt"`
}

type Segment struct {
	StartMs int64 `json:"startMs"`
	EndMs   int64 `json:"endMs"`
}

type AnnotationRevision struct {
	ID           string    `json:"id"`
	DatasetID    string    `json:"datasetId"`
	SampleID     string    `json:"sampleId"`
	RevisionNo   int       `json:"revisionNo"`
	Segments     []Segment `json:"segments"`
	SpeciesCode  string    `json:"speciesCode"`
	CallType     string    `json:"callType"`
	Confidence   float64   `json:"confidence"`
	EvidenceNote string    `json:"evidenceNote"`
	SupersedesID string    `json:"supersedesId,omitempty"`
	SubmittedBy  string    `json:"submittedBy"`
	SubmittedAt  time.Time `json:"submittedAt"`
}

type IssueStatus string
type IssueDecision string

const (
	IssueOpen        IssueStatus   = "open"
	IssueReturned    IssueStatus   = "returned"
	IssueClosed      IssueStatus   = "closed"
	DecisionConfirm  IssueDecision = "confirm"
	DecisionOverride IssueDecision = "override"
	DecisionReturn   IssueDecision = "return"
)

type ReviewIssue struct {
	ID                   string        `json:"id"`
	DatasetID            string        `json:"datasetId"`
	SampleID             string        `json:"sampleId"`
	AnnotationRevisionID string        `json:"annotationRevisionId"`
	Kind                 string        `json:"kind"`
	Severity             string        `json:"severity"`
	Status               IssueStatus   `json:"status"`
	ExpertDecision       IssueDecision `json:"expertDecision,omitempty"`
	DecisionNote         string        `json:"decisionNote,omitempty"`
	ResolutionRevisionID string        `json:"resolutionRevisionId,omitempty"`
	ReviewedBy           string        `json:"reviewedBy,omitempty"`
	ReviewedAt           *time.Time    `json:"reviewedAt,omitempty"`
}

type FrozenItem struct {
	SampleID             string `json:"sampleId"`
	SHA256               string `json:"sha256"`
	AssessmentID         string `json:"assessmentId"`
	AnnotationRevisionID string `json:"annotationRevisionId"`
}

type ReleaseCredential struct {
	ID                 string    `json:"id"`
	DatasetID          string    `json:"datasetId"`
	FrozenRevision     int64     `json:"frozenRevision"`
	ManifestDigest     string    `json:"manifestDigest"`
	SampleCount        int       `json:"sampleCount"`
	QualityRuleVersion string    `json:"qualityRuleVersion"`
	ApprovedBy         string    `json:"approvedBy"`
	ApprovedAt         time.Time `json:"approvedAt"`
	VerificationCode   string    `json:"verificationCode"`
}

type AuditEvent struct {
	Sequence   int64          `json:"sequence"`
	DatasetID  string         `json:"datasetId"`
	Revision   int64          `json:"revision"`
	EventType  string         `json:"eventType"`
	Actor      string         `json:"actor"`
	RequestID  string         `json:"requestId"`
	Details    map[string]any `json:"details"`
	OccurredAt time.Time      `json:"occurredAt"`
}

package domain

import (
	"context"
	"encoding/json"
)

type Snapshot struct {
	Dataset     Dataset
	Samples     []RecordingSample
	Assessments []SignalAssessment
	Annotations []AnnotationRevision
	Issues      []ReviewIssue
	FrozenItems []FrozenItem
	Credential  *ReleaseCredential
}

type Page struct {
	Limit  int
	Offset int
}

type IssueFilter struct {
	Status   string
	Kind     string
	Severity string
	SampleID string
}

type IssueQueryResult struct {
	Items         []ReviewIssue
	Total         int
	StatusSummary map[string]int
	KindSummary   map[string]int
}

type Repository interface {
	Create(context.Context, Snapshot, AuditEvent, string, json.RawMessage) error
	Load(context.Context, string) (Snapshot, error)
	Commit(context.Context, Snapshot, int64, AuditEvent, string, json.RawMessage) error
	IdempotentResult(context.Context, string, string) (json.RawMessage, bool, error)
	ListSamples(context.Context, string, Page) ([]RecordingSample, error)
	ListIssues(context.Context, string, Page) ([]ReviewIssue, error)
	QueryIssues(context.Context, string, IssueFilter, Page) (IssueQueryResult, error)
	Timeline(context.Context, string, Page) ([]AuditEvent, error)
	FindCredential(context.Context, string) (ReleaseCredential, error)
	VerifyIntegrity(context.Context) error
	Close() error
}

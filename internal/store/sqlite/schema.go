package sqlite

import (
	"context"
	"fmt"
)

const schemaVersion = 1

func (s *Store) migrate(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	statements := []string{
		`CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS datasets (
			id TEXT PRIMARY KEY, title TEXT NOT NULL, research_goal TEXT NOT NULL, target_taxa TEXT NOT NULL,
			recording_region TEXT NOT NULL, quality_rule_version TEXT NOT NULL, status TEXT NOT NULL,
			revision INTEGER NOT NULL, created_by TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL,
			frozen_revision INTEGER NOT NULL DEFAULT 0, manifest_digest TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE IF NOT EXISTS recording_samples (
			id TEXT PRIMARY KEY, dataset_id TEXT NOT NULL REFERENCES datasets(id), source_ref TEXT NOT NULL,
			captured_at TEXT NOT NULL, latitude_band TEXT NOT NULL, longitude_band TEXT NOT NULL,
			sample_rate_hz INTEGER NOT NULL, channels INTEGER NOT NULL, duration_ms INTEGER NOT NULL,
			sha256 TEXT NOT NULL, registered_by TEXT NOT NULL, registered_at TEXT NOT NULL,
			UNIQUE(dataset_id, source_ref), UNIQUE(dataset_id, sha256))`,
		`CREATE INDEX IF NOT EXISTS samples_dataset_order ON recording_samples(dataset_id, registered_at, id)`,
		`CREATE TABLE IF NOT EXISTS signal_assessments (
			id TEXT PRIMARY KEY, dataset_id TEXT NOT NULL REFERENCES datasets(id), sample_id TEXT NOT NULL REFERENCES recording_samples(id),
			rule_version TEXT NOT NULL, snr_db REAL NOT NULL, clipping_ratio REAL NOT NULL, silence_ratio REAL NOT NULL,
			outcome TEXT NOT NULL, reasons TEXT NOT NULL, assessed_by TEXT NOT NULL, assessed_at TEXT NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS assessment_sample_order ON signal_assessments(sample_id, assessed_at, id)`,
		`CREATE TABLE IF NOT EXISTS annotation_revisions (
			id TEXT PRIMARY KEY, dataset_id TEXT NOT NULL REFERENCES datasets(id), sample_id TEXT NOT NULL REFERENCES recording_samples(id),
			revision_no INTEGER NOT NULL, segments TEXT NOT NULL, species_code TEXT NOT NULL, call_type TEXT NOT NULL,
			confidence REAL NOT NULL, evidence_note TEXT NOT NULL, supersedes_id TEXT REFERENCES annotation_revisions(id),
			submitted_by TEXT NOT NULL, submitted_at TEXT NOT NULL, UNIQUE(sample_id, revision_no))`,
		`CREATE TABLE IF NOT EXISTS review_issues (
			id TEXT PRIMARY KEY, dataset_id TEXT NOT NULL REFERENCES datasets(id), sample_id TEXT NOT NULL REFERENCES recording_samples(id),
			annotation_revision_id TEXT NOT NULL REFERENCES annotation_revisions(id), kind TEXT NOT NULL, severity TEXT NOT NULL,
			status TEXT NOT NULL, expert_decision TEXT NOT NULL DEFAULT '', decision_note TEXT NOT NULL DEFAULT '',
			resolution_revision_id TEXT REFERENCES annotation_revisions(id), reviewed_by TEXT NOT NULL DEFAULT '', reviewed_at TEXT)`,
		`CREATE INDEX IF NOT EXISTS issues_dataset_order ON review_issues(dataset_id, status, id)`,
		`CREATE INDEX IF NOT EXISTS issues_dataset_filters ON review_issues(dataset_id, kind, severity, sample_id, id)`,
		`CREATE TABLE IF NOT EXISTS frozen_items (
			dataset_id TEXT NOT NULL REFERENCES datasets(id), sample_id TEXT NOT NULL REFERENCES recording_samples(id),
			sha256 TEXT NOT NULL, assessment_id TEXT NOT NULL REFERENCES signal_assessments(id),
			annotation_revision_id TEXT NOT NULL REFERENCES annotation_revisions(id), PRIMARY KEY(dataset_id, sample_id))`,
		`CREATE TABLE IF NOT EXISTS release_credentials (
			id TEXT PRIMARY KEY, dataset_id TEXT NOT NULL UNIQUE REFERENCES datasets(id), frozen_revision INTEGER NOT NULL,
			manifest_digest TEXT NOT NULL, sample_count INTEGER NOT NULL, quality_rule_version TEXT NOT NULL,
			approved_by TEXT NOT NULL, approved_at TEXT NOT NULL, verification_code TEXT NOT NULL UNIQUE)`,
		`CREATE TABLE IF NOT EXISTS idempotency_results (
			dataset_id TEXT NOT NULL, request_id TEXT NOT NULL, response_json BLOB NOT NULL, committed_at TEXT NOT NULL,
			PRIMARY KEY(dataset_id, request_id))`,
		`CREATE TABLE IF NOT EXISTS audit_events (
			sequence INTEGER PRIMARY KEY AUTOINCREMENT, dataset_id TEXT NOT NULL REFERENCES datasets(id), revision INTEGER NOT NULL,
			event_type TEXT NOT NULL, actor TEXT NOT NULL, request_id TEXT NOT NULL, details_json TEXT NOT NULL, occurred_at TEXT NOT NULL,
			UNIQUE(dataset_id, request_id))`,
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("迁移 schema: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES(?, datetime('now'))`, schemaVersion); err != nil {
		return err
	}
	return tx.Commit()
}

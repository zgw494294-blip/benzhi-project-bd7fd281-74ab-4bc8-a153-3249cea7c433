package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"bioacoustic-release-hub/internal/domain"
)

type queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (s *Store) Load(ctx context.Context, id string) (domain.Snapshot, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.Snapshot{}, err
	}
	defer tx.Rollback()
	snapshot, err := loadSnapshot(ctx, tx, id)
	if err != nil {
		return domain.Snapshot{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Snapshot{}, err
	}
	return snapshot, nil
}

func loadSnapshot(ctx context.Context, q queryer, id string) (domain.Snapshot, error) {
	var snap domain.Snapshot
	var taxa, created, updated string
	d := &snap.Dataset
	err := q.QueryRowContext(ctx, `SELECT id,title,research_goal,target_taxa,recording_region,quality_rule_version,status,revision,created_by,created_at,updated_at,frozen_revision,manifest_digest FROM datasets WHERE id=?`, id).Scan(&d.ID, &d.Title, &d.ResearchGoal, &taxa, &d.RecordingRegion, &d.QualityRuleVersion, &d.Status, &d.Revision, &d.CreatedBy, &created, &updated, &d.FrozenRevision, &d.ManifestDigest)
	if err == sql.ErrNoRows {
		return snap, domain.NewError(domain.CodeNotFound, "数据集 %s 不存在", id)
	}
	if err != nil {
		return snap, err
	}
	if err := decode(taxa, &d.TargetTaxa); err != nil {
		return snap, err
	}
	if d.CreatedAt, err = parseTime(created); err != nil {
		return snap, err
	}
	if d.UpdatedAt, err = parseTime(updated); err != nil {
		return snap, err
	}
	if snap.Samples, err = loadSamples(ctx, q, id, 0, 0); err != nil {
		return snap, err
	}
	if snap.Assessments, err = loadAssessments(ctx, q, id); err != nil {
		return snap, err
	}
	if snap.Annotations, err = loadAnnotations(ctx, q, id); err != nil {
		return snap, err
	}
	if snap.Issues, err = loadIssues(ctx, q, id, 0, 0); err != nil {
		return snap, err
	}
	if snap.FrozenItems, err = loadFrozen(ctx, q, id); err != nil {
		return snap, err
	}
	credential, err := findCredential(ctx, q, id)
	if err == nil {
		snap.Credential = &credential
	} else if !domain.IsCode(err, domain.CodeNotFound) {
		return snap, err
	}
	return snap, nil
}

func normalizePage(page domain.Page) (int, int) {
	limit := page.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	if page.Offset < 0 {
		page.Offset = 0
	}
	return limit, page.Offset
}

func (s *Store) ListSamples(ctx context.Context, id string, page domain.Page) ([]domain.RecordingSample, error) {
	limit, offset := normalizePage(page)
	return loadSamples(ctx, s.db, id, limit, offset)
}

func loadSamples(ctx context.Context, q queryer, id string, limit, offset int) ([]domain.RecordingSample, error) {
	query := `SELECT id,dataset_id,source_ref,captured_at,latitude_band,longitude_band,sample_rate_hz,channels,duration_ms,sha256,registered_by,registered_at FROM recording_samples WHERE dataset_id=? ORDER BY registered_at,id`
	args := []any{id}
	if limit > 0 {
		query += ` LIMIT ? OFFSET ?`
		args = append(args, limit, offset)
	}
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.RecordingSample{}
	for rows.Next() {
		var x domain.RecordingSample
		var captured, registered string
		if err := rows.Scan(&x.ID, &x.DatasetID, &x.SourceRef, &captured, &x.LatitudeBand, &x.LongitudeBand, &x.SampleRateHz, &x.Channels, &x.DurationMs, &x.SHA256, &x.RegisteredBy, &registered); err != nil {
			return nil, err
		}
		x.CapturedAt, err = parseTime(captured)
		if err != nil {
			return nil, err
		}
		x.RegisteredAt, err = parseTime(registered)
		if err != nil {
			return nil, err
		}
		items = append(items, x)
	}
	return items, rows.Err()
}

func loadAssessments(ctx context.Context, q queryer, id string) ([]domain.SignalAssessment, error) {
	rows, err := q.QueryContext(ctx, `SELECT id,dataset_id,sample_id,rule_version,snr_db,clipping_ratio,silence_ratio,outcome,reasons,assessed_by,assessed_at FROM signal_assessments WHERE dataset_id=? ORDER BY assessed_at,id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.SignalAssessment{}
	for rows.Next() {
		var x domain.SignalAssessment
		var reasons, at string
		if err := rows.Scan(&x.ID, &x.DatasetID, &x.SampleID, &x.RuleVersion, &x.SignalToNoiseDB, &x.ClippingRatio, &x.SilenceRatio, &x.Outcome, &reasons, &x.AssessedBy, &at); err != nil {
			return nil, err
		}
		if err := decode(reasons, &x.Reasons); err != nil {
			return nil, err
		}
		x.AssessedAt, err = parseTime(at)
		if err != nil {
			return nil, err
		}
		items = append(items, x)
	}
	return items, rows.Err()
}

func loadAnnotations(ctx context.Context, q queryer, id string) ([]domain.AnnotationRevision, error) {
	rows, err := q.QueryContext(ctx, `SELECT id,dataset_id,sample_id,revision_no,segments,species_code,call_type,confidence,evidence_note,COALESCE(supersedes_id,''),submitted_by,submitted_at FROM annotation_revisions WHERE dataset_id=? ORDER BY sample_id,revision_no`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.AnnotationRevision{}
	for rows.Next() {
		var x domain.AnnotationRevision
		var segments, at string
		if err := rows.Scan(&x.ID, &x.DatasetID, &x.SampleID, &x.RevisionNo, &segments, &x.SpeciesCode, &x.CallType, &x.Confidence, &x.EvidenceNote, &x.SupersedesID, &x.SubmittedBy, &at); err != nil {
			return nil, err
		}
		if err := decode(segments, &x.Segments); err != nil {
			return nil, err
		}
		x.SubmittedAt, err = parseTime(at)
		if err != nil {
			return nil, err
		}
		items = append(items, x)
	}
	return items, rows.Err()
}

func (s *Store) ListIssues(ctx context.Context, id string, page domain.Page) ([]domain.ReviewIssue, error) {
	limit, offset := normalizePage(page)
	return loadIssues(ctx, s.db, id, limit, offset)
}

func loadIssues(ctx context.Context, q queryer, id string, limit, offset int) ([]domain.ReviewIssue, error) {
	query := `SELECT id,dataset_id,sample_id,annotation_revision_id,kind,severity,status,expert_decision,decision_note,COALESCE(resolution_revision_id,''),reviewed_by,reviewed_at FROM review_issues WHERE dataset_id=? ORDER BY id`
	args := []any{id}
	if limit > 0 {
		query += ` LIMIT ? OFFSET ?`
		args = append(args, limit, offset)
	}
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.ReviewIssue{}
	for rows.Next() {
		var x domain.ReviewIssue
		var reviewed sql.NullString
		if err := rows.Scan(&x.ID, &x.DatasetID, &x.SampleID, &x.AnnotationRevisionID, &x.Kind, &x.Severity, &x.Status, &x.ExpertDecision, &x.DecisionNote, &x.ResolutionRevisionID, &x.ReviewedBy, &reviewed); err != nil {
			return nil, err
		}
		if reviewed.Valid {
			t, e := parseTime(reviewed.String)
			if e != nil {
				return nil, e
			}
			x.ReviewedAt = &t
		}
		items = append(items, x)
	}
	return items, rows.Err()
}

func loadFrozen(ctx context.Context, q queryer, id string) ([]domain.FrozenItem, error) {
	rows, err := q.QueryContext(ctx, `SELECT sample_id,sha256,assessment_id,annotation_revision_id FROM frozen_items WHERE dataset_id=? ORDER BY sample_id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.FrozenItem{}
	for rows.Next() {
		var x domain.FrozenItem
		if err := rows.Scan(&x.SampleID, &x.SHA256, &x.AssessmentID, &x.AnnotationRevisionID); err != nil {
			return nil, err
		}
		items = append(items, x)
	}
	return items, rows.Err()
}

func (s *Store) FindCredential(ctx context.Context, id string) (domain.ReleaseCredential, error) {
	return findCredential(ctx, s.db, id)
}

func findCredential(ctx context.Context, q queryer, id string) (domain.ReleaseCredential, error) {
	var c domain.ReleaseCredential
	var at string
	err := q.QueryRowContext(ctx, `SELECT id,dataset_id,frozen_revision,manifest_digest,sample_count,quality_rule_version,approved_by,approved_at,verification_code FROM release_credentials WHERE dataset_id=? OR verification_code=?`, id, id).Scan(&c.ID, &c.DatasetID, &c.FrozenRevision, &c.ManifestDigest, &c.SampleCount, &c.QualityRuleVersion, &c.ApprovedBy, &at, &c.VerificationCode)
	if err == sql.ErrNoRows {
		return c, domain.NewError(domain.CodeNotFound, "放行凭据不存在")
	}
	if err != nil {
		return c, fmt.Errorf("读取凭据: %w", err)
	}
	c.ApprovedAt, err = parseTime(at)
	return c, err
}

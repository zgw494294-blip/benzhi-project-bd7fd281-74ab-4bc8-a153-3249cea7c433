package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"bioacoustic-release-hub/internal/domain"
)

func (s *Store) Create(ctx context.Context, snapshot domain.Snapshot, event domain.AuditEvent, requestID string, response json.RawMessage) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	d := snapshot.Dataset
	_, err = tx.ExecContext(ctx, `INSERT INTO datasets(id,title,research_goal,target_taxa,recording_region,quality_rule_version,status,revision,created_by,created_at,updated_at,frozen_revision,manifest_digest) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`, d.ID, d.Title, d.ResearchGoal, encode(d.TargetTaxa), d.RecordingRegion, d.QualityRuleVersion, d.Status, d.Revision, d.CreatedBy, timeText(d.CreatedAt), timeText(d.UpdatedAt), d.FrozenRevision, d.ManifestDigest)
	if err != nil {
		return mapWriteError(err)
	}
	if err := insertEventAndResult(ctx, tx, event, requestID, response); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Commit(ctx context.Context, snapshot domain.Snapshot, expectedRevision int64, event domain.AuditEvent, requestID string, response json.RawMessage) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	d := snapshot.Dataset
	result, err := tx.ExecContext(ctx, `UPDATE datasets SET title=?,research_goal=?,target_taxa=?,recording_region=?,quality_rule_version=?,status=?,revision=?,updated_at=?,frozen_revision=?,manifest_digest=? WHERE id=? AND revision=?`, d.Title, d.ResearchGoal, encode(d.TargetTaxa), d.RecordingRegion, d.QualityRuleVersion, d.Status, d.Revision, timeText(d.UpdatedAt), d.FrozenRevision, d.ManifestDigest, d.ID, expectedRevision)
	if err != nil {
		return mapWriteError(err)
	}
	changed, _ := result.RowsAffected()
	if changed != 1 {
		return domain.NewError(domain.CodeStaleRevision, "聚合 revision 在提交时已变化")
	}
	if err := persistSnapshot(ctx, tx, snapshot); err != nil {
		return err
	}
	if err := insertEventAndResult(ctx, tx, event, requestID, response); err != nil {
		return err
	}
	return tx.Commit()
}

func persistSnapshot(ctx context.Context, tx *sql.Tx, snapshot domain.Snapshot) error {
	for _, sample := range snapshot.Samples {
		_, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO recording_samples(id,dataset_id,source_ref,captured_at,latitude_band,longitude_band,sample_rate_hz,channels,duration_ms,sha256,registered_by,registered_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, sample.ID, sample.DatasetID, sample.SourceRef, timeText(sample.CapturedAt), sample.LatitudeBand, sample.LongitudeBand, sample.SampleRateHz, sample.Channels, sample.DurationMs, sample.SHA256, sample.RegisteredBy, timeText(sample.RegisteredAt))
		if err != nil {
			return mapWriteError(err)
		}
	}
	for _, assessment := range snapshot.Assessments {
		_, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO signal_assessments(id,dataset_id,sample_id,rule_version,snr_db,clipping_ratio,silence_ratio,outcome,reasons,assessed_by,assessed_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, assessment.ID, assessment.DatasetID, assessment.SampleID, assessment.RuleVersion, assessment.SignalToNoiseDB, assessment.ClippingRatio, assessment.SilenceRatio, assessment.Outcome, encode(assessment.Reasons), assessment.AssessedBy, timeText(assessment.AssessedAt))
		if err != nil {
			return mapWriteError(err)
		}
	}
	for _, annotation := range snapshot.Annotations {
		var supersedes any
		if annotation.SupersedesID != "" {
			supersedes = annotation.SupersedesID
		}
		_, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO annotation_revisions(id,dataset_id,sample_id,revision_no,segments,species_code,call_type,confidence,evidence_note,supersedes_id,submitted_by,submitted_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, annotation.ID, annotation.DatasetID, annotation.SampleID, annotation.RevisionNo, encode(annotation.Segments), annotation.SpeciesCode, annotation.CallType, annotation.Confidence, annotation.EvidenceNote, supersedes, annotation.SubmittedBy, timeText(annotation.SubmittedAt))
		if err != nil {
			return mapWriteError(err)
		}
	}
	for _, issue := range snapshot.Issues {
		var reviewedAt any
		if issue.ReviewedAt != nil {
			reviewedAt = timeText(*issue.ReviewedAt)
		}
		var resolution any
		if issue.ResolutionRevisionID != "" {
			resolution = issue.ResolutionRevisionID
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO review_issues(id,dataset_id,sample_id,annotation_revision_id,kind,severity,status,expert_decision,decision_note,resolution_revision_id,reviewed_by,reviewed_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET status=excluded.status,expert_decision=excluded.expert_decision,decision_note=excluded.decision_note,resolution_revision_id=excluded.resolution_revision_id,reviewed_by=excluded.reviewed_by,reviewed_at=excluded.reviewed_at`, issue.ID, issue.DatasetID, issue.SampleID, issue.AnnotationRevisionID, issue.Kind, issue.Severity, issue.Status, issue.ExpertDecision, issue.DecisionNote, resolution, issue.ReviewedBy, reviewedAt)
		if err != nil {
			return mapWriteError(err)
		}
	}
	if len(snapshot.FrozenItems) > 0 {
		if _, err := tx.ExecContext(ctx, `DELETE FROM frozen_items WHERE dataset_id=?`, snapshot.Dataset.ID); err != nil {
			return err
		}
		for _, item := range snapshot.FrozenItems {
			if _, err := tx.ExecContext(ctx, `INSERT INTO frozen_items(dataset_id,sample_id,sha256,assessment_id,annotation_revision_id) VALUES(?,?,?,?,?)`, snapshot.Dataset.ID, item.SampleID, item.SHA256, item.AssessmentID, item.AnnotationRevisionID); err != nil {
				return mapWriteError(err)
			}
		}
	} else if snapshot.Dataset.Status != domain.StatusReleased {
		if _, err := tx.ExecContext(ctx, `DELETE FROM frozen_items WHERE dataset_id=?`, snapshot.Dataset.ID); err != nil {
			return err
		}
	}
	if snapshot.Credential != nil {
		c := snapshot.Credential
		_, err := tx.ExecContext(ctx, `INSERT INTO release_credentials(id,dataset_id,frozen_revision,manifest_digest,sample_count,quality_rule_version,approved_by,approved_at,verification_code) VALUES(?,?,?,?,?,?,?,?,?)`, c.ID, c.DatasetID, c.FrozenRevision, c.ManifestDigest, c.SampleCount, c.QualityRuleVersion, c.ApprovedBy, timeText(c.ApprovedAt), c.VerificationCode)
		if err != nil {
			return mapWriteError(err)
		}
	}
	return nil
}

func insertEventAndResult(ctx context.Context, tx *sql.Tx, event domain.AuditEvent, requestID string, response json.RawMessage) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO audit_events(dataset_id,revision,event_type,actor,request_id,details_json,occurred_at) VALUES(?,?,?,?,?,?,?)`, event.DatasetID, event.Revision, event.EventType, event.Actor, requestID, encode(event.Details), timeText(event.OccurredAt))
	if err != nil {
		return mapWriteError(err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO idempotency_results(dataset_id,request_id,response_json,committed_at) VALUES(?,?,?,?)`, event.DatasetID, requestID, []byte(response), timeText(event.OccurredAt))
	return mapWriteError(err)
}

func mapWriteError(err error) error {
	if err == nil {
		return nil
	}
	return domain.NewError(domain.CodeConflict, "持久化约束冲突: %v", err)
}

func (s *Store) IdempotentResult(ctx context.Context, datasetID, requestID string) (json.RawMessage, bool, error) {
	var raw []byte
	err := s.db.QueryRowContext(ctx, `SELECT response_json FROM idempotency_results WHERE dataset_id=? AND request_id=?`, datasetID, requestID).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("读取幂等结果: %w", err)
	}
	return json.RawMessage(raw), true, nil
}

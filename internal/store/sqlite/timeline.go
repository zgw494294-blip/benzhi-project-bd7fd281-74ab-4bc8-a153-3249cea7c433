package sqlite

import (
	"context"
	"database/sql"

	"bioacoustic-release-hub/internal/domain"
)

func (s *Store) Timeline(ctx context.Context, id string, page domain.Page) ([]domain.AuditEvent, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err := ensureDatasetExists(ctx, tx, id); err != nil {
		return nil, err
	}
	limit, offset := normalizePage(page)
	rows, err := tx.QueryContext(ctx, `SELECT sequence,dataset_id,revision,event_type,actor,request_id,details_json,occurred_at FROM audit_events WHERE dataset_id=? ORDER BY sequence LIMIT ? OFFSET ?`, id, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.AuditEvent{}
	for rows.Next() {
		var x domain.AuditEvent
		var details, at string
		if err := rows.Scan(&x.Sequence, &x.DatasetID, &x.Revision, &x.EventType, &x.Actor, &x.RequestID, &details, &at); err != nil {
			return nil, err
		}
		if err := decode(details, &x.Details); err != nil {
			return nil, err
		}
		x.OccurredAt, err = parseTime(at)
		if err != nil {
			return nil, err
		}
		items = append(items, x)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return items, nil
}

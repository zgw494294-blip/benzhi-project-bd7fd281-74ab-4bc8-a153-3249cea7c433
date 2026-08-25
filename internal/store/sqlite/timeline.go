package sqlite

import (
	"context"

	"bioacoustic-release-hub/internal/domain"
)

func (s *Store) Timeline(ctx context.Context, id string, page domain.Page) ([]domain.AuditEvent, error) {
	limit, offset := normalizePage(page)
	rows, err := s.db.QueryContext(ctx, `SELECT sequence,dataset_id,revision,event_type,actor,request_id,details_json,occurred_at FROM audit_events WHERE dataset_id=? ORDER BY sequence LIMIT ? OFFSET ?`, id, limit, offset)
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
	return items, rows.Err()
}

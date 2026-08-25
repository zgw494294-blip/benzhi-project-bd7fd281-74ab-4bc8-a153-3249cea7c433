package sqlite

import (
	"context"
	"database/sql"
	"strings"

	"bioacoustic-release-hub/internal/domain"
)

func (s *Store) QueryIssues(ctx context.Context, datasetID string, filter domain.IssueFilter, page domain.Page) (domain.IssueQueryResult, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.IssueQueryResult{}, err
	}
	defer tx.Rollback()
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM datasets WHERE id=?`, datasetID).Scan(&exists); err == sql.ErrNoRows {
		return domain.IssueQueryResult{}, domain.NewError(domain.CodeNotFound, "数据集 %s 不存在", datasetID)
	} else if err != nil {
		return domain.IssueQueryResult{}, err
	}
	var inconsistent int
	err = tx.QueryRowContext(ctx, `SELECT count(*)
		FROM review_issues i
		LEFT JOIN annotation_revisions a ON a.id=i.annotation_revision_id
		LEFT JOIN annotation_revisions r ON r.id=i.resolution_revision_id
		WHERE i.dataset_id=? AND (
			a.id IS NULL OR a.dataset_id<>i.dataset_id OR a.sample_id<>i.sample_id OR
			(i.resolution_revision_id IS NOT NULL AND (r.id IS NULL OR r.dataset_id<>i.dataset_id OR r.sample_id<>i.sample_id)))`, datasetID).Scan(&inconsistent)
	if err != nil {
		return domain.IssueQueryResult{}, err
	}
	if inconsistent != 0 {
		return domain.IssueQueryResult{}, domain.NewError(domain.CodeIntegrity, "检测到 %d 个审查问题引用不一致", inconsistent)
	}
	where := strings.Builder{}
	where.WriteString(` WHERE dataset_id=?`)
	args := []any{datasetID}
	for _, condition := range []struct {
		column string
		value  string
	}{{"status", filter.Status}, {"kind", filter.Kind}, {"severity", filter.Severity}, {"sample_id", filter.SampleID}} {
		if condition.value != "" {
			where.WriteString(" AND ")
			where.WriteString(condition.column)
			where.WriteString("=?")
			args = append(args, condition.value)
		}
	}
	result := domain.IssueQueryResult{Items: []domain.ReviewIssue{}, StatusSummary: map[string]int{"open": 0, "returned": 0, "closed": 0}, KindSummary: map[string]int{}}
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM review_issues`+where.String(), args...).Scan(&result.Total); err != nil {
		return domain.IssueQueryResult{}, err
	}
	if err := readIssueSummary(ctx, tx, `SELECT status,count(*) FROM review_issues WHERE dataset_id=? GROUP BY status`, datasetID, result.StatusSummary); err != nil {
		return domain.IssueQueryResult{}, err
	}
	if err := readIssueSummary(ctx, tx, `SELECT kind,count(*) FROM review_issues WHERE dataset_id=? GROUP BY kind`, datasetID, result.KindSummary); err != nil {
		return domain.IssueQueryResult{}, err
	}
	limit, offset := normalizePage(page)
	query := `SELECT id,dataset_id,sample_id,annotation_revision_id,kind,severity,status,expert_decision,decision_note,COALESCE(resolution_revision_id,''),reviewed_by,reviewed_at FROM review_issues` + where.String() + ` ORDER BY CASE status WHEN 'open' THEN 0 WHEN 'returned' THEN 1 ELSE 2 END,id LIMIT ? OFFSET ?`
	pageArgs := append(append([]any(nil), args...), limit, offset)
	rows, err := tx.QueryContext(ctx, query, pageArgs...)
	if err != nil {
		return domain.IssueQueryResult{}, err
	}
	result.Items, err = scanIssueRows(rows)
	if err != nil {
		return domain.IssueQueryResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.IssueQueryResult{}, err
	}
	return result, nil
}

func readIssueSummary(ctx context.Context, q queryer, query, datasetID string, target map[string]int) error {
	rows, err := q.QueryContext(ctx, query, datasetID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		var count int
		if err := rows.Scan(&key, &count); err != nil {
			return err
		}
		target[key] = count
	}
	return rows.Err()
}

func scanIssueRows(rows *sql.Rows) ([]domain.ReviewIssue, error) {
	defer rows.Close()
	items := []domain.ReviewIssue{}
	for rows.Next() {
		var item domain.ReviewIssue
		var reviewed sql.NullString
		if err := rows.Scan(&item.ID, &item.DatasetID, &item.SampleID, &item.AnnotationRevisionID, &item.Kind, &item.Severity, &item.Status, &item.ExpertDecision, &item.DecisionNote, &item.ResolutionRevisionID, &item.ReviewedBy, &reviewed); err != nil {
			return nil, err
		}
		if reviewed.Valid {
			t, err := parseTime(reviewed.String)
			if err != nil {
				return nil, err
			}
			item.ReviewedAt = &t
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

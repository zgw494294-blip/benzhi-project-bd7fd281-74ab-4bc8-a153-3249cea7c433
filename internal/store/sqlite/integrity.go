package sqlite

import (
	"context"
	"fmt"

	"bioacoustic-release-hub/internal/domain"
)

func (s *Store) VerifyIntegrity(ctx context.Context) error {
	var result string
	if err := s.db.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&result); err != nil {
		return err
	}
	if result != "ok" {
		return domain.NewError(domain.CodeIntegrity, "SQLite integrity_check 失败: %s", result)
	}
	checks := []struct{ name, query string }{{"样本数据集悬空", `SELECT count(*) FROM recording_samples s LEFT JOIN datasets d ON d.id=s.dataset_id WHERE d.id IS NULL`}, {"检查样本悬空", `SELECT count(*) FROM signal_assessments a LEFT JOIN recording_samples s ON s.id=a.sample_id WHERE s.id IS NULL`}, {"标注样本悬空", `SELECT count(*) FROM annotation_revisions a LEFT JOIN recording_samples s ON s.id=a.sample_id WHERE s.id IS NULL`}, {"问题标注悬空", `SELECT count(*) FROM review_issues i LEFT JOIN annotation_revisions a ON a.id=i.annotation_revision_id WHERE a.id IS NULL`}, {"冻结摘要不一致", `SELECT count(*) FROM frozen_items f JOIN recording_samples s ON s.id=f.sample_id WHERE f.sha256<>s.sha256`}}
	for _, check := range checks {
		var count int
		if err := s.db.QueryRowContext(ctx, check.query).Scan(&count); err != nil {
			return fmt.Errorf("执行一致性检查 %s: %w", check.name, err)
		}
		if count != 0 {
			return domain.NewError(domain.CodeIntegrity, "检测到 %d 个%s", count, check.name)
		}
	}
	return nil
}

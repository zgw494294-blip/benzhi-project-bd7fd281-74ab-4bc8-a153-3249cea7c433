package application

import (
	"context"
	"strings"

	"bioacoustic-release-hub/internal/domain"
)

func (s *Service) DecideIssue(ctx context.Context, cmd DecideCommand) (Result, error) {
	if err := domain.RequireRole(cmd.Role, domain.RoleExpert); err != nil {
		return Result{}, err
	}
	return s.run(ctx, cmd.Metadata, func(_ context.Context, snapshot *domain.Snapshot) (Result, string, error) {
		if err := domain.EnsureMutable(snapshot.Dataset); err != nil {
			return Result{}, "", err
		}
		var issue *domain.ReviewIssue
		for i := range snapshot.Issues {
			if snapshot.Issues[i].ID == cmd.IssueID {
				issue = &snapshot.Issues[i]
				break
			}
		}
		if issue == nil {
			return Result{}, "", domain.NewError(domain.CodeNotFound, "审查问题 %s 不存在", cmd.IssueID)
		}
		if err := domain.DecideIssue(issue, cmd.Decision, cmd.Note, cmd.Actor, s.now()); err != nil {
			return Result{}, "", err
		}
		domain.Advance(&snapshot.Dataset, snapshot.Dataset.Status, s.now())
		recompute(snapshot)
		return Result{Dataset: snapshot.Dataset, Data: *issue}, "issue.decided", nil
	})
}

func (s *Service) Freeze(ctx context.Context, cmd FreezeCommand) (Result, error) {
	if err := domain.RequireRole(cmd.Role, domain.RoleLead); err != nil {
		return Result{}, err
	}
	return s.run(ctx, cmd.Metadata, func(_ context.Context, snapshot *domain.Snapshot) (Result, string, error) {
		for _, issue := range snapshot.Issues {
			if issue.Status != domain.IssueClosed {
				return Result{}, "", domain.NewError(domain.CodePrecondition, "仍有未关闭问题 %s", issue.ID)
			}
		}
		items, digest, err := domain.BuildManifest(snapshot.Samples, snapshot.Assessments, snapshot.Annotations)
		if err != nil {
			return Result{}, "", err
		}
		if err := domain.Freeze(&snapshot.Dataset, digest, s.now()); err != nil {
			return Result{}, "", err
		}
		snapshot.FrozenItems = items
		return Result{Dataset: snapshot.Dataset, Data: map[string]any{"manifestDigest": digest, "items": items}}, "candidate.frozen", nil
	})
}

func (s *Service) Revoke(ctx context.Context, cmd RevokeCommand) (Result, error) {
	if err := domain.RequireRole(cmd.Role, domain.RoleLead); err != nil {
		return Result{}, err
	}
	if strings.TrimSpace(cmd.Reason) == "" {
		return Result{}, domain.NewError(domain.CodeInvalid, "撤销原因不能为空")
	}
	return s.run(ctx, cmd.Metadata, func(_ context.Context, snapshot *domain.Snapshot) (Result, string, error) {
		if err := domain.RevokeFreeze(&snapshot.Dataset, s.now()); err != nil {
			return Result{}, "", err
		}
		snapshot.FrozenItems = nil
		return Result{Dataset: snapshot.Dataset, Data: map[string]string{"reason": cmd.Reason}}, "candidate.revoked", nil
	})
}

func (s *Service) Approve(ctx context.Context, cmd ApproveCommand) (Result, error) {
	if err := domain.RequireRole(cmd.Role, domain.RoleLead); err != nil {
		return Result{}, err
	}
	return s.run(ctx, cmd.Metadata, func(_ context.Context, snapshot *domain.Snapshot) (Result, string, error) {
		credential, err := domain.NewCredential(s.newID("credential"), snapshot.Dataset, len(snapshot.Samples), cmd.Actor, s.now())
		if err != nil {
			return Result{}, "", err
		}
		snapshot.Credential = &credential
		domain.Advance(&snapshot.Dataset, domain.StatusReleased, s.now())
		return Result{Dataset: snapshot.Dataset, Data: credential}, "release.approved", nil
	})
}

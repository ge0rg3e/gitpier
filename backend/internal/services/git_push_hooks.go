package services

import (
	"context"
	"strings"

	"gitpier/internal/models"
)

func HandleSuccessfulPush(
	ctx context.Context,
	gitSvc *GitService,
	repoSvc *RepoService,
	workflowSvc *WorkflowService,
	webhookSvc *WebhookService,
	repo *models.Repository,
	owner string,
	repoName string,
	repoPath string,
	baseURL string,
	pusherName string,
	pusherEmail string,
	pusherID string,
	oldRefs map[string]string,
) {
	if repo == nil || gitSvc == nil || repoSvc == nil {
		return
	}

	if lang := gitSvc.GetTopLanguage(repoPath, repo.DefaultBranch); lang != "" {
		_ = repoSvc.UpdateLanguage(ctx, repo.ID, lang)
	}
	_ = repoSvc.UpdateRepoSize(ctx, repo, repoPath)

	if workflowSvc == nil && webhookSvc == nil {
		return
	}

	newRefs, _ := gitSvc.GetAllRefs(repoPath)
	for ref, newSHA := range newRefs {
		if !strings.HasPrefix(ref, "refs/heads/") {
			continue
		}

		branch := strings.TrimPrefix(ref, "refs/heads/")
		oldSHA := oldRefs[ref]
		if oldSHA == newSHA {
			continue
		}

		if workflowSvc != nil {
			_ = workflowSvc.TriggerWorkflows(ctx, repo.ID, owner, repoName, "push", branch, newSHA, "")
		}
		if webhookSvc != nil {
			payload := BuildPushPayload(
				gitSvc, repoPath, ref, oldSHA, newSHA,
				repo, owner, repoName, baseURL,
				pusherName, pusherEmail, pusherID,
			)
			webhookSvc.Deliver(ctx, repo.ID, "push", payload)
		}
	}
}

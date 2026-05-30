package services

import (
	"gitpier/internal/models"
	"strings"
	"time"
)

func isZeroSHA(sha string) bool {
	return sha == "" || strings.TrimLeft(sha, "0") == ""
}

// BuildPushPayload constructs a GitHub-compatible push event payload.
func BuildPushPayload(
	gitSvc *GitService,
	repoPath, ref, oldSHA, newSHA string,
	repo *models.Repository,
	ownerUsername, repoName, baseURL string,
	pusherName, pusherEmail string,
	pusherID string,
) map[string]interface{} {
	commits, _ := gitSvc.GetCommitsBetween(repoPath, oldSHA, newSHA, 20)
	commitPayloads := make([]map[string]interface{}, 0, len(commits))
	for _, ci := range commits {
		added, modified, removed := gitSvc.GetCommitFiles(repoPath, ci.SHA)
		commitPayloads = append(commitPayloads, map[string]interface{}{
			"id":        ci.SHA,
			"distinct":  true,
			"message":   ci.Message,
			"timestamp": ci.Author.Date.Format(time.RFC3339),
			"url":       baseURL + "/" + ownerUsername + "/" + repoName + "/commit/" + ci.SHA,
			"author": map[string]string{
				"name":  ci.Author.Name,
				"email": ci.Author.Email,
			},
			"committer": map[string]string{
				"name":  ci.Author.Name,
				"email": ci.Author.Email,
			},
			"added":    added,
			"modified": modified,
			"removed":  removed,
		})
	}

	var headCommit interface{}
	if len(commitPayloads) > 0 {
		headCommit = commitPayloads[0]
	}

	forced := gitSvc.IsForceUpdate(repoPath, oldSHA, newSHA)

	return map[string]interface{}{
		"ref":         ref,
		"before":      oldSHA,
		"after":       newSHA,
		"created":     isZeroSHA(oldSHA),
		"deleted":     isZeroSHA(newSHA),
		"forced":      forced,
		"base_ref":    nil,
		"compare":     baseURL + "/" + ownerUsername + "/" + repoName + "/compare/" + oldSHA + "..." + newSHA,
		"commits":     commitPayloads,
		"head_commit": headCommit,
		"repository": map[string]interface{}{
			"id":             repo.ID,
			"name":           repoName,
			"full_name":      ownerUsername + "/" + repoName,
			"private":        repo.IsPrivate,
			"description":    repo.Description,
			"default_branch": repo.DefaultBranch,
			"html_url":       baseURL + "/" + ownerUsername + "/" + repoName,
			"url":            baseURL + "/" + ownerUsername + "/" + repoName,
			"owner": map[string]interface{}{
				"name":  ownerUsername,
				"login": ownerUsername,
			},
		},
		"pusher": map[string]string{
			"name":  pusherName,
			"email": pusherEmail,
		},
		"sender": map[string]interface{}{
			"login": pusherName,
			"id":    pusherID,
			"type":  "User",
		},
	}
}

package handlers

import (
	"context"
	"strings"
	"time"

	"gitpier/internal/services"
)

func enrichCommitInfoAuthor(ctx context.Context, authSvc *services.AuthService, commit *services.CommitInfo) {
	if commit == nil {
		return
	}
	enrichCommitAuthors(ctx, authSvc, []*services.CommitInfo{commit})
}

func enrichCommitDetailAuthor(ctx context.Context, authSvc *services.AuthService, detail *services.CommitDetail) {
	if detail == nil || detail.CommitInfo == nil {
		return
	}
	enrichCommitInfoAuthor(ctx, authSvc, detail.CommitInfo)
}

func enrichCommitAuthors(ctx context.Context, authSvc *services.AuthService, commits []*services.CommitInfo) {
	if authSvc == nil || len(commits) == 0 {
		return
	}

	emails := make([]string, 0, len(commits))
	for _, commit := range commits {
		if commit == nil {
			continue
		}
		email := strings.TrimSpace(commit.Author.Email)
		if email == "" {
			continue
		}
		emails = append(emails, email)
	}
	if len(emails) == 0 {
		return
	}

	// Keep commit-author enrichment best-effort and low-latency so repository
	// tree/blob endpoints are not blocked by slow database calls.
	lookupCtx, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()

	usersByEmail, err := authSvc.GetUsersByEmails(lookupCtx, emails)
	if err != nil {
		return
	}

	for _, commit := range commits {
		if commit == nil {
			continue
		}
		user := usersByEmail[strings.ToLower(strings.TrimSpace(commit.Author.Email))]
		if user == nil {
			continue
		}
		commit.Author.Username = user.Username
		commit.Author.AvatarURL = user.AvatarURL
	}
}

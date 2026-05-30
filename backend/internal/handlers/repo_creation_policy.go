package handlers

import (
	"strings"

	"gitpier/internal/config"
	"gitpier/internal/models"
)

const repoCreationRestrictedMessage = "repository creation is restricted on this instance; ask a maintainer for access or fork an existing repository"

func canCreateRepositories(cfg *config.Config, currentUser *models.User) bool {
	if cfg == nil || !cfg.RestrictRepoCreation {
		return true
	}
	if currentUser == nil {
		return false
	}
	if strings.EqualFold(currentUser.Role, models.UserRoleAdmin) {
		return true
	}
	if len(cfg.RepoCreationAllowUsers) == 0 {
		return false
	}
	username := strings.ToLower(strings.TrimSpace(currentUser.Username))
	for _, allowed := range cfg.RepoCreationAllowUsers {
		if username == allowed {
			return true
		}
	}
	return false
}

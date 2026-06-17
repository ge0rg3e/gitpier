package handlers

import "gitpier/internal/models"

// sanitizeUserForPublic removes sensitive account fields from user objects that
// are returned on public endpoints.
func sanitizeUserForPublic(user *models.User) {
	if user == nil {
		return
	}
	user.Email = ""
	user.GDPRConsentAt = nil
	user.ProfileWidgets = ""
}

func sanitizeRepoForPublic(repo *models.Repository) {
	if repo == nil {
		return
	}
	sanitizeUserForPublic(&repo.Owner)
	if repo.ForkedFromRepo != nil {
		sanitizeUserForPublic(&repo.ForkedFromRepo.Owner)
	}
}

func sanitizeReposForPublic(repos []models.Repository) {
	for i := range repos {
		sanitizeRepoForPublic(&repos[i])
	}
}

func sanitizeStarsForPublic(stars []models.Star) {
	for i := range stars {
		sanitizeRepoForPublic(&stars[i].Repo)
	}
}

func sanitizeOrgMembersForPublic(members []models.OrganizationMember) {
	for i := range members {
		sanitizeUserForPublic(&members[i].User)
	}
}

func sanitizeTeamMembersForPublic(members []models.TeamMember) {
	for i := range members {
		sanitizeUserForPublic(&members[i].User)
	}
}

func sanitizeTeamReposForPublic(teamRepos []models.TeamRepository) {
	for i := range teamRepos {
		sanitizeRepoForPublic(&teamRepos[i].Repo)
	}
}

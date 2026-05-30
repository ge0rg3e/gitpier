package services

import (
	"context"
	"errors"
	"fmt"

	"gitpier/internal/models"

	"gorm.io/gorm"
)

var (
	ErrOrgNotFound     = errors.New("organization not found")
	ErrOrgExists       = errors.New("organization name already taken")
	ErrOrgAccessDenied = errors.New("access denied")
	ErrAlreadyMember   = errors.New("user is already a member")
	ErrNotMember       = errors.New("user is not a member")
	ErrTeamNotFound    = errors.New("team not found")
	ErrTeamExists      = errors.New("team already exists")
)

type OrgService struct {
	db *gorm.DB
}

func NewOrgService(db *gorm.DB) *OrgService {
	return &OrgService{db: db}
}

// LoginTaken returns true if a user or org already uses this login.
func (s *OrgService) LoginTaken(login string) bool {
	var count int64
	s.db.Model(&models.User{}).Where("username = ?", login).Count(&count)
	if count > 0 {
		return true
	}
	s.db.Model(&models.Organization{}).Where("login = ?", login).Count(&count)
	return count > 0
}

func (s *OrgService) Create(ctx context.Context, creatorID string, login, displayName, description string) (*models.Organization, error) {
	if s.LoginTaken(login) {
		return nil, ErrOrgExists
	}

	org := &models.Organization{
		Login:       login,
		DisplayName: displayName,
		Description: description,
		IsPublic:    true,
	}
	if err := s.db.WithContext(ctx).Create(org).Error; err != nil {
		return nil, fmt.Errorf("failed to create organization: %w", err)
	}

	// Creator becomes an owner
	member := &models.OrganizationMember{
		OrgID:  org.ID,
		UserID: creatorID,
		Role:   models.OrgRoleOwner,
	}
	if err := s.db.WithContext(ctx).Create(member).Error; err != nil {
		s.db.WithContext(ctx).Delete(org)
		return nil, fmt.Errorf("failed to add creator as owner: %w", err)
	}

	return org, nil
}

func (s *OrgService) GetByLogin(ctx context.Context, login string) (*models.Organization, error) {
	var org models.Organization
	if err := s.db.WithContext(ctx).Where("login = ?", login).First(&org).Error; err != nil {
		return nil, ErrOrgNotFound
	}
	return &org, nil
}

func (s *OrgService) GetByID(ctx context.Context, id string) (*models.Organization, error) {
	var org models.Organization
	if err := s.db.WithContext(ctx).First(&org, id).Error; err != nil {
		return nil, ErrOrgNotFound
	}
	return &org, nil
}

func (s *OrgService) Update(ctx context.Context, org *models.Organization, updates map[string]interface{}) error {
	return s.db.WithContext(ctx).Model(org).Updates(updates).Error
}

func (s *OrgService) Delete(ctx context.Context, org *models.Organization) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Remove team members and team repos first.
		teamIDs := tx.Model(&models.Team{}).Where("org_id = ?", org.ID).Select("id")
		if err := tx.Where("team_id IN (?)", teamIDs).Delete(&models.TeamMember{}).Error; err != nil {
			return err
		}
		if err := tx.Where("team_id IN (?)", teamIDs).Delete(&models.TeamRepository{}).Error; err != nil {
			return err
		}
		if err := tx.Where("org_id = ?", org.ID).Delete(&models.Team{}).Error; err != nil {
			return err
		}

		// Remove org-level follows and membership links.
		if err := tx.Where("org_id = ?", org.ID).Delete(&models.OrganizationMember{}).Error; err != nil {
			return err
		}
		if err := tx.Where("org_id = ?", org.ID).Delete(&models.OrgFollow{}).Error; err != nil {
			return err
		}

		// Finally remove the organization row itself.
		return tx.Delete(org).Error
	})
}

func (s *OrgService) ListByMember(ctx context.Context, userID string) ([]models.Organization, error) {
	var orgs []models.Organization
	err := s.db.WithContext(ctx).
		Joins("JOIN organization_members ON organization_members.org_id = organizations.id").
		Where("organization_members.user_id = ?", userID).
		Find(&orgs).Error
	return orgs, err
}

func (s *OrgService) CountMembers(ctx context.Context, orgID string, count *int64) {
	s.db.WithContext(ctx).Model(&models.OrganizationMember{}).Where("org_id = ?", orgID).Count(count)
}

func (s *OrgService) CountRepos(ctx context.Context, orgID string, count *int64) {
	s.db.WithContext(ctx).Model(&models.Repository{}).Where("org_id = ?", orgID).Count(count)
}

func (s *OrgService) GetMembership(ctx context.Context, orgID, userID string) (*models.OrganizationMember, error) {
	var m models.OrganizationMember
	err := s.db.WithContext(ctx).
		Where("org_id = ? AND user_id = ?", orgID, userID).
		Preload("User").
		First(&m).Error
	if err != nil {
		return nil, ErrNotMember
	}
	return &m, nil
}

func (s *OrgService) IsOwner(ctx context.Context, orgID, userID string) bool {
	var count int64
	s.db.WithContext(ctx).Model(&models.OrganizationMember{}).
		Where("org_id = ? AND user_id = ? AND role = ?", orgID, userID, models.OrgRoleOwner).
		Count(&count)
	return count > 0
}

func (s *OrgService) IsMember(ctx context.Context, orgID, userID string) bool {
	var count int64
	s.db.WithContext(ctx).Model(&models.OrganizationMember{}).
		Where("org_id = ? AND user_id = ?", orgID, userID).
		Count(&count)
	return count > 0
}

func (s *OrgService) AddMember(ctx context.Context, orgID, userID string, role string) error {
	if s.IsMember(ctx, orgID, userID) {
		return ErrAlreadyMember
	}
	if role != models.OrgRoleOwner && role != models.OrgRoleMember {
		role = models.OrgRoleMember
	}
	m := &models.OrganizationMember{
		OrgID:  orgID,
		UserID: userID,
		Role:   role,
	}
	return s.db.WithContext(ctx).Create(m).Error
}

func (s *OrgService) RemoveMember(ctx context.Context, orgID, userID string) error {
	result := s.db.WithContext(ctx).
		Where("org_id = ? AND user_id = ?", orgID, userID).
		Delete(&models.OrganizationMember{})
	if result.RowsAffected == 0 {
		return ErrNotMember
	}
	return result.Error
}

func (s *OrgService) UpdateMemberRole(ctx context.Context, orgID, userID string, role string) error {
	if role != models.OrgRoleOwner && role != models.OrgRoleMember {
		return errors.New("invalid role")
	}
	return s.db.WithContext(ctx).
		Model(&models.OrganizationMember{}).
		Where("org_id = ? AND user_id = ?", orgID, userID).
		Update("role", role).Error
}

func (s *OrgService) ListMembers(ctx context.Context, orgID string) ([]models.OrganizationMember, error) {
	var members []models.OrganizationMember
	err := s.db.WithContext(ctx).
		Where("org_id = ?", orgID).
		Preload("User").
		Find(&members).Error
	return members, err
}

func (s *OrgService) CreateTeam(ctx context.Context, orgID string, name, description, permission string) (*models.Team, error) {
	// Ensure name is unique within the org
	var count int64
	s.db.WithContext(ctx).Model(&models.Team{}).Where("org_id = ? AND name = ?", orgID, name).Count(&count)
	if count > 0 {
		return nil, ErrTeamExists
	}

	if permission != models.PermissionRead && permission != models.PermissionWrite && permission != models.PermissionAdmin {
		permission = models.PermissionRead
	}

	team := &models.Team{
		OrgID:       orgID,
		Name:        name,
		Description: description,
		Permission:  permission,
	}
	if err := s.db.WithContext(ctx).Create(team).Error; err != nil {
		return nil, fmt.Errorf("failed to create team: %w", err)
	}
	return team, nil
}

func (s *OrgService) GetTeam(ctx context.Context, teamID string) (*models.Team, error) {
	var team models.Team
	if err := s.db.WithContext(ctx).First(&team, teamID).Error; err != nil {
		return nil, ErrTeamNotFound
	}
	return &team, nil
}

func (s *OrgService) ListTeams(ctx context.Context, orgID string) ([]models.Team, error) {
	var teams []models.Team
	if err := s.db.WithContext(ctx).Where("org_id = ?", orgID).Find(&teams).Error; err != nil {
		return nil, err
	}
	// Attach counts
	for i := range teams {
		var mc, rc int64
		s.db.WithContext(ctx).Model(&models.TeamMember{}).Where("team_id = ?", teams[i].ID).Count(&mc)
		s.db.WithContext(ctx).Model(&models.TeamRepository{}).Where("team_id = ?", teams[i].ID).Count(&rc)
		teams[i].MemberCount = int(mc)
		teams[i].RepoCount = int(rc)
	}
	return teams, nil
}

func (s *OrgService) UpdateTeam(ctx context.Context, team *models.Team, updates map[string]interface{}) error {
	return s.db.WithContext(ctx).Model(team).Updates(updates).Error
}

func (s *OrgService) DeleteTeam(ctx context.Context, team *models.Team) error {
	s.db.WithContext(ctx).Where("team_id = ?", team.ID).Delete(&models.TeamMember{})
	s.db.WithContext(ctx).Where("team_id = ?", team.ID).Delete(&models.TeamRepository{})
	return s.db.WithContext(ctx).Delete(team).Error
}

func (s *OrgService) AddTeamMember(ctx context.Context, teamID, userID string) error {
	var count int64
	s.db.WithContext(ctx).Model(&models.TeamMember{}).Where("team_id = ? AND user_id = ?", teamID, userID).Count(&count)
	if count > 0 {
		return ErrAlreadyMember
	}
	return s.db.WithContext(ctx).Create(&models.TeamMember{TeamID: teamID, UserID: userID}).Error
}

func (s *OrgService) RemoveTeamMember(ctx context.Context, teamID, userID string) error {
	result := s.db.WithContext(ctx).
		Where("team_id = ? AND user_id = ?", teamID, userID).
		Delete(&models.TeamMember{})
	if result.RowsAffected == 0 {
		return ErrNotMember
	}
	return result.Error
}

func (s *OrgService) ListTeamMembers(ctx context.Context, teamID string) ([]models.TeamMember, error) {
	var members []models.TeamMember
	err := s.db.WithContext(ctx).
		Where("team_id = ?", teamID).
		Preload("User").
		Find(&members).Error
	return members, err
}

func (s *OrgService) AddTeamRepo(ctx context.Context, teamID, repoID string) error {
	var count int64
	s.db.WithContext(ctx).Model(&models.TeamRepository{}).Where("team_id = ? AND repo_id = ?", teamID, repoID).Count(&count)
	if count > 0 {
		return errors.New("repo already in team")
	}
	return s.db.WithContext(ctx).Create(&models.TeamRepository{TeamID: teamID, RepoID: repoID}).Error
}

func (s *OrgService) RemoveTeamRepo(ctx context.Context, teamID, repoID string) error {
	result := s.db.WithContext(ctx).
		Where("team_id = ? AND repo_id = ?", teamID, repoID).
		Delete(&models.TeamRepository{})
	if result.RowsAffected == 0 {
		return errors.New("repo not in team")
	}
	return result.Error
}

func (s *OrgService) ListTeamRepos(ctx context.Context, teamID string) ([]models.TeamRepository, error) {
	var repos []models.TeamRepository
	err := s.db.WithContext(ctx).
		Where("team_id = ?", teamID).
		Preload("Repo.Owner").
		Find(&repos).Error
	return repos, err
}

func (s *OrgService) ListOrgRepos(ctx context.Context, orgID string, includePrivate bool) ([]models.Repository, error) {
	var repos []models.Repository
	q := s.db.WithContext(ctx).Preload("Owner").Where("org_id = ?", orgID)
	if !includePrivate {
		q = q.Where("is_private = false")
	}
	err := q.Order("updated_at DESC").Find(&repos).Error
	return repos, err
}

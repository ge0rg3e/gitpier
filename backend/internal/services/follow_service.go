package services

import (
	"context"
	"errors"

	"gitpier/internal/models"

	"gorm.io/gorm"
)

var (
	ErrCannotFollowSelf = errors.New("cannot follow yourself")
)

type FollowService struct {
	db *gorm.DB
}

func NewFollowService(db *gorm.DB) *FollowService {
	return &FollowService{db: db}
}

func (s *FollowService) FollowUser(ctx context.Context, followerID, followingID string) error {
	if followerID == followingID {
		return ErrCannotFollowSelf
	}
	f := models.UserFollow{FollowerID: followerID, FollowingID: followingID}
	return s.db.WithContext(ctx).Where(models.UserFollow{FollowerID: followerID, FollowingID: followingID}).FirstOrCreate(&f).Error
}

func (s *FollowService) UnfollowUser(ctx context.Context, followerID, followingID string) error {
	return s.db.WithContext(ctx).Where("follower_id = ? AND following_id = ?", followerID, followingID).Delete(&models.UserFollow{}).Error
}

func (s *FollowService) IsFollowingUser(ctx context.Context, followerID, followingID string) bool {
	if followerID == "" || followingID == "" || followerID == followingID {
		return false
	}
	var count int64
	s.db.WithContext(ctx).Model(&models.UserFollow{}).Where("follower_id = ? AND following_id = ?", followerID, followingID).Count(&count)
	return count > 0
}

func (s *FollowService) CountUserFollowers(ctx context.Context, userID string) int64 {
	var count int64
	s.db.WithContext(ctx).Model(&models.UserFollow{}).Where("following_id = ?", userID).Count(&count)
	return count
}

func (s *FollowService) CountUserFollowing(ctx context.Context, userID string) int64 {
	var count int64
	s.db.WithContext(ctx).Model(&models.UserFollow{}).Where("follower_id = ?", userID).Count(&count)
	return count
}

func (s *FollowService) CountOrgFollowing(ctx context.Context, userID string) int64 {
	var count int64
	s.db.WithContext(ctx).Model(&models.OrgFollow{}).Where("user_id = ?", userID).Count(&count)
	return count
}

func (s *FollowService) ListUserFollowers(ctx context.Context, userID string) ([]models.UserFollow, error) {
	var follows []models.UserFollow
	err := s.db.WithContext(ctx).
		Where("following_id = ?", userID).
		Preload("Follower").
		Order("created_at DESC").
		Find(&follows).Error
	return follows, err
}

func (s *FollowService) ListUserFollowing(ctx context.Context, userID string) ([]models.UserFollow, error) {
	var follows []models.UserFollow
	err := s.db.WithContext(ctx).
		Where("follower_id = ?", userID).
		Preload("Following").
		Order("created_at DESC").
		Find(&follows).Error
	return follows, err
}

func (s *FollowService) ListOrgFollowing(ctx context.Context, userID string) ([]models.OrgFollow, error) {
	var follows []models.OrgFollow
	err := s.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Preload("Org").
		Order("created_at DESC").
		Find(&follows).Error
	return follows, err
}

func (s *FollowService) FollowOrg(ctx context.Context, userID, orgID string) error {
	f := models.OrgFollow{UserID: userID, OrgID: orgID}
	return s.db.WithContext(ctx).Where(models.OrgFollow{UserID: userID, OrgID: orgID}).FirstOrCreate(&f).Error
}

func (s *FollowService) UnfollowOrg(ctx context.Context, userID, orgID string) error {
	return s.db.WithContext(ctx).Where("user_id = ? AND org_id = ?", userID, orgID).Delete(&models.OrgFollow{}).Error
}

func (s *FollowService) IsFollowingOrg(ctx context.Context, userID, orgID string) bool {
	if userID == "" || orgID == "" {
		return false
	}
	var count int64
	s.db.WithContext(ctx).Model(&models.OrgFollow{}).Where("user_id = ? AND org_id = ?", userID, orgID).Count(&count)
	return count > 0
}

func (s *FollowService) CountOrgFollowers(ctx context.Context, orgID string) int64 {
	var count int64
	s.db.WithContext(ctx).Model(&models.OrgFollow{}).Where("org_id = ?", orgID).Count(&count)
	return count
}

func (s *FollowService) ListOrgFollowers(ctx context.Context, orgID string) ([]models.OrgFollow, error) {
	var follows []models.OrgFollow
	err := s.db.WithContext(ctx).
		Where("org_id = ?", orgID).
		Preload("User").
		Order("created_at DESC").
		Find(&follows).Error
	return follows, err
}

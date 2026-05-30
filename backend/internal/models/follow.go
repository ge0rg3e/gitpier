package models

import "time"

// UserFollow represents a user following another user.
type UserFollow struct {
	ID        string    `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`

	FollowerID  string `gorm:"not null;uniqueIndex:idx_user_follow_pair" json:"follower_id"`
	FollowingID string `gorm:"not null;uniqueIndex:idx_user_follow_pair" json:"following_id"`

	Follower  User `gorm:"foreignKey:FollowerID;constraint:OnDelete:CASCADE" json:"follower"`
	Following User `gorm:"foreignKey:FollowingID;constraint:OnDelete:CASCADE" json:"following"`
}

// OrgFollow represents a user following an organization.
type OrgFollow struct {
	ID        string    `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`

	UserID string `gorm:"not null;uniqueIndex:idx_org_follow_pair" json:"user_id"`
	OrgID  string `gorm:"not null;uniqueIndex:idx_org_follow_pair" json:"org_id"`

	User User         `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"user"`
	Org  Organization `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE" json:"org"`
}

package models

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	OrgRoleOwner  = "owner"
	OrgRoleMember = "member"
)

type Organization struct {
	ID        string    `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Login       string      `gorm:"uniqueIndex;size:39;not null" json:"login"`
	DisplayName string      `json:"display_name"`
	Description string      `json:"description"`
	AvatarURL   string      `json:"avatar_url"`
	Website     string      `json:"website"`
	SocialLinks SocialLinks `gorm:"type:jsonb;serializer:json" json:"social_links"`
	Location    string      `json:"location"`
	IsPublic    bool        `gorm:"default:true" json:"is_public"`
	IsSuspended bool        `gorm:"not null;default:false" json:"is_suspended"`

	Members []OrganizationMember `gorm:"foreignKey:OrgID" json:"-"`
	Teams   []Team               `gorm:"foreignKey:OrgID" json:"-"`
	Repos   []Repository         `gorm:"foreignKey:OrgID" json:"-"`
}

type SocialLink struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

type SocialLinks []SocialLink

func (s SocialLinks) MarshalJSON() ([]byte, error) {
	if s == nil {
		return []byte("[]"), nil
	}
	return json.Marshal([]SocialLink(s))
}

func (s *SocialLinks) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		*s = SocialLinks{}
		return nil
	}

	// New canonical format: array of links.
	if trimmed[0] == '[' {
		var list []SocialLink
		if err := json.Unmarshal(trimmed, &list); err != nil {
			return err
		}
		*s = SocialLinks(list)
		return nil
	}

	// Backward compatibility: a single object was previously stored by some records.
	if trimmed[0] == '{' {
		var single SocialLink
		if err := json.Unmarshal(trimmed, &single); err != nil {
			return err
		}
		if strings.TrimSpace(single.URL) == "" && strings.TrimSpace(single.Label) == "" {
			*s = SocialLinks{}
			return nil
		}
		*s = SocialLinks{single}
		return nil
	}

	return fmt.Errorf("invalid social_links JSON")
}

func (s SocialLinks) Value() (driver.Value, error) {
	b, err := s.MarshalJSON()
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

type OrganizationMember struct {
	ID        string    `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`

	OrgID  string `gorm:"not null;uniqueIndex:idx_org_user" json:"org_id"`
	UserID string `gorm:"not null;uniqueIndex:idx_org_user" json:"user_id"`
	Role   string `gorm:"not null;default:member" json:"role"` // owner, member

	User User         `json:"user"`
	Org  Organization `gorm:"foreignKey:OrgID" json:"-"`
}

type Team struct {
	ID        string    `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	OrgID       string `gorm:"not null;index" json:"org_id"`
	Name        string `gorm:"size:100;not null" json:"name"`
	Description string `json:"description"`
	Permission  string `gorm:"not null;default:read" json:"permission"` // read, write, admin

	MemberCount int `gorm:"-" json:"member_count,omitempty"`
	RepoCount   int `gorm:"-" json:"repo_count,omitempty"`

	Members []TeamMember     `gorm:"foreignKey:TeamID" json:"-"`
	Repos   []TeamRepository `gorm:"foreignKey:TeamID" json:"-"`
	Org     Organization     `gorm:"foreignKey:OrgID" json:"-"`
}

type TeamMember struct {
	ID        string    `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`

	TeamID string `gorm:"not null;uniqueIndex:idx_team_user" json:"team_id"`
	UserID string `gorm:"not null;uniqueIndex:idx_team_user" json:"user_id"`

	User User `json:"user"`
	Team Team `gorm:"foreignKey:TeamID" json:"-"`
}

type TeamRepository struct {
	ID        string    `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`

	TeamID string `gorm:"not null;uniqueIndex:idx_team_repo" json:"team_id"`
	RepoID string `gorm:"not null;uniqueIndex:idx_team_repo" json:"repo_id"`

	Repo Repository `json:"repo"`
	Team Team       `gorm:"foreignKey:TeamID" json:"-"`
}

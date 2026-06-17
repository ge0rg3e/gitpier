package services

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"gitpier/internal/models"

	"gorm.io/gorm"
)

type SearchService struct {
	db      *gorm.DB
	repoSvc *RepoService
}

func NewSearchService(db *gorm.DB, repoSvc *RepoService) *SearchService {
	return &SearchService{db: db, repoSvc: repoSvc}
}

// CodeMatch represents a single line match from git grep.
type CodeMatch struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Content string `json:"content"`
}

// FileMatch represents a file path match.
type FileMatch struct {
	Path string `json:"path"`
	Type string `json:"type"` // "blob" or "tree"
}

// SearchReposResult holds repository search results.
type SearchReposResult struct {
	Items []models.Repository `json:"items"`
	Total int64               `json:"total"`
}

// SearchUsersResult holds user search results.
type SearchUsersResult struct {
	Items []models.User `json:"items"`
	Total int64         `json:"total"`
}

// SearchOrgsResult holds organization search results.
type SearchOrgsResult struct {
	Items []models.Organization `json:"items"`
	Total int64                 `json:"total"`
}

// SearchCodeResult holds code search results.
type SearchCodeResult struct {
	Items []CodeMatch `json:"items"`
	Total int         `json:"total"`
}

// SearchFilesResult holds file search results.
type SearchFilesResult struct {
	Items []FileMatch `json:"items"`
	Total int         `json:"total"`
}

// SearchRepos searches public repositories by name and description.
func (s *SearchService) SearchRepos(ctx context.Context, query string, limit, offset int) (SearchReposResult, error) {
	var items []models.Repository
	var total int64

	like := "%" + strings.ToLower(query) + "%"

	base := s.db.WithContext(ctx).Model(&models.Repository{}).
		Where("is_private = false AND (LOWER(repositories.name) LIKE ? OR LOWER(repositories.description) LIKE ?)", like, like)

	base.Count(&total)

	err := base.Preload("Owner").
		Order("repositories.updated_at DESC").
		Limit(limit).Offset(offset).
		Find(&items).Error

	return SearchReposResult{Items: items, Total: total}, err
}

// SearchUsers searches users by username and display name.
func (s *SearchService) SearchUsers(ctx context.Context, query string, limit, offset int) (SearchUsersResult, error) {
	var items []models.User
	var total int64

	like := "%" + strings.ToLower(query) + "%"

	base := s.db.WithContext(ctx).Model(&models.User{}).
		Where("LOWER(username) LIKE ? OR LOWER(display_name) LIKE ?", like, like)

	base.Count(&total)

	err := base.Select("id, created_at, updated_at, username, display_name, bio, avatar_url, location, website").
		Order("username ASC").
		Limit(limit).Offset(offset).
		Find(&items).Error

	return SearchUsersResult{Items: items, Total: total}, err
}

// SearchOrgs searches organizations by login and display name.
func (s *SearchService) SearchOrgs(ctx context.Context, query string, limit, offset int) (SearchOrgsResult, error) {
	var items []models.Organization
	var total int64

	like := "%" + strings.ToLower(query) + "%"

	base := s.db.WithContext(ctx).Model(&models.Organization{}).
		Where("is_public = true AND (LOWER(login) LIKE ? OR LOWER(display_name) LIKE ? OR LOWER(description) LIKE ?)", like, like, like)

	base.Count(&total)

	err := base.Order("login ASC").
		Limit(limit).Offset(offset).
		Find(&items).Error

	return SearchOrgsResult{Items: items, Total: total}, err
}

const (
	maxCodeResults = 200
	maxFileResults = 500
)

// SearchCode searches file contents inside a repo using git grep.
// It returns at most maxCodeResults matches.
func (s *SearchService) SearchCode(repoPath, ref, query string) (SearchCodeResult, error) {
	if ref == "" {
		ref = "HEAD"
	}

	// Validate ref to prevent flag injection (e.g. ref starting with '-').
	safeR, err := safeRef(ref)
	if err != nil {
		return SearchCodeResult{}, fmt.Errorf("invalid ref: %w", err)
	}

	// git grep: case-insensitive, line-numbered, text files only, null-separated
	// -l would be faster but we want line previews.
	// Limit output via head to avoid massive allocations on huge repos.
	args := []string{
		"-C", repoPath,
		"grep",
		"-n",         // line numbers
		"-I",         // skip binary files
		"--no-color", // no ANSI
		"-i",         // case-insensitive
		"-e", query,
		safeR,
	}

	cmd := exec.Command("git", args...)
	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		// Exit code 1 means no matches — not an error
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return SearchCodeResult{Items: []CodeMatch{}, Total: 0}, nil
		}
		return SearchCodeResult{}, fmt.Errorf("git grep failed: %w", err)
	}

	var matches []CodeMatch
	scanner := bufio.NewScanner(bytes.NewReader(out.Bytes()))
	for scanner.Scan() {
		line := scanner.Text()
		// Format: <ref>:<path>:<linenum>:<content>
		// Strip the "ref:" prefix first
		after, found := strings.CutPrefix(line, safeR+":")
		if !found {
			after = line
		}

		parts := strings.SplitN(after, ":", 3)
		if len(parts) != 3 {
			continue
		}
		lineNum, err := strconv.Atoi(parts[1])
		if err != nil {
			continue
		}
		content := strings.TrimRight(parts[2], "\r\n")
		// Truncate very long lines
		if len(content) > 512 {
			content = content[:512]
		}

		matches = append(matches, CodeMatch{
			Path:    parts[0],
			Line:    lineNum,
			Content: content,
		})

		if len(matches) >= maxCodeResults {
			break
		}
	}

	if matches == nil {
		matches = []CodeMatch{}
	}
	return SearchCodeResult{Items: matches, Total: len(matches)}, nil
}

// SearchFiles searches file paths inside a repo using git ls-tree.
// It returns at most maxFileResults matches.
func (s *SearchService) SearchFiles(repoPath, ref, query string) (SearchFilesResult, error) {
	if ref == "" {
		ref = "HEAD"
	}

	// Validate ref to prevent flag injection.
	safeR, err := safeRef(ref)
	if err != nil {
		return SearchFilesResult{}, fmt.Errorf("invalid ref: %w", err)
	}

	cmd := exec.Command("git", "-C", repoPath, "ls-tree", "-r", "--name-only", safeR)
	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 128 {
			// Empty repo or bad ref
			return SearchFilesResult{Items: []FileMatch{}, Total: 0}, nil
		}
		return SearchFilesResult{}, fmt.Errorf("git ls-tree failed: %w", err)
	}

	lower := strings.ToLower(query)
	var matches []FileMatch
	scanner := bufio.NewScanner(bytes.NewReader(out.Bytes()))
	for scanner.Scan() {
		path := strings.TrimSpace(scanner.Text())
		if strings.Contains(strings.ToLower(path), lower) {
			matches = append(matches, FileMatch{Path: path, Type: "blob"})
			if len(matches) >= maxFileResults {
				break
			}
		}
	}

	if matches == nil {
		matches = []FileMatch{}
	}
	return SearchFilesResult{Items: matches, Total: len(matches)}, nil
}

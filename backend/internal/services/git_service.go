package services

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
)

// treeCacheEntry holds a cached tree listing keyed by (repoPath, commitHash, dirPath).
// Since the key includes the commit hash, entries are immutable — a new push generates
// a new hash and misses the cache automatically.
type treeCacheEntry struct {
	entries []*FileEntry
	expiry  time.Time
}

type commitCountCacheEntry struct {
	count  int
	expiry time.Time
}

type branchesCacheEntry struct {
	branches []string
	expiry   time.Time
}

type headCommitCacheEntry struct {
	commit *CommitInfo
	expiry time.Time
}

type contributionsCacheEntry struct {
	counts map[string]int
	expiry time.Time
}

type CommitFilters struct {
	Author string
	Query  string
	Since  string
	Until  string
}

var (
	treeCacheMu sync.RWMutex
	treeCache   = make(map[string]treeCacheEntry)

	commitCountCacheMu sync.RWMutex
	commitCountCache   = make(map[string]commitCountCacheEntry)

	branchesCacheMu sync.RWMutex
	branchesCache   = make(map[string]branchesCacheEntry)

	headCommitCacheMu sync.RWMutex
	headCommitCache   = make(map[string]headCommitCacheEntry)

	contributionsCacheMu sync.RWMutex
	contributionsCache   = make(map[string]contributionsCacheEntry)

	contributionsInFlightMu sync.Mutex
	contributionsInFlight   = make(map[string]*contributionsInflight)

	commitCountWarmMu       sync.Mutex
	commitCountWarmInFlight = make(map[string]struct{})
)

type contributionsInflight struct {
	done   chan struct{}
	counts map[string]int
	err    error
}

const (
	treeCacheTTL          = 5 * time.Minute
	treeCacheMaxEntries   = 2000
	commitCountCacheTTL   = 10 * time.Minute
	branchesCacheTTL      = 5 * time.Minute
	headCommitCacheTTL    = 15 * time.Minute
	commitCountTimeout    = 1200 * time.Millisecond
	contributionsCacheTTL = 10 * time.Minute
	contributionsTimeout  = 8 * time.Second
)

func cloneDayCounts(src map[string]int) map[string]int {
	if len(src) == 0 {
		return map[string]int{}
	}
	dst := make(map[string]int, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// treeCacheEvict removes expired entries and, when the cache still exceeds
// treeCacheMaxEntries, evicts arbitrary entries until it is within the limit.
// Must be called with treeCacheMu write-lock held.
func treeCacheEvict() {
	now := time.Now()
	for k, v := range treeCache {
		if now.After(v.expiry) {
			delete(treeCache, k)
		}
	}
	for len(treeCache) >= treeCacheMaxEntries {
		for k := range treeCache {
			delete(treeCache, k)
			break
		}
	}
}

func treeCacheKey(repoPath, head, dirPath string, includeCommitMeta bool) string {
	metaFlag := "0"
	if includeCommitMeta {
		metaFlag = "1"
	}
	return repoPath + "\x00" + head + "\x00" + dirPath + "\x00" + metaFlag
}

var ErrEmptyRepository = errors.New("repository is empty")
var ErrForkHasLocalChanges = errors.New("fork has local commits and cannot be synced automatically")
var ErrGitRepositoryNotFound = errors.New("git repository not found")
var ErrGitReferenceNotFound = errors.New("git reference not found")
var ErrTagAlreadyExists = errors.New("tag already exists")

// ErrInvalidGitParam is returned when a user-supplied git parameter (ref, SHA, path)
// fails basic safety validation.
var ErrInvalidGitParam = errors.New("invalid git parameter")

// safeRef validates a user-supplied git ref (branch/tag name or SHA).
// It rejects values that start with '-' to prevent flag-injection into git
// sub-commands and values containing null bytes or path-separator sequences.
func safeRef(ref string) (string, error) {
	if ref == "" {
		return "", ErrInvalidGitParam
	}
	if strings.HasPrefix(ref, "-") {
		return "", ErrInvalidGitParam
	}
	if strings.ContainsAny(ref, "\x00\r\n") {
		return "", ErrInvalidGitParam
	}
	return ref, nil
}

// safeSHA validates that a string looks like a git SHA (40-char hex or short ≥4-char hex).
func safeSHA(sha string) (string, error) {
	if len(sha) < 4 || len(sha) > 40 {
		return "", ErrInvalidGitParam
	}
	for _, c := range sha {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return "", ErrInvalidGitParam
		}
	}
	return sha, nil
}

// safeFilePath validates a user-supplied file path component used inside a git
// object reference (e.g. "ref:path"). It rejects null bytes, leading dashes,
// and path traversal sequences.
func safeFilePath(p string) (string, error) {
	if strings.ContainsAny(p, "\x00\r\n") {
		return "", ErrInvalidGitParam
	}
	if strings.HasPrefix(p, "-") {
		return "", ErrInvalidGitParam
	}
	// Reject traversal sequences – git would handle them, but reject early.
	clean := filepath.ToSlash(filepath.Clean(p))
	if strings.HasPrefix(clean, "../") || clean == ".." {
		return "", ErrInvalidGitParam
	}
	return p, nil
}

type FileEntry struct {
	Name      string    `json:"name"`
	Type      string    `json:"type"` // "blob" or "tree"
	Path      string    `json:"path"`
	Mode      string    `json:"mode"`
	SHA       string    `json:"sha"`
	CommitSHA string    `json:"commit_sha,omitempty"`
	Message   string    `json:"message,omitempty"`
	Author    string    `json:"author,omitempty"`
	Date      time.Time `json:"date,omitempty"`
}

type CommitInfo struct {
	SHA     string     `json:"sha"`
	Message string     `json:"message"`
	Author  AuthorInfo `json:"author"`
	Files   []string   `json:"files,omitempty"`
	// Aggregate change stats for this commit.
	Additions    int  `json:"additions"`
	Deletions    int  `json:"deletions"`
	ChangedFiles int  `json:"changed_files"`
	WebCommit    bool `json:"web_commit,omitempty"`
}

// FileDiff describes changes to a single file.
type FileDiff struct {
	Path       string `json:"path"`
	OldPath    string `json:"old_path,omitempty"`
	Type       string `json:"type"` // "added", "modified", "deleted", "renamed"
	Additions  int    `json:"additions"`
	Deletions  int    `json:"deletions"`
	Content    string `json:"content,omitempty"`
	OldContent string `json:"old_content,omitempty"`
	Patch      string `json:"patch,omitempty"` // unified diff patch for PR diffs
}

type CommitDetail struct {
	*CommitInfo
	Diffs []FileDiff `json:"diffs"`
}

type AuthorInfo struct {
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Date      time.Time `json:"date"`
	Username  string    `json:"username,omitempty"`
	AvatarURL string    `json:"avatar_url,omitempty"`
}

type RepoStats struct {
	Commits  int    `json:"commits"`
	Branches int    `json:"branches"`
	Tags     int    `json:"tags"`
	Branch   string `json:"branch"`
}

type GitService struct {
	defaultIdentityName  string
	defaultIdentityEmail string
}

func NewGitService(defaultIdentityName, defaultIdentityEmail string) *GitService {
	name := strings.TrimSpace(defaultIdentityName)
	email := strings.TrimSpace(defaultIdentityEmail)
	if name == "" {
		name = "GitPier"
	}
	if email == "" {
		email = "noreply@gitpier.local"
	}
	return &GitService{
		defaultIdentityName:  name,
		defaultIdentityEmail: email,
	}
}

func (s *GitService) InitRepo(repoPath string) error {
	if err := os.MkdirAll(filepath.Dir(repoPath), 0755); err != nil {
		return fmt.Errorf("failed to create repo directory: %w", err)
	}
	_, err := gogit.PlainInit(repoPath, true) // bare repository
	if err != nil {
		return err
	}
	// Set HEAD to main (go-git defaults to master)
	headPath := filepath.Join(repoPath, "HEAD")
	if err := os.WriteFile(headPath, []byte("ref: refs/heads/main\n"), 0644); err != nil {
		return fmt.Errorf("failed to set HEAD: %w", err)
	}
	// Enable git http-backend to allow pushes over HTTP
	configPath := filepath.Join(repoPath, "config")
	f, err := os.OpenFile(configPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil // non-fatal; SSH push still works
	}
	defer f.Close()
	_, _ = f.WriteString("\n[http]\n\treceivepack = true\n")
	return nil
}

// InitializeDefaultBranch creates the initial commit on the provided branch.
// When readmeContent is non-empty, it also creates README.md with that content.
func (s *GitService) InitializeDefaultBranch(repoPath, branch, authorName, authorEmail, readmeContent string) error {
	safeBranch, err := safeRef(branch)
	if err != nil {
		return fmt.Errorf("invalid branch: %w", err)
	}
	if authorName == "" {
		authorName = "Unknown"
	}
	if authorEmail == "" {
		authorEmail = "unknown@gitpier"
	}
	if strings.TrimSpace(readmeContent) == "" {
		// Use plumbing directly on the bare repo to create an initial empty commit
		// and point refs/heads/<branch> to it. This avoids any clone/orphan edge-cases.
		cmd := exec.Command("git", "-C", repoPath, "commit-tree", "4b825dc642cb6eb9a060e54bf8d69288fbee4904", "-m", "Initial commit")
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME="+authorName,
			"GIT_AUTHOR_EMAIL="+authorEmail,
			"GIT_COMMITTER_NAME="+authorName,
			"GIT_COMMITTER_EMAIL="+authorEmail,
		)
		out, err := cmd.Output()
		if err != nil {
			return fmt.Errorf("failed to create initial commit: %w", err)
		}
		sha := strings.TrimSpace(string(out))
		if sha == "" {
			return fmt.Errorf("failed to create initial commit: empty sha")
		}
		ref := "refs/heads/" + safeBranch
		if out, err := exec.Command("git", "-C", repoPath, "update-ref", ref, sha).CombinedOutput(); err != nil {
			return fmt.Errorf("failed to create branch ref: %s", strings.TrimSpace(string(out)))
		}
		if out, err := exec.Command("git", "-C", repoPath, "symbolic-ref", "HEAD", ref).CombinedOutput(); err != nil {
			return fmt.Errorf("failed to set HEAD: %s", strings.TrimSpace(string(out)))
		}
		branchesCacheMu.Lock()
		delete(branchesCache, repoPath)
		branchesCacheMu.Unlock()
		return nil
	}

	tmpDir, err := os.MkdirTemp("", "gitpier-init-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Build the first commit from a fresh non-bare temp repo, then push branch to bare target.
	// This avoids edge-cases cloning an empty bare repository with an unborn HEAD.
	if out, err := exec.Command("git", "-C", tmpDir, "init", "--quiet").CombinedOutput(); err != nil {
		return fmt.Errorf("git init failed: %s", strings.TrimSpace(string(out)))
	}
	if out, err := exec.Command("git", "-C", tmpDir, "checkout", "-b", safeBranch).CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create initial branch: %s", strings.TrimSpace(string(out)))
	}
	_ = exec.Command("git", "-C", tmpDir, "config", "user.name", authorName).Run()
	_ = exec.Command("git", "-C", tmpDir, "config", "user.email", authorEmail).Run()

	if err := os.WriteFile(filepath.Join(tmpDir, "README.md"), []byte(readmeContent), 0644); err != nil {
		return fmt.Errorf("failed to write README: %w", err)
	}
	if out, err := exec.Command("git", "-C", tmpDir, "add", "README.md").CombinedOutput(); err != nil {
		return fmt.Errorf("git add failed: %s", strings.TrimSpace(string(out)))
	}
	if out, err := exec.Command("git", "-C", tmpDir, "commit", "-m", "Initial commit", "--trailer", "GitPier-Web: true").CombinedOutput(); err != nil {
		return fmt.Errorf("git commit failed: %s", strings.TrimSpace(string(out)))
	}
	if out, err := exec.Command("git", "-C", tmpDir, "remote", "add", "origin", repoPath).CombinedOutput(); err != nil {
		return fmt.Errorf("git remote add failed: %s", strings.TrimSpace(string(out)))
	}
	if out, err := exec.Command("git", "-C", tmpDir, "push", "--set-upstream", "origin", safeBranch).CombinedOutput(); err != nil {
		return fmt.Errorf("push failed: %s", strings.TrimSpace(string(out)))
	}
	branchesCacheMu.Lock()
	delete(branchesCache, repoPath)
	branchesCacheMu.Unlock()

	return nil
}

// InitializeWithReadme creates an initial commit on the given branch containing README.md.
func (s *GitService) InitializeWithReadme(repoPath, branch, repoName, authorName, authorEmail string) error {
	return s.InitializeDefaultBranch(repoPath, branch, authorName, authorEmail, fmt.Sprintf("# %s\n", repoName))
}

func (s *GitService) DeleteRepo(repoPath string) error {
	return os.RemoveAll(repoPath)
}

func (s *GitService) RenameRepo(oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}

// FixHEADIfBroken rewrites HEAD to refs/heads/main when it points to a ref
// that doesn't exist (e.g. go-git's default "master" on a repo that only has main).
func (s *GitService) FixHEADIfBroken(repoPath string) error {
	headPath := filepath.Join(repoPath, "HEAD")
	data, err := os.ReadFile(headPath)
	if err != nil {
		return nil // not a bare repo, skip
	}
	headContent := strings.TrimSpace(string(data))
	if !strings.HasPrefix(headContent, "ref: refs/heads/") {
		return nil // detached HEAD or already correct
	}
	ref := strings.TrimPrefix(headContent, "ref: ")
	// Check loose ref file
	if _, err := os.Stat(filepath.Join(repoPath, ref)); err == nil {
		return nil // ref exists
	}
	// Check packed-refs
	if packed, err := os.ReadFile(filepath.Join(repoPath, "packed-refs")); err == nil {
		for _, line := range strings.Split(string(packed), "\n") {
			if strings.HasSuffix(line, " "+ref) {
				return nil // ref exists in packed-refs
			}
		}
	}
	// HEAD is dangling – point it at main
	return os.WriteFile(headPath, []byte("ref: refs/heads/main\n"), 0644)
}

// MigrateHEADs walks reposPath and fixes any bare repos with a broken HEAD.
func (s *GitService) MigrateHEADs(reposPath string) {
	owners, _ := os.ReadDir(reposPath)
	for _, owner := range owners {
		if !owner.IsDir() {
			continue
		}
		repos, _ := os.ReadDir(filepath.Join(reposPath, owner.Name()))
		for _, repo := range repos {
			if !repo.IsDir() || !strings.HasSuffix(repo.Name(), ".git") {
				continue
			}
			repoPath := filepath.Join(reposPath, owner.Name(), repo.Name())
			_ = s.FixHEADIfBroken(repoPath)
		}
	}
}

func (s *GitService) RepoExists(repoPath string) bool {
	_, err := os.Stat(repoPath)
	return err == nil
}

func (s *GitService) CloneBareRepo(sourcePath, targetPath string) error {
	return s.CloneForkRepo(sourcePath, targetPath, "", false)
}

func (s *GitService) CloneForkRepo(sourcePath, targetPath, defaultBranch string, copyDefaultBranchOnly bool) error {
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return fmt.Errorf("failed to create parent directory: %w", err)
	}

	args := []string{"clone", "--bare"}
	if copyDefaultBranchOnly && defaultBranch != "" {
		args = append(args, "--single-branch", "--branch", defaultBranch)
	}
	args = append(args, sourcePath, targetPath)

	if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
		if copyDefaultBranchOnly {
			fallbackOut, fallbackErr := exec.Command("git", "clone", "--bare", sourcePath, targetPath).CombinedOutput()
			if fallbackErr != nil {
				return fmt.Errorf("failed to clone repository: %s", strings.TrimSpace(string(fallbackOut)))
			}
		} else {
			return fmt.Errorf("failed to clone repository: %s", strings.TrimSpace(string(out)))
		}
	}

	if !copyDefaultBranchOnly {
		// Ensure all branches are available as local refs/heads/* in the bare fork.
		// Depending on clone mode/source, only the default branch may be mapped locally.
		fetchArgs := []string{
			"-C", targetPath,
			"fetch", "--prune", sourcePath,
			"+refs/heads/*:refs/heads/*",
			"+refs/tags/*:refs/tags/*",
		}
		if out, err := exec.Command("git", fetchArgs...).CombinedOutput(); err != nil {
			return fmt.Errorf("failed to copy all refs: %s", strings.TrimSpace(string(out)))
		}
	}

	// Keep behavior consistent with repositories created through InitRepo.
	configPath := filepath.Join(targetPath, "config")
	f, err := os.OpenFile(configPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err == nil {
		defer f.Close()
		_, _ = f.WriteString("\n[http]\n\treceivepack = true\n")
	}

	return nil
}

type ForkSyncResult struct {
	Status    string `json:"status"`
	BeforeSHA string `json:"before_sha,omitempty"`
	AfterSHA  string `json:"after_sha,omitempty"`
	Message   string `json:"message"`
}

func (s *GitService) SyncForkDefaultBranch(forkRepoPath, upstreamRepoPath, branch string) (*ForkSyncResult, error) {
	if branch == "" {
		branch = "main"
	}

	safeBranch, err := safeRef(branch)
	if err != nil {
		return nil, ErrInvalidGitParam
	}

	upstreamSyncRef := fmt.Sprintf("refs/gitpier/upstream-sync/%s", safeBranch)
	fetchSpec := fmt.Sprintf("refs/heads/%s:%s", safeBranch, upstreamSyncRef)
	if out, err := exec.Command("git", "-C", forkRepoPath, "fetch", "--no-tags", upstreamRepoPath, fetchSpec).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("failed to fetch upstream branch: %s", strings.TrimSpace(string(out)))
	}

	upstreamSHABytes, err := exec.Command("git", "-C", forkRepoPath, "rev-parse", upstreamSyncRef).Output()
	if err != nil {
		return nil, fmt.Errorf("failed to resolve upstream branch")
	}
	upstreamSHA := strings.TrimSpace(string(upstreamSHABytes))
	localRef := fmt.Sprintf("refs/heads/%s", safeBranch)

	localSHABytes, err := exec.Command("git", "-C", forkRepoPath, "rev-parse", "--verify", localRef).Output()
	if err != nil {
		if out, updateErr := exec.Command("git", "-C", forkRepoPath, "update-ref", localRef, upstreamSHA).CombinedOutput(); updateErr != nil {
			return nil, fmt.Errorf("failed to create local branch: %s", strings.TrimSpace(string(out)))
		}
		return &ForkSyncResult{Status: "synced", AfterSHA: upstreamSHA, Message: "Fork branch created from upstream."}, nil
	}

	localSHA := strings.TrimSpace(string(localSHABytes))
	if localSHA == upstreamSHA {
		return &ForkSyncResult{Status: "up_to_date", BeforeSHA: localSHA, AfterSHA: upstreamSHA, Message: "Fork is already up to date."}, nil
	}

	mergeBaseBytes, err := exec.Command("git", "-C", forkRepoPath, "merge-base", localRef, upstreamSyncRef).Output()
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate fork sync status")
	}

	mergeBase := strings.TrimSpace(string(mergeBaseBytes))
	if mergeBase != localSHA {
		return nil, ErrForkHasLocalChanges
	}

	if out, err := exec.Command("git", "-C", forkRepoPath, "update-ref", localRef, upstreamSHA, localSHA).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("failed to fast-forward fork branch: %s", strings.TrimSpace(string(out)))
	}

	return &ForkSyncResult{Status: "synced", BeforeSHA: localSHA, AfterSHA: upstreamSHA, Message: "Fork synchronized with upstream."}, nil
}

func (s *GitService) ListTree(repoPath, ref, dirPath string, includeCommitMeta bool) ([]*FileEntry, error) {
	if !includeCommitMeta {
		return s.listTreeFast(repoPath, ref, dirPath)
	}

	if ref == "" {
		ref = "HEAD"
	}
	safeR, err := safeRef(ref)
	if err != nil {
		return nil, ErrEmptyRepository
	}

	head, err := resolveRefHashFast(repoPath, safeR)
	if err != nil {
		if errors.Is(err, ErrEmptyRepository) {
			return nil, ErrEmptyRepository
		}
		return nil, err
	}

	// Check cache early (keyed by commit hash so new pushes auto-invalidate)
	cacheKey := treeCacheKey(repoPath, head, dirPath, includeCommitMeta)
	treeCacheMu.RLock()
	if cached, ok := treeCache[cacheKey]; ok && time.Now().Before(cached.expiry) {
		treeCacheMu.RUnlock()
		return cached.entries, nil
	}
	treeCacheMu.RUnlock()

	entries, err := s.listTreeFast(repoPath, safeR, dirPath)
	if err != nil {
		return nil, err
	}

	if includeCommitMeta {
		// Populate per-entry last-commit dates only when explicitly requested.
		fillEntryDates(repoPath, plumbing.NewHash(head), dirPath, entries)
	}

	// Store in cache
	treeCacheMu.Lock()
	if len(treeCache) >= treeCacheMaxEntries {
		treeCacheEvict()
	}
	treeCache[cacheKey] = treeCacheEntry{entries: entries, expiry: time.Now().Add(treeCacheTTL)}
	treeCacheMu.Unlock()

	return entries, nil
}

// listTreeFast lists entries using native git subprocesses.
// This avoids go-git's repository/object loading overhead on large packfiles.
func (s *GitService) listTreeFast(repoPath, ref, dirPath string) ([]*FileEntry, error) {
	if ref == "" {
		ref = "HEAD"
	}
	safeR, err := safeRef(ref)
	if err != nil {
		return nil, ErrEmptyRepository
	}

	head, err := resolveRefHashFast(repoPath, safeR)
	if err != nil {
		if errors.Is(err, ErrEmptyRepository) {
			return nil, ErrEmptyRepository
		}
		return nil, err
	}

	cacheKey := treeCacheKey(repoPath, head, dirPath, false)
	treeCacheMu.RLock()
	if cached, ok := treeCache[cacheKey]; ok && time.Now().Before(cached.expiry) {
		treeCacheMu.RUnlock()
		return cached.entries, nil
	}
	treeCacheMu.RUnlock()

	args := []string{"-C", repoPath, "ls-tree", "-z"}
	if dirPath != "" {
		safeP, err := safeFilePath(dirPath)
		if err != nil {
			return nil, err
		}
		args = append(args, safeR+":"+safeP)
	} else {
		args = append(args, safeR)
	}

	out, err := exec.Command("git", args...).Output()
	if err != nil {
		if dirPath != "" {
			return nil, fmt.Errorf("path not found: %w", err)
		}
		return nil, err
	}

	records := bytes.Split(out, []byte{0})
	entries := make([]*FileEntry, 0, len(records))
	for _, rec := range records {
		if len(rec) == 0 {
			continue
		}
		tab := bytes.IndexByte(rec, '\t')
		if tab <= 0 || tab >= len(rec)-1 {
			continue
		}
		header := strings.Fields(string(rec[:tab]))
		if len(header) < 3 {
			continue
		}

		name := string(rec[tab+1:])
		entryType := "blob"
		if header[1] == "tree" || header[1] == "commit" {
			entryType = "tree"
		}

		fullPath := name
		if dirPath != "" {
			fullPath = dirPath + "/" + name
		}

		entries = append(entries, &FileEntry{
			Name: name,
			Type: entryType,
			Path: fullPath,
			Mode: header[0],
			SHA:  header[2],
		})
	}

	treeCacheMu.Lock()
	if len(treeCache) >= treeCacheMaxEntries {
		treeCacheEvict()
	}
	treeCache[cacheKey] = treeCacheEntry{entries: entries, expiry: time.Now().Add(treeCacheTTL)}
	treeCacheMu.Unlock()

	return entries, nil
}

func resolveRefHashFast(repoPath, ref string) (string, error) {
	out, err := exec.Command("git", "-C", repoPath, "rev-parse", ref+"^{commit}").Output()
	if err != nil {
		return "", ErrEmptyRepository
	}
	hash := strings.TrimSpace(string(out))
	if hash == "" {
		return "", ErrEmptyRepository
	}
	return hash, nil
}

// fillEntryDates populates per-entry last-commit metadata using a single
// streaming "git log --first-parent --name-only" pass.  Walking history once
// (and stopping as soon as every entry has been resolved) is orders of
// magnitude faster than spawning one "git log -1 -- <path>" process per entry
// on repos with deep histories (e.g. thousands of commits).
func fillEntryDates(repoPath string, head plumbing.Hash, dirPath string, entries []*FileEntry) {
	if len(entries) == 0 {
		return
	}

	// Build a name→entry map so we can resolve each path in O(1).
	type slot struct {
		entry    *FileEntry
		resolved bool
	}
	byName := make(map[string]*slot, len(entries))
	for _, e := range entries {
		byName[e.Name] = &slot{entry: e}
	}
	remaining := len(entries)

	// Scope the log to the directory subtree when we're not at the repo root.
	// --first-parent keeps us on the mainline, skipping merge-commit side branches,
	// which dramatically reduces the number of commits visited on busy repos.
	args := []string{
		"-C", repoPath,
		"log", "--first-parent",
		"--pretty=tformat:COMMIT\x1f%H\x1f%aI\x1f%an\x1f%s",
		"--name-only",
		head.String(),
	}
	if dirPath != "" {
		args = append(args, "--", dirPath+"/")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return
	}
	if err := cmd.Start(); err != nil {
		return
	}
	defer cmd.Wait()

	prefix := ""
	if dirPath != "" {
		prefix = dirPath + "/"
	}

	var (
		curSHA    string
		curDate   time.Time
		curAuthor string
		curMsg    string
	)

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 512*1024), 512*1024)
	for scanner.Scan() {
		if remaining == 0 {
			break
		}
		line := scanner.Text()

		if strings.HasPrefix(line, "COMMIT\x1f") {
			// New commit header.
			parts := strings.SplitN(line, "\x1f", 5)
			if len(parts) == 5 {
				curSHA = parts[1]
				t, err := time.Parse(time.RFC3339, parts[2])
				if err == nil {
					curDate = t
				}
				curAuthor = parts[3]
				curMsg = parts[4]
			}
			continue
		}
		if line == "" {
			continue
		}

		// line is a changed file path.
		filePath := line
		if prefix != "" {
			if !strings.HasPrefix(filePath, prefix) {
				continue
			}
			filePath = filePath[len(prefix):]
		}
		// Take only the first path component to match directory entries too.
		if idx := strings.IndexByte(filePath, '/'); idx >= 0 {
			filePath = filePath[:idx]
		}
		if filePath == "" {
			continue
		}

		sl, ok := byName[filePath]
		if !ok || sl.resolved {
			continue
		}
		sl.resolved = true
		sl.entry.CommitSHA = curSHA
		sl.entry.Date = curDate
		sl.entry.Author = curAuthor
		sl.entry.Message = curMsg
		remaining--
	}
}

func (s *GitService) GetBlob(repoPath, ref, filePath string) ([]byte, error) {
	if ref == "" {
		ref = "HEAD"
	}
	safeR, err := safeRef(ref)
	if err != nil {
		return nil, fmt.Errorf("invalid ref: %w", err)
	}
	safeP, err := safeFilePath(filePath)
	if err != nil {
		return nil, fmt.Errorf("invalid path: %w", err)
	}
	// git cat-file blob is significantly faster than go-git on large pack files
	// because it uses git's own object lookup (with pack index) without re-opening the repo.
	out, execErr := exec.Command("git", "-C", repoPath, "cat-file", "blob", safeR+":"+safeP).Output()
	if execErr != nil {
		return nil, fmt.Errorf("file not found: %w", execErr)
	}
	return out, nil
}

func getChangedFiles(c *object.Commit) []string {
	var files []string
	if c.NumParents() == 0 {
		// Initial commit: list all files in tree
		tree, err := c.Tree()
		if err != nil {
			return files
		}
		_ = tree.Files().ForEach(func(f *object.File) error {
			files = append(files, f.Name)
			return nil
		})
	} else {
		parent, err := c.Parents().Next()
		if err != nil {
			return files
		}
		parentTree, err := parent.Tree()
		if err != nil {
			return files
		}
		commitTree, err := c.Tree()
		if err != nil {
			return files
		}
		diffs, err := object.DiffTree(parentTree, commitTree)
		if err != nil {
			return files
		}
		for _, d := range diffs {
			if d.From.Name != "" {
				files = append(files, d.From.Name)
			}
			if d.To.Name != "" {
				files = append(files, d.To.Name)
			}
		}
	}
	return files
}

func (s *GitService) GetCommits(repoPath, ref string, limit, offset int) ([]*CommitInfo, bool, error) {
	return s.GetCommitsFiltered(repoPath, ref, limit, offset, CommitFilters{})
}

func (s *GitService) GetCommitsFiltered(repoPath, ref string, limit, offset int, filters CommitFilters) ([]*CommitInfo, bool, error) {
	if limit <= 0 {
		limit = 30
	}
	if offset < 0 {
		offset = 0
	}
	safeR, err := safeRef(ref)
	if err != nil {
		return []*CommitInfo{}, false, nil
	}
	args := []string{
		"-C", repoPath,
		"log",
	}
	args = append(args, buildCommitFilterArgs(filters)...)
	args = append(args,
		"--format=COMMIT%x1f%H%x1f%aI%x1f%an%x1f%ae%x1f%s%x1f%(trailers:key=GitPier-Web,valueonly=true)",
		"-n", strconv.Itoa(limit+1),
		"--skip", strconv.Itoa(offset),
		safeR,
	)
	out, err := exec.Command("git", args...).Output()
	if err != nil || len(strings.TrimSpace(string(out))) == 0 {
		return []*CommitInfo{}, false, nil
	}

	var commits []*CommitInfo
	for _, line := range strings.Split(strings.ReplaceAll(string(out), "\r\n", "\n"), "\n") {
		if !strings.HasPrefix(line, "COMMIT\x1f") {
			continue
		}
		parts := strings.SplitN(line, "\x1f", 7)
		if len(parts) < 6 {
			continue
		}
		t, _ := time.Parse(time.RFC3339, parts[2])
		webCommit := len(parts) == 7 && strings.TrimSpace(parts[6]) == "true"
		commits = append(commits, &CommitInfo{
			SHA:       parts[1],
			Message:   parts[5],
			WebCommit: webCommit,
			Author: AuthorInfo{
				Name:  parts[3],
				Email: parts[4],
				Date:  t,
			},
		})
	}

	hasMore := len(commits) > limit
	if hasMore {
		commits = commits[:limit]
	}

	return commits, hasMore, nil
}

func (s *GitService) CountCommits(repoPath, ref string) (int, error) {
	return s.CountCommitsFiltered(repoPath, ref, CommitFilters{})
}

func (s *GitService) CountCommitsFiltered(repoPath, ref string, filters CommitFilters) (int, error) {
	safeR, err := safeRef(ref)
	if err != nil {
		return 0, nil
	}

	// Resolve to a concrete hash so the cache key is stable across equivalent refs.
	head, err := resolveRefHashFast(repoPath, safeR)
	if err != nil {
		head = safeR // fall back to the ref name itself
	}

	cacheKey := repoPath + "\x00" + head + "\x00" + filters.cacheKey()
	commitCountCacheMu.RLock()
	if cached, ok := commitCountCache[cacheKey]; ok && time.Now().Before(cached.expiry) {
		commitCountCacheMu.RUnlock()
		return cached.count, nil
	}
	commitCountCacheMu.RUnlock()

	args := []string{"-C", repoPath, "rev-list", "--count"}
	args = append(args, buildCommitFilterArgs(filters)...)
	args = append(args, head)
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		return 0, nil
	}

	n, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil || n < 0 {
		return 0, nil
	}

	commitCountCacheMu.Lock()
	commitCountCache[cacheKey] = commitCountCacheEntry{count: n, expiry: time.Now().Add(commitCountCacheTTL)}
	commitCountCacheMu.Unlock()

	return n, nil
}

func (f CommitFilters) cacheKey() string {
	return strings.Join([]string{f.Author, f.Query, f.Since, f.Until}, "\x00")
}

func buildCommitFilterArgs(filters CommitFilters) []string {
	args := make([]string, 0, 5)
	if filters.Author != "" || filters.Query != "" {
		args = append(args, "--regexp-ignore-case")
	}
	if filters.Author != "" {
		args = append(args, "--author="+regexp.QuoteMeta(filters.Author))
	}
	if filters.Query != "" {
		args = append(args, "--fixed-strings", "--grep="+filters.Query)
	}
	if filters.Since != "" {
		args = append(args, "--since="+filters.Since)
	}
	if filters.Until != "" {
		args = append(args, "--until="+filters.Until)
	}
	return args
}

func parseNumStatValue(v string) int {
	v = strings.TrimSpace(v)
	if v == "" || v == "-" {
		return 0
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0
	}
	return n
}

func getCommitNumStatTotals(repoPath, sha string) (additions, deletions, changedFiles int) {
	parentsOut, err := exec.Command("git", "-C", repoPath, "rev-list", "--parents", "-n", "1", sha).Output()
	if err != nil {
		return 0, 0, 0
	}
	fields := strings.Fields(strings.TrimSpace(string(parentsOut)))
	if len(fields) == 0 {
		return 0, 0, 0
	}

	var args []string
	if len(fields) == 1 {
		args = []string{"-C", repoPath, "diff-tree", "--root", "--numstat", "--no-commit-id", "-r", fields[0]}
	} else {
		args = []string{"-C", repoPath, "diff-tree", "--numstat", "--no-commit-id", "-r", fields[1], fields[0]}
	}
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		return 0, 0, 0
	}

	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 3 {
			continue
		}
		additions += parseNumStatValue(parts[0])
		deletions += parseNumStatValue(parts[1])
		changedFiles++
	}
	return additions, deletions, changedFiles
}

func findCommitByPrefix(repo *gogit.Repository, prefix string) (*object.Commit, error) {
	iter, err := repo.References()
	if err != nil {
		return nil, fmt.Errorf("failed to list refs: %w", err)
	}
	var found *object.Commit
	err = iter.ForEach(func(ref *plumbing.Reference) error {
		if ref.Type() != plumbing.HashReference {
			return nil
		}
		commit, err := repo.CommitObject(ref.Hash())
		if err != nil {
			return nil
		}
		if strings.HasPrefix(strings.ToLower(commit.Hash.String()), strings.ToLower(prefix)) {
			found = commit
			return storer.ErrStop
		}
		return nil
	})
	if err != nil && err != storer.ErrStop {
		log.Printf("ref iteration error: %v", err)
	}
	if found != nil {
		return found, nil
	}
	return nil, plumbing.ErrReferenceNotFound
}

func (s *GitService) resolveCommit(repoPath, sha string) (*gogit.Repository, *object.Commit, error) {
	if _, err := safeSHA(sha); err != nil {
		return nil, nil, fmt.Errorf("invalid SHA: %w", err)
	}
	repo, err := gogit.PlainOpen(repoPath)
	if err != nil {
		return nil, nil, err
	}

	var commit *object.Commit
	if len(sha) >= 40 {
		hash := plumbing.NewHash(sha)
		commit, err = repo.CommitObject(hash)
	} else {
		commit, err = findCommitByPrefix(repo, sha)
	}
	if err != nil {
		return nil, nil, err
	}

	return repo, commit, nil
}

func (s *GitService) buildCommitInfo(repoPath string, commit *object.Commit) *CommitInfo {
	adds, dels, files := getCommitNumStatTotals(repoPath, commit.Hash.String())
	return &CommitInfo{
		SHA:     commit.Hash.String(),
		Message: commit.Message,
		Author: AuthorInfo{
			Name:  commit.Author.Name,
			Email: commit.Author.Email,
			Date:  commit.Author.When,
		},
		Additions:    adds,
		Deletions:    dels,
		ChangedFiles: files,
	}
}

func readBlobContent(repo *gogit.Repository, tree *object.Tree, path string) string {
	if tree == nil || path == "" {
		return ""
	}
	entry, err := tree.FindEntry(path)
	if err != nil {
		return ""
	}
	blob, err := repo.BlobObject(entry.Hash)
	if err != nil {
		return ""
	}
	r, err := blob.Reader()
	if err != nil {
		return ""
	}
	defer r.Close()
	b, err := io.ReadAll(r)
	if err != nil {
		return ""
	}
	return string(b)
}

func (s *GitService) GetCommitInfo(repoPath, sha string) (*CommitInfo, error) {
	safe, err := safeSHA(sha)
	if err != nil {
		return nil, err
	}

	out, err := exec.Command("git", "-C", repoPath, "show", "--quiet", "--format=%H%x1f%an%x1f%ae%x1f%aI%x1f%B", safe).Output()
	if err != nil {
		return nil, err
	}

	parts := strings.SplitN(string(out), "\x1f", 5)
	if len(parts) < 5 {
		return nil, errors.New("failed to parse commit info")
	}

	authorWhen, err := time.Parse(time.RFC3339, strings.TrimSpace(parts[3]))
	if err != nil {
		authorWhen = time.Time{}
	}
	resolvedSHA := strings.TrimSpace(parts[0])
	adds, dels, files := getCommitNumStatTotals(repoPath, resolvedSHA)

	return &CommitInfo{
		SHA:     resolvedSHA,
		Message: strings.TrimRight(parts[4], "\n"),
		Author: AuthorInfo{
			Name:  strings.TrimSpace(parts[1]),
			Email: strings.TrimSpace(parts[2]),
			Date:  authorWhen,
		},
		Additions:    adds,
		Deletions:    dels,
		ChangedFiles: files,
	}, nil
}

func (s *GitService) GetCommitDiffs(repoPath, sha string, limit, offset int) ([]FileDiff, int, bool, error) {
	if limit <= 0 {
		limit = 25
	}
	if offset < 0 {
		offset = 0
	}

	safe, err := safeSHA(sha)
	if err != nil {
		return nil, 0, false, err
	}

	fullSHAOut, err := exec.Command("git", "-C", repoPath, "rev-parse", safe+"^{commit}").Output()
	if err != nil {
		return nil, 0, false, err
	}
	fullSHA := strings.TrimSpace(string(fullSHAOut))
	if fullSHA == "" {
		return nil, 0, false, plumbing.ErrReferenceNotFound
	}

	parentsOut, err := exec.Command("git", "-C", repoPath, "rev-list", "--parents", "-n", "1", fullSHA).Output()
	if err != nil {
		return nil, 0, false, err
	}
	fields := strings.Fields(strings.TrimSpace(string(parentsOut)))
	if len(fields) == 0 {
		return nil, 0, false, plumbing.ErrReferenceNotFound
	}

	// Build the diff-tree range argument.
	// For initial commits (no parents) use --root.
	// For merge commits we always diff against the first parent, same as GitHub/GitLab.
	// Using <parent> <sha> explicitly avoids the silent-empty-output bug that
	// `diff-tree -r <sha>` has for merge commits (requires -m/-c to show anything).
	var baseArgs []string
	if len(fields) == 1 {
		baseArgs = []string{"--root", fullSHA}
	} else {
		parentSHA := fields[1]
		baseArgs = []string{parentSHA, fullSHA}
	}

	nameStatusArgs := append([]string{"-C", repoPath, "diff-tree", "--no-commit-id", "--name-status", "-r"}, baseArgs...)
	nameStatusOut, nsErr := exec.Command("git", nameStatusArgs...).Output()
	if nsErr != nil {
		return nil, 0, false, fmt.Errorf("git diff-tree name-status: %w", nsErr)
	}
	numstatArgs := append([]string{"-C", repoPath, "diff-tree", "--no-commit-id", "--numstat", "-r"}, baseArgs...)
	numstatOnly, _ := exec.Command("git", numstatArgs...).Output()

	// Parse name-status
	var allDiffs []FileDiff
	for _, line := range strings.Split(strings.TrimSpace(string(nameStatusOut)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		status := parts[0]
		var diffType, path, oldPath string
		switch {
		case status == "A":
			diffType, path = "added", parts[1]
		case status == "D":
			diffType, path = "deleted", parts[1]
		case strings.HasPrefix(status, "R") && len(parts) >= 3:
			diffType, oldPath, path = "renamed", parts[1], parts[2]
		case strings.HasPrefix(status, "C") && len(parts) >= 3:
			diffType, oldPath, path = "added", parts[1], parts[2]
		default:
			diffType, path = "modified", parts[len(parts)-1]
		}
		allDiffs = append(allDiffs, FileDiff{
			Path:    path,
			OldPath: oldPath,
			Type:    diffType,
		})
	}

	// Parse numstat for per-file addition/deletion counts
	statsMap := make(map[string][2]int) // path -> [adds, dels]
	for _, line := range strings.Split(strings.TrimSpace(string(numstatOnly)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}
		adds := parseNumStatValue(parts[0])
		dels := parseNumStatValue(parts[1])
		statsMap[parts[2]] = [2]int{adds, dels}
	}
	for i := range allDiffs {
		if s, ok := statsMap[allDiffs[i].Path]; ok {
			allDiffs[i].Additions = s[0]
			allDiffs[i].Deletions = s[1]
		}
	}

	total := len(allDiffs)
	if offset >= total {
		return []FileDiff{}, total, false, nil
	}

	end := offset + limit
	if end > total {
		end = total
	}
	paged := append([]FileDiff(nil), allDiffs[offset:end]...)

	// Get the unified diff patch for this page's files only, limiting data sent to the client.
	// git diff-tree -p --root <sha> -- <file1> <file2> ...
	paths := make([]string, 0, len(paged))
	for _, d := range paged {
		if d.Path != "" {
			paths = append(paths, d.Path)
		}
	}
	if len(paths) > 0 {
		patchBaseArgs := append([]string{"-C", repoPath, "diff-tree", "--no-commit-id", "-p", "-U3", "-r"}, baseArgs...)
		patchBaseArgs = append(patchBaseArgs, "--")
		args := append(patchBaseArgs, paths...)
		patchOut, patchErr := exec.Command("git", args...).Output()
		if patchErr == nil {
			patches := splitDiffByFile(string(patchOut))
			for i := range paged {
				key := paged[i].Path
				if p, ok := patches[key]; ok {
					paged[i].Patch = p
				}
			}
		}
	}

	hasMore := end < total
	return paged, total, hasMore, nil
}

func (s *GitService) GetCommitDetail(repoPath, sha string) (*CommitDetail, error) {
	info, err := s.GetCommitInfo(repoPath, sha)
	if err != nil {
		return nil, err
	}
	diffs, _, _, err := s.GetCommitDiffs(repoPath, sha, int(^uint(0)>>1), 0)
	if err != nil {
		return nil, err
	}
	return &CommitDetail{CommitInfo: info, Diffs: diffs}, nil
}

func (s *GitService) GetBranches(repoPath string) ([]string, error) {
	// Check cache first
	branchesCacheMu.RLock()
	if cached, ok := branchesCache[repoPath]; ok && time.Now().Before(cached.expiry) {
		branchesCacheMu.RUnlock()
		return cached.branches, nil
	}
	branchesCacheMu.RUnlock()

	// Fast path: ask git for local heads.
	names, gitErr := getBranchNamesFromGit(repoPath)
	if gitErr != nil {
		// Fallback: parse refs directly. This tolerates repos with broken refs where
		// `git for-each-ref` aborts entirely.
		parsed, parseErr := getBranchNamesFromRefsFS(repoPath)
		if parseErr != nil {
			return nil, fmt.Errorf("failed to list branches: git error: %w; refs parse error: %v", gitErr, parseErr)
		}
		names = parsed
	}

	if len(names) == 0 {
		// Some imported repos may only have remote-tracking refs.
		if remoteNames, remoteErr := getRemoteOriginBranchNamesFromGit(repoPath); remoteErr == nil {
			names = remoteNames
		}
	}
	sort.Strings(names)

	// Store in cache
	branchesCacheMu.Lock()
	branchesCache[repoPath] = branchesCacheEntry{branches: names, expiry: time.Now().Add(branchesCacheTTL)}
	branchesCacheMu.Unlock()

	return names, nil
}

func getBranchNamesFromGit(repoPath string) ([]string, error) {
	cmd := exec.Command("git", "-C", repoPath, "for-each-ref", "--format=%(refname:short)", "refs/heads/")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		return nil, fmt.Errorf("%w: %s", err, msg)
	}
	return parseGitLineList(stdout.String()), nil
}

func getRemoteOriginBranchNamesFromGit(repoPath string) ([]string, error) {
	cmd := exec.Command("git", "-C", repoPath, "for-each-ref", "--format=%(refname:short)", "refs/remotes/origin/")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		return nil, fmt.Errorf("%w: %s", err, msg)
	}

	remoteRefs := parseGitLineList(stdout.String())
	seen := make(map[string]struct{}, len(remoteRefs))
	branches := make([]string, 0, len(remoteRefs))
	for _, ref := range remoteRefs {
		if ref == "origin/HEAD" || !strings.HasPrefix(ref, "origin/") {
			continue
		}
		name := strings.TrimPrefix(ref, "origin/")
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		branches = append(branches, name)
	}
	return branches, nil
}

func parseGitLineList(out string) []string {
	lines := strings.Split(out, "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		result = append(result, line)
	}
	return result
}

func getBranchNamesFromRefsFS(repoPath string) ([]string, error) {
	info, err := os.Stat(repoPath)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("repository path is not a directory: %s", repoPath)
	}

	refsRoot := filepath.Join(repoPath, "refs", "heads")
	seen := make(map[string]struct{})
	branches := make([]string, 0, 128)

	if _, err := os.Stat(refsRoot); err == nil {
		walkErr := filepath.WalkDir(refsRoot, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				return nil
			}

			rel, err := filepath.Rel(refsRoot, path)
			if err != nil {
				return nil
			}
			rel = filepath.ToSlash(strings.TrimSpace(rel))
			if rel == "" || rel == "." {
				return nil
			}
			if _, exists := seen[rel]; exists {
				return nil
			}
			seen[rel] = struct{}{}
			branches = append(branches, rel)
			return nil
		})
		if walkErr != nil {
			return nil, walkErr
		}
	}

	packedPath := filepath.Join(repoPath, "packed-refs")
	file, err := os.Open(packedPath)
	if err == nil {
		defer file.Close()
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "^") {
				continue
			}

			parts := strings.Fields(line)
			if len(parts) < 2 {
				continue
			}
			ref := strings.TrimSpace(parts[1])
			if !strings.HasPrefix(ref, "refs/heads/") {
				continue
			}
			name := strings.TrimPrefix(ref, "refs/heads/")
			if name == "" {
				continue
			}
			if _, exists := seen[name]; exists {
				continue
			}
			seen[name] = struct{}{}
			branches = append(branches, name)
		}
		if scanErr := scanner.Err(); scanErr != nil {
			return nil, scanErr
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	return branches, nil
}

func (s *GitService) GetHeadCommit(repoPath, ref string) (*CommitInfo, error) {
	if ref == "" {
		ref = "HEAD"
	}
	safeR, err := safeRef(ref)
	if err != nil {
		return nil, ErrEmptyRepository
	}

	// Resolve the ref to its commit hash for cache keying
	head, err := resolveRefHashFast(repoPath, safeR)
	if err != nil {
		return nil, ErrEmptyRepository
	}

	// Check cache first
	cacheKey := repoPath + "\x00" + head
	headCommitCacheMu.RLock()
	if cached, ok := headCommitCache[cacheKey]; ok && time.Now().Before(cached.expiry) {
		headCommitCacheMu.RUnlock()
		return cached.commit, nil
	}
	headCommitCacheMu.RUnlock()

	// git log -1 uses commit-graph when available; much faster than go-git on large repos
	out, execErr := exec.Command("git", "-C", repoPath, "log", "-1",
		"--format=%H%x1f%aI%x1f%an%x1f%ae%x1f%s", head).Output()
	if execErr != nil || len(strings.TrimSpace(string(out))) == 0 {
		return nil, ErrEmptyRepository
	}
	parts := strings.SplitN(strings.TrimSpace(string(out)), "\x1f", 5)
	if len(parts) < 5 {
		return nil, ErrEmptyRepository
	}
	t, _ := time.Parse(time.RFC3339, parts[1])
	commit := &CommitInfo{
		SHA:     parts[0],
		Message: parts[4],
		Author: AuthorInfo{
			Name:  parts[2],
			Email: parts[3],
			Date:  t,
		},
	}

	// Store in cache
	headCommitCacheMu.Lock()
	headCommitCache[cacheKey] = headCommitCacheEntry{commit: commit, expiry: time.Now().Add(headCommitCacheTTL)}
	headCommitCacheMu.Unlock()

	return commit, nil
}

func (s *GitService) GetStats(repoPath, ref string) (*RepoStats, error) {
	stats := &RepoStats{Branch: ref}

	// Branch count is provided by the caller (already fetched via GetBranches)
	// to avoid duplicate refs scans on this hot endpoint.

	if ref == "" {
		ref = "HEAD"
	}
	safeR, err := safeRef(ref)
	if err != nil {
		return stats, nil
	}

	head, err := resolveRefHashFast(repoPath, safeR)
	if err != nil {
		return stats, nil
	}

	cacheKey := repoPath + "\x00" + head
	commitCountCacheMu.RLock()
	if cached, ok := commitCountCache[cacheKey]; ok && time.Now().Before(cached.expiry) {
		commitCountCacheMu.RUnlock()
		stats.Commits = cached.count
		return stats, nil
	}
	commitCountCacheMu.RUnlock()

	ctx, cancel := context.WithTimeout(context.Background(), commitCountTimeout)
	countOut, err := exec.CommandContext(ctx, "git", "-C", repoPath, "rev-list", "--count", head).Output()
	cancel()
	if err == nil {
		n, _ := strconv.Atoi(strings.TrimSpace(string(countOut)))
		stats.Commits = n

		commitCountCacheMu.Lock()
		commitCountCache[cacheKey] = commitCountCacheEntry{count: n, expiry: time.Now().Add(commitCountCacheTTL)}
		commitCountCacheMu.Unlock()
		return stats, nil
	}

	// Large repos can take several seconds on first count. Warm cache in background
	// so this hot endpoint stays responsive and exact count appears shortly after.
	commitCountWarmMu.Lock()
	if _, exists := commitCountWarmInFlight[cacheKey]; !exists {
		commitCountWarmInFlight[cacheKey] = struct{}{}
		go func(key, path, resolvedHead string) {
			defer func() {
				commitCountWarmMu.Lock()
				delete(commitCountWarmInFlight, key)
				commitCountWarmMu.Unlock()
			}()

			out, runErr := exec.Command("git", "-C", path, "rev-list", "--count", resolvedHead).Output()
			if runErr != nil {
				return
			}
			n, convErr := strconv.Atoi(strings.TrimSpace(string(out)))
			if convErr != nil {
				return
			}

			commitCountCacheMu.Lock()
			commitCountCache[key] = commitCountCacheEntry{count: n, expiry: time.Now().Add(commitCountCacheTTL)}
			commitCountCacheMu.Unlock()
		}(cacheKey, repoPath, head)
	}
	commitCountWarmMu.Unlock()

	return stats, nil
}

func resolveRef(repo *gogit.Repository, ref string) (*plumbing.Hash, error) {
	// Try as branch name
	branchRef, err := repo.Reference(plumbing.NewBranchReferenceName(ref), true)
	if err == nil {
		h := branchRef.Hash()
		return &h, nil
	}

	// Try as tag
	tagRef, err := repo.Reference(plumbing.NewTagReferenceName(ref), true)
	if err == nil {
		h := tagRef.Hash()
		return &h, nil
	}

	// Try HEAD
	if ref == "HEAD" || ref == "" {
		head, err := repo.Head()
		if err != nil {
			return nil, plumbing.ErrReferenceNotFound
		}
		h := head.Hash()
		return &h, nil
	}

	// Try as commit hash
	hash := plumbing.NewHash(ref)
	if !hash.IsZero() {
		return &hash, nil
	}

	return nil, plumbing.ErrReferenceNotFound
}

// GetContributions returns a "YYYY-MM-DD" → commit count map for the given
// author email across all branches of the repo for commits after `since`.
func (s *GitService) GetContributions(repoPath, authorEmail string, since time.Time) (map[string]int, error) {
	if strings.TrimSpace(repoPath) == "" || strings.TrimSpace(authorEmail) == "" {
		return map[string]int{}, nil
	}

	sinceUTC := since.UTC().Truncate(24 * time.Hour)
	cacheKey := repoPath + "\x00" + strings.ToLower(strings.TrimSpace(authorEmail)) + "\x00" + sinceUTC.Format("2006-01-02")
	now := time.Now().UTC()

	contributionsCacheMu.RLock()
	if cached, ok := contributionsCache[cacheKey]; ok && now.Before(cached.expiry) {
		contributionsCacheMu.RUnlock()
		return cloneDayCounts(cached.counts), nil
	}
	contributionsCacheMu.RUnlock()

	contributionsInFlightMu.Lock()
	if inflight, ok := contributionsInFlight[cacheKey]; ok {
		done := inflight.done
		contributionsInFlightMu.Unlock()
		<-done
		if inflight.err != nil {
			return map[string]int{}, nil
		}
		return cloneDayCounts(inflight.counts), nil
	}
	inflight := &contributionsInflight{done: make(chan struct{})}
	contributionsInFlight[cacheKey] = inflight
	contributionsInFlightMu.Unlock()

	finish := func(counts map[string]int, err error) {
		if err == nil {
			contributionsCacheMu.Lock()
			contributionsCache[cacheKey] = contributionsCacheEntry{counts: cloneDayCounts(counts), expiry: time.Now().Add(contributionsCacheTTL)}
			contributionsCacheMu.Unlock()
		}
		contributionsInFlightMu.Lock()
		inflight.counts = cloneDayCounts(counts)
		inflight.err = err
		delete(contributionsInFlight, cacheKey)
		close(inflight.done)
		contributionsInFlightMu.Unlock()
	}

	ctx, cancel := context.WithTimeout(context.Background(), contributionsTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "-C", repoPath, "log", "--all", "--since="+sinceUTC.Format(time.RFC3339), "--author="+authorEmail, "--format=%aI").Output()
	if err != nil {
		finish(map[string]int{}, err)
		return map[string]int{}, nil
	}

	counts := make(map[string]int)
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if len(line) < 10 {
			continue
		}
		day := line[:10]
		counts[day]++
	}

	finish(counts, nil)
	return counts, nil
}

func (s *GitService) BranchExists(repoPath, branchName string) (bool, error) {
	repo, err := gogit.PlainOpen(repoPath)
	if err != nil {
		return false, err
	}

	ref, err := repo.Reference(plumbing.NewBranchReferenceName(branchName), false)
	if err != nil {
		return false, nil
	}

	return !ref.Hash().IsZero(), nil
}

func (s *GitService) CreateBranch(repoPath, branchName, fromRef string) error {
	if _, err := safeRef(branchName); err != nil {
		return fmt.Errorf("invalid branch name")
	}
	if _, err := safeRef(fromRef); err != nil {
		return fmt.Errorf("invalid source ref")
	}
	repo, err := gogit.PlainOpen(repoPath)
	if err != nil {
		return fmt.Errorf("failed to open repository: %w", err)
	}

	hash, err := resolveRef(repo, fromRef)
	if err != nil {
		return fmt.Errorf("reference not found: %w", err)
	}

	ref := plumbing.NewBranchReferenceName(branchName)
	err = repo.Storer.SetReference(plumbing.NewHashReference(ref, *hash))
	if err != nil {
		return fmt.Errorf("failed to create branch: %w", err)
	}
	branchesCacheMu.Lock()
	delete(branchesCache, repoPath)
	branchesCacheMu.Unlock()

	return nil
}

func (s *GitService) DeleteBranch(repoPath, branchName string) error {
	if _, err := safeRef(branchName); err != nil {
		return fmt.Errorf("invalid branch name")
	}

	repo, err := gogit.PlainOpen(repoPath)
	if err != nil {
		return fmt.Errorf("failed to open repository: %w", err)
	}

	if err := repo.Storer.RemoveReference(plumbing.NewBranchReferenceName(branchName)); err != nil {
		return fmt.Errorf("failed to delete branch: %w", err)
	}
	branchesCacheMu.Lock()
	delete(branchesCache, repoPath)
	branchesCacheMu.Unlock()

	return nil
}

func (s *GitService) MergeBranches(baseRepoPath, headRepoPath, baseBranch, headBranch string) error {
	log.Printf("MergeBranches: base=%s, head=%s, baseBranch=%s, headBranch=%s", baseRepoPath, headRepoPath, baseBranch, headBranch)
	return nil
}

// MergePR performs a real merge using the specified method and returns the resulting commit SHA.
// method is one of "merge", "squash", or "rebase".
func (s *GitService) MergePR(baseRepoPath, headRepoPath, baseBranch, headBranch, method, commitTitle, mergerName, mergerEmail string) (string, error) {
	tmpDir, err := os.MkdirTemp("", "gitpier-merge-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Clone base repo into a working copy
	if out, err := exec.Command("git", "clone", "--quiet", baseRepoPath, tmpDir).CombinedOutput(); err != nil {
		return "", fmt.Errorf("clone failed: %s", string(out))
	}

	// Configure committer identity
	exec.Command("git", "-C", tmpDir, "config", "user.name", mergerName).Run()
	exec.Command("git", "-C", tmpDir, "config", "user.email", mergerEmail).Run()

	// Checkout base branch
	if out, err := exec.Command("git", "-C", tmpDir, "checkout", baseBranch).CombinedOutput(); err != nil {
		return "", fmt.Errorf("checkout base failed: %s", string(out))
	}

	// Fetch head branch
	if out, err := exec.Command("git", "-C", tmpDir, "fetch", headRepoPath, headBranch+":pr_head").CombinedOutput(); err != nil {
		return "", fmt.Errorf("fetch head failed: %s", string(out))
	}

	var mergeSHA string

	switch method {
	case PRMergeMethodSquash:
		if out, err := exec.Command("git", "-C", tmpDir, "merge", "--squash", "pr_head").CombinedOutput(); err != nil {
			return "", fmt.Errorf("squash merge failed: %s", string(out))
		}
		if out, err := exec.Command("git", "-C", tmpDir, "commit", "-m", commitTitle).CombinedOutput(); err != nil {
			return "", fmt.Errorf("squash commit failed: %s", string(out))
		}
		shaOut, _ := exec.Command("git", "-C", tmpDir, "rev-parse", "HEAD").Output()
		mergeSHA = strings.TrimSpace(string(shaOut))
		if out, err := exec.Command("git", "-C", tmpDir, "push", "origin", baseBranch).CombinedOutput(); err != nil {
			return "", fmt.Errorf("push after squash failed: %s", string(out))
		}

	case PRMergeMethodRebase:
		// Replay head commits on top of base
		if out, err := exec.Command("git", "-C", tmpDir, "checkout", "pr_head").CombinedOutput(); err != nil {
			return "", fmt.Errorf("checkout pr_head failed: %s", string(out))
		}
		if out, err := exec.Command("git", "-C", tmpDir, "rebase", baseBranch).CombinedOutput(); err != nil {
			return "", fmt.Errorf("rebase failed: %s", string(out))
		}
		shaOut, _ := exec.Command("git", "-C", tmpDir, "rev-parse", "HEAD").Output()
		mergeSHA = strings.TrimSpace(string(shaOut))
		if out, err := exec.Command("git", "-C", tmpDir, "push", "origin", "HEAD:refs/heads/"+baseBranch).CombinedOutput(); err != nil {
			return "", fmt.Errorf("push after rebase failed: %s", string(out))
		}

	default: // "merge" — always create a merge commit
		msg := fmt.Sprintf("Merge pull request: %s", commitTitle)
		if out, err := exec.Command("git", "-C", tmpDir, "merge", "--no-ff", "pr_head", "-m", msg).CombinedOutput(); err != nil {
			return "", fmt.Errorf("merge failed: %s", string(out))
		}
		shaOut, _ := exec.Command("git", "-C", tmpDir, "rev-parse", "HEAD").Output()
		mergeSHA = strings.TrimSpace(string(shaOut))
		if out, err := exec.Command("git", "-C", tmpDir, "push", "origin", baseBranch).CombinedOutput(); err != nil {
			return "", fmt.Errorf("push after merge failed: %s", string(out))
		}
	}

	return mergeSHA, nil
}

// PRMergeMethod constants (mirrored from models to avoid import cycle).
const (
	PRMergeMethodMerge  = "merge"
	PRMergeMethodSquash = "squash"
	PRMergeMethodRebase = "rebase"
)

// IsMergeable checks whether headBranch can be merged into baseBranch without conflicts.
func (s *GitService) IsMergeable(baseRepoPath, headRepoPath, baseBranch, headBranch string) bool {
	tmpDir, err := os.MkdirTemp("", "gitpier-check-*")
	if err != nil {
		return false
	}
	defer os.RemoveAll(tmpDir)

	if err := exec.Command("git", "clone", "--quiet", baseRepoPath, tmpDir).Run(); err != nil {
		return false
	}
	if err := exec.Command("git", "-C", tmpDir, "checkout", baseBranch).Run(); err != nil {
		return false
	}
	if err := exec.Command("git", "-C", tmpDir, "fetch", headRepoPath, headBranch+":pr_head").Run(); err != nil {
		return false
	}
	// Attempt a dry-run merge
	err = exec.Command("git", "-C", tmpDir, "merge", "--no-commit", "--no-ff", "pr_head").Run()
	return err == nil
}

// resolveHeadRef resolves headRef to a valid commit SHA, falling back to headSHA
// if the branch no longer exists (e.g., after merge and branch deletion).
func (s *GitService) resolveHeadRef(repoPath, headRef, headSHA string) (string, error) {
	// First try to use the branch ref directly
	_, err := exec.Command("git", "-C", repoPath, "rev-parse", "--verify", headRef).Output()
	if err == nil {
		return headRef, nil
	}
	// Branch doesn't exist, try using headSHA directly
	if headSHA != "" {
		_, err := exec.Command("git", "-C", repoPath, "rev-parse", "--verify", headSHA).Output()
		if err == nil {
			return headSHA, nil
		}
	}
	return "", fmt.Errorf("neither branch %q nor SHA %q found", headRef, headSHA)
}

func (s *GitService) preparePRCompareRepo(baseRepoPath, headRepoPath, headRef string) (string, string, func(), error) {
	if baseRepoPath == headRepoPath {
		return baseRepoPath, headRef, func() {}, nil
	}

	tmpDir, err := os.MkdirTemp("", "gitpier-pr-compare-*")
	if err != nil {
		return "", "", nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(tmpDir) }

	if out, err := exec.Command("git", "clone", "--quiet", baseRepoPath, tmpDir).CombinedOutput(); err != nil {
		cleanup()
		return "", "", nil, fmt.Errorf("clone failed: %s", strings.TrimSpace(string(out)))
	}
	if out, err := exec.Command("git", "-C", tmpDir, "fetch", headRepoPath, headRef+":pr_head").CombinedOutput(); err != nil {
		cleanup()
		return "", "", nil, fmt.Errorf("fetch head failed: %s", strings.TrimSpace(string(out)))
	}

	return tmpDir, "pr_head", cleanup, nil
}

// GetPRCommits returns commits on headRef that are not reachable from baseRef.
func (s *GitService) GetPRCommits(repoPath, baseRef, headRef, headSHA string) ([]*CommitInfo, error) {
	return s.GetPRCommitsBetweenRepos(repoPath, repoPath, baseRef, headRef, headSHA)
}

// GetPRCommitsBetweenRepos returns commits for a PR where the head branch may
// come from a different repository (fork).
func (s *GitService) GetPRCommitsBetweenRepos(baseRepoPath, headRepoPath, baseRef, headRef, headSHA string) ([]*CommitInfo, error) {
	workRepoPath, compareHeadRef, cleanup, err := s.preparePRCompareRepo(baseRepoPath, headRepoPath, headRef)
	if err != nil {
		return []*CommitInfo{}, nil
	}
	defer cleanup()

	// Resolve head ref (handle deleted branch after merge)
	resolvedHead, err := s.resolveHeadRef(workRepoPath, compareHeadRef, headSHA)
	if err != nil {
		return []*CommitInfo{}, nil
	}

	// Find merge base
	mergeBaseOut, err := exec.Command("git", "-C", workRepoPath, "merge-base", baseRef, resolvedHead).Output()
	if err != nil {
		return []*CommitInfo{}, nil
	}
	mergeBase := strings.TrimSpace(string(mergeBaseOut))

	args := []string{
		"-C", workRepoPath,
		"log",
		"--format=%H%x1f%aI%x1f%an%x1f%ae%x1f%s",
		mergeBase + ".." + resolvedHead,
	}
	out, err := exec.Command("git", args...).Output()
	if err != nil || len(strings.TrimSpace(string(out))) == 0 {
		return []*CommitInfo{}, nil
	}

	var commits []*CommitInfo
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\x1f", 5)
		if len(parts) < 5 {
			continue
		}
		t, _ := time.Parse(time.RFC3339, parts[1])
		commits = append(commits, &CommitInfo{
			SHA:     parts[0],
			Message: parts[4],
			Author: AuthorInfo{
				Name:  parts[2],
				Email: parts[3],
				Date:  t,
			},
		})
	}
	return commits, nil
}

// GetPRDiff returns per-file diffs for all changes between baseRef and headRef
// using the three-dot (merge-base) comparison.
func (s *GitService) GetPRDiff(repoPath, baseRef, headRef, headSHA string) ([]FileDiff, error) {
	return s.GetPRDiffBetweenRepos(repoPath, repoPath, baseRef, headRef, headSHA)
}

// GetPRDiffBetweenRepos returns per-file diffs for PRs where the head branch
// may be in a different repository.
func (s *GitService) GetPRDiffBetweenRepos(baseRepoPath, headRepoPath, baseRef, headRef, headSHA string) ([]FileDiff, error) {
	workRepoPath, compareHeadRef, cleanup, err := s.preparePRCompareRepo(baseRepoPath, headRepoPath, headRef)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	// Resolve head ref (handle deleted branch after merge)
	resolvedHead, err := s.resolveHeadRef(workRepoPath, compareHeadRef, headSHA)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve head ref: %w", err)
	}

	// Resolve merge base
	mergeBaseOut, err := exec.Command("git", "-C", workRepoPath, "merge-base", baseRef, resolvedHead).Output()
	if err != nil {
		return nil, fmt.Errorf("merge-base failed: %w", err)
	}
	mergeBase := strings.TrimSpace(string(mergeBaseOut))
	rangeSpec := mergeBase + ".." + resolvedHead

	// Additions/deletions per file
	numStatOut, _ := exec.Command("git", "-C", workRepoPath, "diff", "--numstat", rangeSpec).Output()
	type fileStat struct{ adds, dels int }
	stats := map[string]fileStat{}
	for _, line := range strings.Split(strings.TrimSpace(string(numStatOut)), "\n") {
		if line == "" {
			continue
		}
		p := strings.Fields(line)
		if len(p) < 3 {
			continue
		}
		a, _ := strconv.Atoi(p[0])
		d, _ := strconv.Atoi(p[1])
		stats[p[2]] = fileStat{a, d}
	}

	// File status (A/M/D/R)
	nameStatusOut, _ := exec.Command("git", "-C", workRepoPath, "diff", "--name-status", rangeSpec).Output()
	type fileEntry struct{ status, oldPath, newPath string }
	var files []fileEntry
	for _, line := range strings.Split(strings.TrimSpace(string(nameStatusOut)), "\n") {
		if line == "" {
			continue
		}
		p := strings.Fields(line)
		if len(p) < 2 {
			continue
		}
		fe := fileEntry{}
		switch {
		case p[0] == "A":
			fe.status, fe.newPath = "added", p[1]
		case p[0] == "D":
			fe.status, fe.oldPath, fe.newPath = "deleted", p[1], p[1]
		case strings.HasPrefix(p[0], "R") && len(p) >= 3:
			fe.status, fe.oldPath, fe.newPath = "renamed", p[1], p[2]
		default:
			fe.status, fe.newPath = "modified", p[len(p)-1]
		}
		files = append(files, fe)
	}

	// Full unified diff split by file
	diffOut, _ := exec.Command("git", "-C", workRepoPath, "diff", "-U3", rangeSpec).Output()
	patches := splitDiffByFile(string(diffOut))

	var diffs []FileDiff
	for _, fe := range files {
		key := fe.newPath
		if fe.status == "deleted" {
			key = fe.oldPath
		}
		s := stats[key]
		diffs = append(diffs, FileDiff{
			Path:      fe.newPath,
			OldPath:   fe.oldPath,
			Type:      fe.status,
			Additions: s.adds,
			Deletions: s.dels,
			Patch:     patches[key],
		})
	}
	return diffs, nil
}

// splitDiffByFile splits a unified diff output into per-file patches keyed by the b/ path.
func splitDiffByFile(diffOutput string) map[string]string {
	result := make(map[string]string)
	lines := strings.Split(diffOutput, "\n")
	var currentFile string
	var buf strings.Builder

	for _, line := range lines {
		if strings.HasPrefix(line, "diff --git ") {
			if currentFile != "" {
				result[currentFile] = buf.String()
			}
			buf.Reset()
			// Extract b/ path
			parts := strings.Fields(line)
			if len(parts) >= 4 {
				currentFile = strings.TrimPrefix(parts[3], "b/")
			}
		}
		if currentFile != "" {
			buf.WriteString(line + "\n")
		}
	}
	if currentFile != "" {
		result[currentFile] = buf.String()
	}
	return result
}

// GetAllRefs returns a map of ref name → commit SHA for all hash-type references in the repo.
// Returns an empty map (no error) if the repo does not yet exist or is empty.
func (s *GitService) GetAllRefs(repoPath string) (map[string]string, error) {
	repo, err := gogit.PlainOpen(repoPath)
	if err != nil {
		return map[string]string{}, nil
	}

	result := make(map[string]string)
	iter, err := repo.References()
	if err != nil {
		return result, err
	}

	_ = iter.ForEach(func(ref *plumbing.Reference) error {
		if ref.Type() == plumbing.HashReference {
			result[ref.Name().String()] = ref.Hash().String()
		}
		return nil
	})

	return result, nil
}

// GetCommitsBetween returns commits reachable from `after` but not from `before`.
// If before is empty or all-zero (new branch), returns up to limit commits leading to after.
func (s *GitService) GetCommitsBetween(repoPath, before, after string, limit int) ([]*CommitInfo, error) {
	if limit <= 0 {
		limit = 20
	}
	isZero := before == "" || strings.TrimLeft(before, "0") == ""
	var rangeArg string
	if isZero {
		rangeArg = after
	} else {
		rangeArg = before + ".." + after
	}
	args := []string{
		"-C", repoPath,
		"log",
		"--format=%H%x1f%aI%x1f%an%x1f%ae%x1f%s",
		"-n", strconv.Itoa(limit),
		rangeArg,
	}
	out, err := exec.Command("git", args...).Output()
	if err != nil || len(strings.TrimSpace(string(out))) == 0 {
		return []*CommitInfo{}, nil
	}
	var commits []*CommitInfo
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\x1f", 5)
		if len(parts) < 5 {
			continue
		}
		t, _ := time.Parse(time.RFC3339, parts[1])
		commits = append(commits, &CommitInfo{
			SHA:     parts[0],
			Message: parts[4],
			Author: AuthorInfo{
				Name:  parts[2],
				Email: parts[3],
				Date:  t,
			},
		})
	}
	return commits, nil
}

// IsForceUpdate returns true when the update from before→after is not a fast-forward.
// Returns false for new-branch (before empty/zeros) or branch-deletion (after empty/zeros).
func (s *GitService) IsForceUpdate(repoPath, before, after string) bool {
	if before == "" || strings.TrimLeft(before, "0") == "" {
		return false
	}
	if after == "" || strings.TrimLeft(after, "0") == "" {
		return false
	}
	err := exec.Command("git", "-C", repoPath, "merge-base", "--is-ancestor", before, after).Run()
	return err != nil
}

// GetCommitFiles returns the lists of added, modified, and removed file paths for a single commit.
// It handles the initial commit (no parent) via --root.
func (s *GitService) GetCommitFiles(repoPath, sha string) (added, modified, removed []string) {
	added = []string{}
	modified = []string{}
	removed = []string{}

	out, err := exec.Command("git", "-C", repoPath, "diff-tree", "--root", "-r", "--name-status", "--no-commit-id", sha).Output()
	if err != nil || len(strings.TrimSpace(string(out))) == 0 {
		return
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 2 {
			continue
		}
		status := parts[0]
		switch {
		case strings.HasPrefix(status, "A"):
			added = append(added, parts[1])
		case strings.HasPrefix(status, "M"), strings.HasPrefix(status, "T"):
			modified = append(modified, parts[1])
		case strings.HasPrefix(status, "D"):
			removed = append(removed, parts[1])
		case strings.HasPrefix(status, "R"):
			if len(parts) >= 3 {
				removed = append(removed, parts[1])
				added = append(added, parts[2])
			}
		case strings.HasPrefix(status, "C"):
			if len(parts) >= 3 {
				added = append(added, parts[2])
			}
		}
	}
	return
}

// TagInfo describes a git tag.
type TagInfo struct {
	Name      string    `json:"name"`
	SHA       string    `json:"sha"`        // tag object SHA (or commit SHA for lightweight tags)
	CommitSHA string    `json:"commit_sha"` // resolved commit SHA
	Message   string    `json:"message"`
	Date      time.Time `json:"date"`
}

// GetTags returns all tags for a repository, sorted newest first.
func (s *GitService) GetTags(repoPath string) ([]TagInfo, error) {
	repo, err := gogit.PlainOpen(repoPath)
	if err != nil {
		return nil, err
	}

	tagIter, err := repo.Tags()
	if err != nil {
		return nil, err
	}

	var tags []TagInfo
	_ = tagIter.ForEach(func(ref *plumbing.Reference) error {
		ti := TagInfo{
			Name: ref.Name().Short(),
			SHA:  ref.Hash().String(),
		}

		// Try to resolve as annotated tag first
		tagObj, err := repo.TagObject(ref.Hash())
		if err == nil {
			ti.CommitSHA = tagObj.Target.String()
			ti.Message = strings.TrimSpace(tagObj.Message)
			ti.Date = tagObj.Tagger.When
		} else {
			// Lightweight tag — hash points directly to a commit
			ti.CommitSHA = ref.Hash().String()
			commit, cerr := repo.CommitObject(ref.Hash())
			if cerr == nil {
				ti.Date = commit.Author.When
			}
		}

		tags = append(tags, ti)
		return nil
	})

	// Sort newest first
	sort.Slice(tags, func(i, j int) bool {
		return tags[i].Date.After(tags[j].Date)
	})

	return tags, nil
}

// TagExists reports whether the given tag exists in the repository.
func (s *GitService) TagExists(repoPath, tagName string) (bool, error) {
	if _, err := safeRef(tagName); err != nil {
		return false, ErrInvalidGitParam
	}
	repo, err := gogit.PlainOpen(repoPath)
	if err != nil {
		if errors.Is(err, gogit.ErrRepositoryNotExists) {
			return false, ErrGitRepositoryNotFound
		}
		return false, fmt.Errorf("failed to open repository: %w", err)
	}
	_, err = repo.Tag(tagName)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, gogit.ErrTagNotFound) || errors.Is(err, plumbing.ErrReferenceNotFound) {
		return false, nil
	}
	return false, fmt.Errorf("failed to resolve tag: %w", err)
}

// CommitFile writes (creates or updates) a single file in the repository and commits it.
// The caller must ensure filePath and branch have already been validated.
func (s *GitService) CommitFile(repoPath, branch, filePath, content, message, authorName, authorEmail string) (string, error) {
	safeBranch, err := safeRef(branch)
	if err != nil {
		return "", fmt.Errorf("invalid branch: %w", err)
	}
	safeP, err := safeFilePath(filePath)
	if err != nil {
		return "", fmt.Errorf("invalid file path: %w", err)
	}
	if message == "" {
		message = fmt.Sprintf("Update %s", safeP)
	}
	// Sanitize message for shell safety (no null bytes or control chars)
	message = strings.Map(func(r rune) rune {
		if r == 0 {
			return -1
		}
		return r
	}, message)
	if authorName == "" {
		authorName = "Unknown"
	}
	if authorEmail == "" {
		authorEmail = "unknown@gitpier"
	}

	tmpDir, err := os.MkdirTemp("", "gitpier-edit-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Clone the bare repo into a working copy, checking out the target branch.
	if out, err := exec.Command("git", "clone", "--quiet", "--branch", safeBranch, repoPath, tmpDir).CombinedOutput(); err != nil {
		return "", fmt.Errorf("clone failed: %s", string(out))
	}

	// Configure committer identity.
	exec.Command("git", "-C", tmpDir, "config", "user.name", authorName).Run()
	exec.Command("git", "-C", tmpDir, "config", "user.email", authorEmail).Run()

	// Write file content, creating parent directories as needed.
	fullPath := filepath.Join(tmpDir, filepath.FromSlash(safeP))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return "", fmt.Errorf("failed to create directories: %w", err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	// Stage the specific file only.
	if out, err := exec.Command("git", "-C", tmpDir, "add", safeP).CombinedOutput(); err != nil {
		return "", fmt.Errorf("git add failed: %s", string(out))
	}

	// Abort if nothing changed.
	statusOut, _ := exec.Command("git", "-C", tmpDir, "status", "--porcelain").Output()
	if len(strings.TrimSpace(string(statusOut))) == 0 {
		return "", fmt.Errorf("no changes to commit")
	}

	if out, err := exec.Command("git", "-C", tmpDir, "commit", "-m", message, "--trailer", "GitPier-Web: true").CombinedOutput(); err != nil {
		return "", fmt.Errorf("git commit failed: %s", string(out))
	}

	shaOut, _ := exec.Command("git", "-C", tmpDir, "rev-parse", "HEAD").Output()
	sha := strings.TrimSpace(string(shaOut))

	if out, err := exec.Command("git", "-C", tmpDir, "push", "origin", safeBranch).CombinedOutput(); err != nil {
		return "", fmt.Errorf("push failed: %s", string(out))
	}

	return sha, nil
}

// CreateTag creates an annotated tag at the given commit or ref (or HEAD if targetRef is empty).
func (s *GitService) CreateTag(repoPath, tagName, targetRef, message string) error {
	if _, err := safeRef(tagName); err != nil {
		return fmt.Errorf("invalid tag name")
	}
	if exists, err := s.TagExists(repoPath, tagName); err == nil && exists {
		return ErrTagAlreadyExists
	} else if err != nil && !errors.Is(err, ErrGitRepositoryNotFound) {
		return err
	}
	if strings.TrimSpace(message) == "" {
		message = tagName
	}

	var targetSHA string
	if targetRef != "" {
		repo, err := gogit.PlainOpen(repoPath)
		if err != nil {
			if errors.Is(err, gogit.ErrRepositoryNotExists) {
				return ErrGitRepositoryNotFound
			}
			return fmt.Errorf("failed to open repository: %w", err)
		}

		if safe, err := safeSHA(targetRef); err == nil {
			targetSHA = safe
		} else {
			if _, err := safeRef(targetRef); err != nil {
				return fmt.Errorf("invalid target ref")
			}
			hash, err := resolveRef(repo, targetRef)
			if err != nil {
				return ErrGitReferenceNotFound
			}
			targetSHA = hash.String()
		}
	}

	// Annotated tags require a tagger identity. In containerized/self-hosted
	// deployments, global git config may be missing, so always provide one.
	args := []string{
		"-C", repoPath,
		"-c", "user.name=" + s.defaultIdentityName,
		"-c", "user.email=" + s.defaultIdentityEmail,
		"tag", "-a", tagName, "-m", message,
	}
	if targetSHA != "" {
		args = append(args, targetSHA)
	}
	cmd := exec.Command("git", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.ToLower(strings.TrimSpace(string(out)))
		switch {
		case strings.Contains(msg, "already exists"):
			return ErrTagAlreadyExists
		case strings.Contains(msg, "not a git repository"):
			return ErrGitRepositoryNotFound
		case strings.Contains(msg, "failed to resolve 'head'"),
			strings.Contains(msg, "needed a single revision"),
			strings.Contains(msg, "unknown revision or path not in the working tree"):
			return ErrEmptyRepository
		}
		return fmt.Errorf("failed to create tag: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func (s *GitService) DeleteTag(repoPath, tagName string) error {
	if _, err := safeRef(tagName); err != nil {
		return fmt.Errorf("invalid tag name")
	}
	cmd := exec.Command("git", "-C", repoPath, "tag", "-d", tagName)
	if _, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to delete tag")
	}
	return nil
}

// GetArchive returns the contents of a source archive (zip or tar.gz) for the given ref.
// format must be "zip" or "tar.gz".
func (s *GitService) GetArchive(repoPath, ref, format string) ([]byte, error) {
	return s.GetArchiveWithPrefix(repoPath, ref, format, "source/")
}

// GetArchiveWithPrefix returns an archive for the given ref and optional path prefix.
// Pass an empty prefix to place files at the archive root.
func (s *GitService) GetArchiveWithPrefix(repoPath, ref, format, prefix string) ([]byte, error) {
	if format != "zip" && format != "tar.gz" {
		return nil, fmt.Errorf("unsupported archive format: %s", format)
	}
	safeR, err := safeRef(ref)
	if err != nil {
		return nil, fmt.Errorf("invalid ref")
	}
	// git archive produces the archive on stdout
	args := []string{"-C", repoPath, "archive", "--format=" + format}
	if prefix != "" {
		args = append(args, "--prefix="+prefix)
	}
	args = append(args, safeR)
	cmd := exec.Command("git", args...)
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("git archive: %s", strings.TrimSpace(string(ee.Stderr)))
		}
		return nil, fmt.Errorf("git archive: %w", err)
	}
	return out, nil
}

// extensionLanguages maps common file extensions to their language name.
var extensionLanguages = map[string]string{
	".go":     "Go",
	".js":     "JavaScript",
	".ts":     "TypeScript",
	".jsx":    "JavaScript",
	".tsx":    "TypeScript",
	".py":     "Python",
	".rb":     "Ruby",
	".rs":     "Rust",
	".java":   "Java",
	".kt":     "Kotlin",
	".swift":  "Swift",
	".c":      "C",
	".h":      "C",
	".cpp":    "C++",
	".cc":     "C++",
	".cxx":    "C++",
	".cs":     "C#",
	".php":    "PHP",
	".html":   "HTML",
	".htm":    "HTML",
	".css":    "CSS",
	".scss":   "CSS",
	".sass":   "CSS",
	".vue":    "Vue",
	".svelte": "Svelte",
	".sh":     "Shell",
	".bash":   "Shell",
	".zsh":    "Shell",
	".fish":   "Shell",
	".ps1":    "PowerShell",
	".lua":    "Lua",
	".r":      "R",
	".scala":  "Scala",
	".ex":     "Elixir",
	".exs":    "Elixir",
	".erl":    "Erlang",
	".hrl":    "Erlang",
	".hs":     "Haskell",
	".lhs":    "Haskell",
	".clj":    "Clojure",
	".cljs":   "Clojure",
	".dart":   "Dart",
	".m":      "Objective-C",
	".mm":     "Objective-C",
	".pl":     "Perl",
	".pm":     "Perl",
	".groovy": "Groovy",
	".tf":     "HCL",
	".nix":    "Nix",
	".ml":     "OCaml",
	".mli":    "OCaml",
	".f90":    "Fortran",
	".f95":    "Fortran",
	".f":      "Fortran",
	".jl":     "Julia",
	".zig":    "Zig",
	".v":      "V",
	".nim":    "Nim",
	".cr":     "Crystal",
	".d":      "D",
}

// langByteCount scans the selected ref tree of a bare repo and returns a map of
// language name -> total bytes. Returns nil if the repo is empty.
// Uses a streaming scanner so large repos don't blow up the pipe buffer.
func langByteCount(repoPath, ref string) map[string]int64 {
	refs := make([]string, 0, 4)
	seen := make(map[string]struct{}, 4)
	for _, r := range []string{strings.TrimSpace(ref), "HEAD", "master", "main"} {
		if r == "" {
			continue
		}
		if _, ok := seen[r]; ok {
			continue
		}
		seen[r] = struct{}{}
		refs = append(refs, r)
	}

	for _, r := range refs {
		langBytes, ok := langByteCountForRef(repoPath, r)
		if ok {
			if len(langBytes) == 0 {
				return nil
			}
			return langBytes
		}
	}

	return nil
}

// langByteCountForRef returns (counts, true) when the ref exists and was scanned,
// and (nil, false) when the ref cannot be resolved.
func langByteCountForRef(repoPath, ref string) (map[string]int64, bool) {
	cmd := exec.Command("git", "-C", repoPath, "ls-tree", "-r", "--long", ref)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, false
	}
	if err := cmd.Start(); err != nil {
		return nil, false
	}

	langBytes := make(map[string]int64)
	scanner := bufio.NewScanner(stdout)
	// Each line can be long on repos with deep paths; give it plenty of room.
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		tabIdx := strings.IndexByte(line, '\t')
		if tabIdx < 0 {
			continue
		}
		path := line[tabIdx+1:]
		ext := strings.ToLower(filepath.Ext(path))
		lang, ok := extensionLanguages[ext]
		if !ok {
			continue
		}
		fields := strings.Fields(line[:tabIdx])
		if len(fields) < 4 {
			continue
		}
		size, err := strconv.ParseInt(fields[3], 10, 64)
		if err != nil {
			continue
		}
		langBytes[lang] += size
	}

	if err := scanner.Err(); err != nil {
		_ = cmd.Wait()
		return nil, false
	}
	if err := cmd.Wait(); err != nil {
		return nil, false
	}

	return langBytes, true
}

// LanguageStat holds one language entry in a breakdown.
type LanguageStat struct {
	Name    string  `json:"name"`
	Bytes   int64   `json:"bytes"`
	Percent float64 `json:"percent"`
}

// GetTopLanguage returns the dominant programming language in the repo.
// Returns an empty string if the repo is empty or has no recognisable source files.
func (s *GitService) GetTopLanguage(repoPath, ref string) string {
	langBytes := langByteCount(repoPath, ref)
	var topLang string
	var topBytes int64
	for lang, b := range langBytes {
		if b > topBytes {
			topBytes = b
			topLang = lang
		}
	}
	return topLang
}

// GetLanguageBreakdown returns all detected languages sorted by byte count descending,
// each annotated with its percentage of total code bytes.
func (s *GitService) GetLanguageBreakdown(repoPath, ref string) []LanguageStat {
	langBytes := langByteCount(repoPath, ref)
	if len(langBytes) == 0 {
		return nil
	}

	var total int64
	for _, b := range langBytes {
		total += b
	}

	stats := make([]LanguageStat, 0, len(langBytes))
	for lang, b := range langBytes {
		pct := float64(b) / float64(total) * 100
		stats = append(stats, LanguageStat{Name: lang, Bytes: b, Percent: pct})
	}
	// Sort descending by bytes
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].Bytes > stats[j].Bytes
	})
	return stats
}

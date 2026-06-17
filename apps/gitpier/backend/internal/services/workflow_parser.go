package services

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// WorkflowDef is the top-level GitHub Actions-compatible workflow structure.
type WorkflowDef struct {
	Name        string                    `yaml:"name"`
	On          WorkflowTriggers          `yaml:"on"`
	Permissions map[string]string         `yaml:"permissions,omitempty"`
	Env         map[string]string         `yaml:"env,omitempty"`
	Jobs        map[string]WorkflowJobDef `yaml:"jobs"`
}

// WorkflowTriggers handles the `on:` field which can be a string, array, or map.
type WorkflowTriggers struct {
	Push             *PushTrigger             `yaml:"push,omitempty"`
	PullRequest      *PullRequestTrigger      `yaml:"pull_request,omitempty"`
	Release          *ReleaseTrigger          `yaml:"release,omitempty"`
	WorkflowDispatch *WorkflowDispatchTrigger `yaml:"workflow_dispatch,omitempty"`
}

func (t *WorkflowTriggers) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		// on: push
		switch value.Value {
		case "push":
			t.Push = &PushTrigger{}
		case "pull_request":
			t.PullRequest = &PullRequestTrigger{}
		case "release":
			t.Release = &ReleaseTrigger{}
		case "workflow_dispatch":
			t.WorkflowDispatch = &WorkflowDispatchTrigger{}
		}
	case yaml.SequenceNode:
		// on: [push, pull_request, workflow_dispatch]
		for _, item := range value.Content {
			switch item.Value {
			case "push":
				t.Push = &PushTrigger{}
			case "pull_request":
				t.PullRequest = &PullRequestTrigger{}
			case "release":
				t.Release = &ReleaseTrigger{}
			case "workflow_dispatch":
				t.WorkflowDispatch = &WorkflowDispatchTrigger{}
			}
		}
	case yaml.MappingNode:
		// on: { push: { branches: [...] }, pull_request: {...}, workflow_dispatch: {} }
		type rawTriggers struct {
			Push             *PushTrigger             `yaml:"push,omitempty"`
			PullRequest      *PullRequestTrigger      `yaml:"pull_request,omitempty"`
			Release          *ReleaseTrigger          `yaml:"release,omitempty"`
			WorkflowDispatch *WorkflowDispatchTrigger `yaml:"workflow_dispatch,omitempty"`
		}
		var raw rawTriggers
		if err := value.Decode(&raw); err != nil {
			return err
		}
		t.Push = raw.Push
		t.PullRequest = raw.PullRequest
		t.Release = raw.Release
		t.WorkflowDispatch = raw.WorkflowDispatch
	}
	return nil
}

type PushTrigger struct {
	Branches []string `yaml:"branches,omitempty"`
	Paths    []string `yaml:"paths,omitempty"`
	Tags     []string `yaml:"tags,omitempty"`
}

type PullRequestTrigger struct {
	Branches []string `yaml:"branches,omitempty"`
	Paths    []string `yaml:"paths,omitempty"`
	Types    []string `yaml:"types,omitempty"`
}

type ReleaseTrigger struct {
	Types []string `yaml:"types,omitempty"`
	Tags  []string `yaml:"tags,omitempty"`
}

// WorkflowDispatchTrigger represents the `workflow_dispatch:` trigger (manual runs).
// Inputs are intentionally ignored for now — future extension point.
type WorkflowDispatchTrigger struct{}

// WorkflowJobDef describes a single job in the workflow.
type WorkflowJobDef struct {
	Name        string            `yaml:"name"`
	RunsOn      string            `yaml:"runs-on"`
	Needs       StringOrSlice     `yaml:"needs,omitempty"`
	Env         map[string]string `yaml:"env,omitempty"`
	Permissions map[string]string `yaml:"permissions,omitempty"`
	Steps       []WorkflowStepDef `yaml:"steps"`
}

// WorkflowStepDef describes a single step in a job.
type WorkflowStepDef struct {
	ID   string            `yaml:"id,omitempty"`
	Name string            `yaml:"name,omitempty"`
	Uses string            `yaml:"uses,omitempty"`
	Run  string            `yaml:"run,omitempty"`
	With AnyStringMap      `yaml:"with,omitempty"`
	Env  map[string]string `yaml:"env,omitempty"`
	If   string            `yaml:"if,omitempty"`
}

// StringOrSlice unmarshals either "value" or ["a","b"] YAML into []string.
type StringOrSlice []string

func (s *StringOrSlice) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		*s = []string{value.Value}
		return nil
	}
	var slice []string
	if err := value.Decode(&slice); err != nil {
		return err
	}
	*s = slice
	return nil
}

// AnyStringMap unmarshals map[string]any → map[string]string, coercing all values to strings.
type AnyStringMap map[string]string

func (m *AnyStringMap) UnmarshalYAML(value *yaml.Node) error {
	if *m == nil {
		*m = make(AnyStringMap)
	}
	if value.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(value.Content); i += 2 {
		key := value.Content[i].Value
		val := value.Content[i+1].Value
		(*m)[key] = val
	}
	return nil
}

// ParseWorkflow parses raw YAML bytes into a WorkflowDef.
func ParseWorkflow(content []byte) (*WorkflowDef, error) {
	var wf WorkflowDef
	if err := yaml.Unmarshal(content, &wf); err != nil {
		return nil, fmt.Errorf("invalid workflow YAML: %w", err)
	}
	if len(wf.Jobs) == 0 {
		return nil, fmt.Errorf("workflow has no jobs")
	}
	return &wf, nil
}

// workflowDirs lists the directories to search for workflow YAML files, in priority order.
var workflowDirs = []string{
	".gitpier/workflows",
	".github/workflows",
	".workflows",
}

// WorkflowFile holds the path and parsed content of a workflow file.
type WorkflowFile struct {
	Path    string
	Content []byte
	Def     *WorkflowDef
}

// FindAllWorkflowFiles scans all supported workflow directories and returns every valid
// workflow file at the given ref, regardless of its trigger configuration.
func FindAllWorkflowFiles(gitSvc *GitService, repoPath, ref string) ([]WorkflowFile, error) {
	var results []WorkflowFile

	for _, dir := range workflowDirs {
		entries, err := gitSvc.ListTree(repoPath, ref, dir, false)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if entry.Type != "blob" {
				continue
			}
			if !strings.HasSuffix(entry.Name, ".yml") && !strings.HasSuffix(entry.Name, ".yaml") {
				continue
			}

			content, err := gitSvc.GetBlob(repoPath, ref, entry.Path)
			if err != nil {
				continue
			}

			wf, err := ParseWorkflow(content)
			if err != nil {
				continue
			}

			results = append(results, WorkflowFile{
				Path:    entry.Path,
				Content: content,
				Def:     wf,
			})
		}
	}

	return results, nil
}

// FindWorkflowFiles scans all supported workflow directories in the repository at the given ref
// and returns parsed workflow files that match the trigger event and branch.
func FindWorkflowFiles(gitSvc *GitService, repoPath, ref, event, refName, eventAction string) ([]WorkflowFile, error) {
	var results []WorkflowFile

	for _, dir := range workflowDirs {
		entries, err := gitSvc.ListTree(repoPath, ref, dir, false)
		if err != nil {
			// Directory doesn't exist in this ref – skip
			continue
		}

		for _, entry := range entries {
			if entry.Type != "blob" {
				continue
			}
			if !strings.HasSuffix(entry.Name, ".yml") && !strings.HasSuffix(entry.Name, ".yaml") {
				continue
			}

			content, err := gitSvc.GetBlob(repoPath, ref, entry.Path)
			if err != nil {
				continue
			}

			wf, err := ParseWorkflow(content)
			if err != nil {
				continue
			}

			if !MatchesTrigger(wf, event, refName, eventAction) {
				continue
			}

			results = append(results, WorkflowFile{
				Path:    entry.Path,
				Content: content,
				Def:     wf,
			})
		}
	}

	return results, nil
}

// MatchesTrigger returns true when the workflow should fire for the given event and branch.
func MatchesTrigger(wf *WorkflowDef, event, refName, eventAction string) bool {
	switch event {
	case "push":
		if wf.On.Push == nil {
			return false
		}
		trigger := wf.On.Push
		if len(trigger.Branches) == 0 {
			return true
		}
		return matchesBranchFilter(trigger.Branches, refName)

	case "pull_request":
		if wf.On.PullRequest == nil {
			return false
		}
		trigger := wf.On.PullRequest
		if len(trigger.Branches) == 0 {
			return true
		}
		return matchesBranchFilter(trigger.Branches, refName)

	case "release":
		if wf.On.Release == nil {
			return false
		}
		trigger := wf.On.Release
		if len(trigger.Types) > 0 && !matchesBranchFilter(trigger.Types, eventAction) {
			return false
		}
		if len(trigger.Tags) > 0 && !matchesBranchFilter(trigger.Tags, refName) {
			return false
		}
		return true

	case "workflow_dispatch":
		return wf.On.WorkflowDispatch != nil
	}
	return false
}

// matchesBranchFilter checks if branch matches any pattern in the list.
// Supports glob-style '*' wildcards.
func matchesBranchFilter(patterns []string, branch string) bool {
	for _, pattern := range patterns {
		if matchGlob(pattern, branch) {
			return true
		}
	}
	return false
}

func matchGlob(pattern, s string) bool {
	// Simple glob: * matches any substring, ** matches across /
	if pattern == "*" || pattern == "**" {
		return true
	}
	if !strings.Contains(pattern, "*") {
		return pattern == s
	}
	parts := strings.Split(pattern, "*")
	pos := 0
	for i, part := range parts {
		if part == "" {
			continue
		}
		idx := strings.Index(s[pos:], part)
		if idx == -1 {
			return false
		}
		if i == 0 && idx != 0 {
			return false // pattern doesn't start with *
		}
		pos += idx + len(part)
	}
	if !strings.HasSuffix(pattern, "*") && pos != len(s) {
		return false
	}
	return true
}

// StepDisplayName returns a human-readable name for a step.
func StepDisplayName(step WorkflowStepDef, index int) string {
	if step.Name != "" {
		return step.Name
	}
	if step.Uses != "" {
		return step.Uses
	}
	// First line of run script
	lines := strings.SplitN(strings.TrimSpace(step.Run), "\n", 2)
	if len(lines) > 0 && lines[0] != "" {
		return "Run: " + lines[0]
	}
	return fmt.Sprintf("Step %d", index+1)
}

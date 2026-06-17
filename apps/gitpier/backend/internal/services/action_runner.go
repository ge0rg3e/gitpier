package services

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	dockerbuild "github.com/docker/docker/api/types/build"
	dockercontainer "github.com/docker/docker/api/types/container"
	dockerimage "github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/pkg/stdcopy"
	"gopkg.in/yaml.v3"
)

var actionCacheMu sync.Mutex

// ActionManifest holds the parsed content of an action's action.yml / action.yaml.
type ActionManifest struct {
	Name        string                 `yaml:"name"`
	Description string                 `yaml:"description"`
	Inputs      map[string]ActionInput `yaml:"inputs"`
	Runs        ActionRuns             `yaml:"runs"`
}

// ActionInput describes one input parameter of a GitHub Action.
type ActionInput struct {
	Description string `yaml:"description"`
	Required    bool   `yaml:"required"`
	Default     string `yaml:"default"`
}

// ActionRuns describes the execution type and entrypoint for the action.
type ActionRuns struct {
	Using      string          `yaml:"using"`
	Image      string          `yaml:"image,omitempty"`
	Entrypoint string          `yaml:"entrypoint,omitempty"`
	Args       StringOrSlice   `yaml:"args,omitempty"`
	Main       string          `yaml:"main,omitempty"`
	Pre        string          `yaml:"pre,omitempty"`
	Post       string          `yaml:"post,omitempty"`
	Steps      []CompositeStep `yaml:"steps,omitempty"`
}

// CompositeStep is a step defined inside a composite action's runs.steps list.
type CompositeStep struct {
	ID    string            `yaml:"id,omitempty"`
	Name  string            `yaml:"name,omitempty"`
	Uses  string            `yaml:"uses,omitempty"`
	Run   string            `yaml:"run,omitempty"`
	With  AnyStringMap      `yaml:"with,omitempty"`
	Env   map[string]string `yaml:"env,omitempty"`
	Shell string            `yaml:"shell,omitempty"`
	If    string            `yaml:"if,omitempty"`
}

// fetchAction clones a GitHub Action to the local cache and returns the directory
// containing action.yml. The uses format is "owner/repo@ref" or "owner/repo/subdir@ref".
// Cached clones are reused on subsequent calls.
func fetchAction(uses, actionsCache string) (string, error) {
	actionCacheMu.Lock()
	defer actionCacheMu.Unlock()

	atIdx := strings.LastIndex(uses, "@")
	if atIdx == -1 {
		return "", fmt.Errorf("invalid action ref (missing @version): %s", uses)
	}
	ref := uses[atIdx+1:]
	repoAndSub := uses[:atIdx]

	parts := strings.SplitN(repoAndSub, "/", 3)
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid action path: %s", repoAndSub)
	}
	owner, repo := parts[0], parts[1]
	subdir := ""
	if len(parts) == 3 {
		subdir = parts[2]
	}

	safeRef := strings.NewReplacer("/", "-", "\\", "-", ":", "-").Replace(ref)
	cacheDir := filepath.Join(actionsCache, strings.ToLower(owner), strings.ToLower(repo), safeRef)
	actionDir := cacheDir
	if subdir != "" {
		actionDir = filepath.Join(cacheDir, subdir)
	}

	// Return cached copy if manifest already present
	if hasActionManifest(actionDir) {
		return actionDir, nil
	}

	if err := os.MkdirAll(filepath.Dir(cacheDir), 0755); err != nil {
		return "", fmt.Errorf("failed to create actions cache: %w", err)
	}
	os.RemoveAll(cacheDir) // remove any partial clone

	cloneURL := fmt.Sprintf("https://github.com/%s/%s.git", owner, repo)

	// Try shallow clone on the specific branch/tag first
	cmd := exec.Command("git", "clone", "--depth=1", "--branch", ref, "--single-branch", cloneURL, cacheDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		// Fall back: shallow clone the default branch then fetch+checkout the ref (handles SHAs)
		os.RemoveAll(cacheDir)
		if out2, err2 := exec.Command("git", "clone", "--depth=1", cloneURL, cacheDir).CombinedOutput(); err2 != nil {
			return "", fmt.Errorf("failed to clone action %s/%s: %s", owner, repo, strings.TrimSpace(string(out2)))
		}
		_ = out
		exec.Command("git", "-C", cacheDir, "fetch", "--depth=1", "origin", ref).Run() //nolint:errcheck
		if out4, err4 := exec.Command("git", "-C", cacheDir, "checkout", ref).CombinedOutput(); err4 != nil {
			// Last resort: try FETCH_HEAD
			if out5, err5 := exec.Command("git", "-C", cacheDir, "checkout", "FETCH_HEAD").CombinedOutput(); err5 != nil {
				return "", fmt.Errorf("failed to checkout %s in action %s/%s: %s", ref, owner, repo, strings.TrimSpace(string(out5)))
			}
			_ = out4
		}
	}

	if !hasActionManifest(actionDir) {
		return "", fmt.Errorf("action %s has no action.yml or action.yaml at expected path", uses)
	}
	return actionDir, nil
}

func hasActionManifest(dir string) bool {
	for _, name := range []string{"action.yml", "action.yaml"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return true
		}
	}
	return false
}

// parseActionManifest reads and parses action.yml or action.yaml in actionDir.
func parseActionManifest(actionDir string) (*ActionManifest, error) {
	for _, name := range []string{"action.yml", "action.yaml"} {
		data, err := os.ReadFile(filepath.Join(actionDir, name))
		if err != nil {
			continue
		}
		var m ActionManifest
		if err := yaml.Unmarshal(data, &m); err != nil {
			return nil, fmt.Errorf("invalid action manifest: %w", err)
		}
		return &m, nil
	}
	return nil, fmt.Errorf("no action.yml or action.yaml found in %s", actionDir)
}

// buildInputEnvVars converts action `with:` inputs to INPUT_* environment variables,
// resolving any ${{ secrets.X }} / ${{ vars.X }} expressions in values.
// Defaults from the action manifest are applied first and overridden by explicit values.
func buildInputEnvVars(with AnyStringMap, inputs map[string]ActionInput, vars, secrets map[string]string) map[string]string {
	env := make(map[string]string)
	for name, input := range inputs {
		if input.Default != "" {
			env[inputEnvKey(name)] = input.Default
		}
	}
	for name, val := range with {
		env[inputEnvKey(name)] = resolveExpressions(val, vars, secrets)
	}
	return env
}

// inputEnvKey mirrors the GitHub Actions toolkit convention:
// spaces â†’ underscores, hyphens kept as-is, then uppercased.
// e.g. "vercel-token" â†’ "INPUT_VERCEL-TOKEN"
// e.g. "node version" â†’ "INPUT_NODE_VERSION"
func inputEnvKey(name string) string {
	return "INPUT_" + strings.ToUpper(strings.ReplaceAll(name, " ", "_"))
}

// executeExternalAction handles arbitrary `uses:` actions in Docker-container mode.
// It fetches the action from GitHub, parses action.yml, then dispatches by type.
func (r *WorkflowRunner) executeExternalAction(
	ctx context.Context,
	containerID string,
	stepID string,
	step WorkflowStepDef,
	envMap map[string]string,
	secretValues []string,
	workspaceDir string,
	actionsCache string,
	workflowSvc *WorkflowService,
	repoVars, repoSecrets map[string]string,
) error {
	workflowSvc.AppendStepLog(stepID, fmt.Sprintf("Fetching action %s...\n", step.Uses))
	actionDir, err := fetchAction(step.Uses, actionsCache)
	if err != nil {
		return r.stepFailure(workflowSvc, stepID, err.Error())
	}

	manifest, err := parseActionManifest(actionDir)
	if err != nil {
		return r.stepFailure(workflowSvc, stepID, err.Error())
	}

	inputEnv := buildInputEnvVars(step.With, manifest.Inputs, repoVars, repoSecrets)
	mergedEnv := mergeStringMaps(envMap, inputEnv)

	// Set GITHUB_ACTION to the action ref so the toolkit can identify itself.
	mergedEnv["GITHUB_ACTION"] = step.Uses

	// Inject GITHUB_TOKEN from secrets if available (many actions need it for API calls).
	if mergedEnv["GITHUB_TOKEN"] == "" {
		if tok, ok := repoSecrets["GITHUB_TOKEN"]; ok && tok != "" {
			mergedEnv["GITHUB_TOKEN"] = tok
		}
	}

	using := strings.ToLower(manifest.Runs.Using)
	switch {
	case using == "docker":
		return r.executeDockerAction(ctx, stepID, manifest, actionDir, workspaceDir, mergedEnv, secretValues, workflowSvc)
	case strings.HasPrefix(using, "node"):
		return r.executeNodeAction(ctx, containerID, stepID, manifest, actionDir, workspaceDir, mergedEnv, secretValues, workflowSvc)
	case using == "composite":
		return r.executeCompositeAction(ctx, containerID, stepID, manifest, workspaceDir, actionsCache, mergedEnv, secretValues, workflowSvc, repoVars, repoSecrets)
	default:
		return r.stepFailure(workflowSvc, stepID, fmt.Sprintf("unsupported action type %q in %s", manifest.Runs.Using, step.Uses))
	}
}

// executeDockerAction runs a Docker-based action using the Docker SDK.
// The image is either pulled from a registry (docker://) or built from a Dockerfile.
func (r *WorkflowRunner) executeDockerAction(
	ctx context.Context,
	stepID string,
	manifest *ActionManifest,
	actionDir, workspaceDir string,
	envMap map[string]string,
	secretValues []string,
	workflowSvc *WorkflowService,
) error {
	if r.docker == nil {
		return r.stepFailure(workflowSvc, stepID, "Docker daemon unavailable; cannot run docker action")
	}

	imageRef := manifest.Runs.Image
	var containerImage string

	switch {
	case strings.HasPrefix(imageRef, "docker://"):
		containerImage = strings.TrimPrefix(imageRef, "docker://")
		workflowSvc.AppendStepLog(stepID, fmt.Sprintf("Pulling image %s...\n", containerImage))
		rc, err := r.docker.ImagePull(ctx, containerImage, dockerimage.PullOptions{})
		if err != nil {
			return r.stepFailure(workflowSvc, stepID, fmt.Sprintf("failed to pull %s: %v", containerImage, err))
		}
		io.Copy(io.Discard, rc) // drain pull output
		rc.Close()

	case imageRef == "Dockerfile" || strings.HasSuffix(strings.ToLower(imageRef), "/dockerfile"):
		containerImage = "-action-" + strings.ToLower(strings.ReplaceAll(filepath.Base(actionDir), " ", "-"))
		dockerfile := imageRef
		if imageRef == "Dockerfile" {
			dockerfile = "Dockerfile"
		}
		workflowSvc.AppendStepLog(stepID, fmt.Sprintf("Building image %s from %s...\n", containerImage, dockerfile))

		tarCtx, err := tarDirectory(actionDir)
		if err != nil {
			return r.stepFailure(workflowSvc, stepID, fmt.Sprintf("failed to create build context: %v", err))
		}
		defer tarCtx.Close()

		buildResp, err := r.docker.ImageBuild(ctx, tarCtx, dockerbuild.ImageBuildOptions{
			Dockerfile: dockerfile,
			Tags:       []string{containerImage},
			Remove:     true,
		})
		if err != nil {
			return r.stepFailure(workflowSvc, stepID, fmt.Sprintf("failed to build action image: %v", err))
		}
		var buildOut bytes.Buffer
		io.Copy(&buildOut, buildResp.Body)
		buildResp.Body.Close()
		workflowSvc.AppendStepLog(stepID, logSafeLines(maskSecrets(buildOut.String(), secretValues)))

	default:
		return r.stepFailure(workflowSvc, stepID, fmt.Sprintf("unsupported docker image reference %q", imageRef))
	}

	// Build entrypoint and args
	var entrypoint strslice
	if manifest.Runs.Entrypoint != "" {
		entrypoint = strslice{manifest.Runs.Entrypoint}
	}
	var args strslice
	for _, a := range manifest.Runs.Args {
		args = append(args, resolveExpressions(a, nil, nil))
	}

	envList := envMapToList(envMap)
	cfg := &dockercontainer.Config{
		Image:      containerImage,
		Env:        envList,
		WorkingDir: "/github/workspace",
	}
	if len(entrypoint) > 0 {
		cfg.Entrypoint = entrypoint
	}
	if len(args) > 0 {
		cfg.Cmd = args
	}

	hostCfg := &dockercontainer.HostConfig{
		Mounts: []mount.Mount{
			{
				Type:   mount.TypeBind,
				Source: workspaceDir,
				Target: "/github/workspace",
			},
		},
		NetworkMode: "bridge",
		Privileged:  false,
	}
	pidsLimit := int64(r.cfg.WorkflowContainerPidsLimit)
	if pidsLimit <= 0 {
		pidsLimit = 256
	}
	hostCfg.Resources.PidsLimit = &pidsLimit
	if r.cfg.WorkflowContainerNoNewPrivileges {
		hostCfg.SecurityOpt = append(hostCfg.SecurityOpt, "no-new-privileges:true")
	}
	if r.cfg.WorkflowContainerDropAllCaps {
		hostCfg.CapDrop = append(hostCfg.CapDrop, "ALL")
	}

	resp, err := r.docker.ContainerCreate(ctx, cfg, hostCfg, nil, nil, "")
	if err != nil {
		return r.stepFailure(workflowSvc, stepID, fmt.Sprintf("failed to create action container: %v", err))
	}
	defer r.removeContainer(ctx, resp.ID)

	if err := r.docker.ContainerStart(ctx, resp.ID, dockercontainer.StartOptions{}); err != nil {
		return r.stepFailure(workflowSvc, stepID, fmt.Sprintf("failed to start action container: %v", err))
	}

	statusCh, errCh := r.docker.ContainerWait(ctx, resp.ID, dockercontainer.WaitConditionNotRunning)
	var exitCode int64
	select {
	case waitErr := <-errCh:
		if waitErr != nil {
			return r.stepFailure(workflowSvc, stepID, fmt.Sprintf("container wait error: %v", waitErr))
		}
	case status := <-statusCh:
		exitCode = status.StatusCode
	}

	logReader, err := r.docker.ContainerLogs(ctx, resp.ID, dockercontainer.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
	})
	if err == nil {
		var logBuf bytes.Buffer
		stdcopy.StdCopy(&logBuf, &logBuf, logReader)
		logReader.Close()
		workflowSvc.AppendStepLog(stepID, logSafeLines(maskSecrets(logBuf.String(), secretValues)))
	}

	code := int(exitCode)
	workflowSvc.UpdateStepStatus(stepID, statusFromCode(code), &code)
	if code != 0 {
		return fmt.Errorf("action container exited with code %d", code)
	}
	return nil
}

// executeNodeAction runs a Node.js action inside the runner container.
// The action is accessible via the /actions-cache/ bind mount.
func (r *WorkflowRunner) executeNodeAction(
	ctx context.Context,
	containerID string,
	stepID string,
	manifest *ActionManifest,
	actionDir, workspaceDir string,
	envMap map[string]string,
	secretValues []string,
	workflowSvc *WorkflowService,
) error {
	mainFile := manifest.Runs.Main
	if mainFile == "" {
		return r.stepFailure(workflowSvc, stepID, "action has no 'main' entrypoint")
	}

	// The actionsCache directory is mounted at /actions-cache/ in the container.
	// actionDir is a sub-path of actionsCache; compute the relative suffix.
	// We derive the container path by replacing the host workspace root prefix.
	// Since actionDir was cloned into actionsCache (mounted at /actions-cache/),
	// we determine the relative path inside the cache.
	containerActionDir := toContainerActionPath(actionDir, workspaceDir)
	script := "node " + shellQuote(containerActionDir+"/"+mainFile)
	return r.executeRun(ctx, containerID, stepID, script, envMap, secretValues, workflowSvc)
}

// toContainerActionPath maps a host actionDir to its path inside the container.
// If the actionDir is under workspaceDir/_actions/, it maps to /workspace/_actions/...
// Otherwise it maps to /actions-cache/... (the global actions cache mount).
func toContainerActionPath(actionDir, workspaceDir string) string {
	// Check if action was staged into workspace
	actionsSubdir := filepath.Join(workspaceDir, "_actions")
	if rel, err := filepath.Rel(actionsSubdir, actionDir); err == nil && !strings.HasPrefix(rel, "..") {
		return "/workspace/_actions/" + filepath.ToSlash(rel)
	}
	// Derive from a common parent: actionsCache is workspaceRoot/actions-cache
	// actionDir is workspaceRoot/actions-cache/owner/repo/ref[/subdir]
	// We need to strip workspaceRoot/actions-cache and prepend /actions-cache
	// Since we don't have actionsCache path here, use the last 3+ segments heuristic
	// by finding "_actions-cache_" in the path â€“ or just use the basename approach.
	// Best effort: use a path segment approach on the known structure.
	parts := strings.Split(filepath.ToSlash(actionDir), "/")
	for i, p := range parts {
		if p == "actions-cache" && i+1 < len(parts) {
			return "/actions-cache/" + strings.Join(parts[i+1:], "/")
		}
	}
	// Fallback: stage into workspace
	return "/workspace/_actions/" + filepath.Base(actionDir)
}

// executeCompositeAction runs a composite action's steps inside the runner container.
func (r *WorkflowRunner) executeCompositeAction(
	ctx context.Context,
	containerID string,
	stepID string,
	manifest *ActionManifest,
	workspaceDir, actionsCache string,
	envMap map[string]string,
	secretValues []string,
	workflowSvc *WorkflowService,
	repoVars, repoSecrets map[string]string,
) error {
	workflowSvc.AppendStepLog(stepID, fmt.Sprintf("Running composite action (%d steps)...\n", len(manifest.Runs.Steps)))
	for i, cs := range manifest.Runs.Steps {
		stepEnv := mergeStringMaps(envMap, resolveEnvExpressions(cs.Env, repoVars, repoSecrets))

		log.Printf("composite action step %d: uses=%q run=%q", i+1, cs.Uses, cs.Run)

		if cs.Uses != "" {
			nestedStep := WorkflowStepDef{
				Name: cs.Name,
				Uses: cs.Uses,
				With: cs.With,
				Env:  cs.Env,
				If:   cs.If,
			}
			if err := r.executeExternalAction(ctx, containerID, stepID, nestedStep, stepEnv, secretValues, workspaceDir, actionsCache, workflowSvc, repoVars, repoSecrets); err != nil {
				return err
			}
		} else if cs.Run != "" {
			resolved := resolveExpressions(cs.Run, repoVars, repoSecrets)
			if err := r.executeRun(ctx, containerID, stepID, resolved, stepEnv, secretValues, workflowSvc); err != nil {
				return err
			}
		}
	}
	return nil
}

// executeExternalActionLocally runs a GitHub Action in local (no-Docker runner) mode.
func (r *WorkflowRunner) executeExternalActionLocally(
	ctx context.Context,
	stepID string,
	step WorkflowStepDef,
	envMap map[string]string,
	secretValues []string,
	workspaceDir, actionsCache string,
	workflowSvc *WorkflowService,
	repoVars, repoSecrets map[string]string,
) error {
	workflowSvc.AppendStepLog(stepID, fmt.Sprintf("Fetching action %s...\n", step.Uses))
	actionDir, err := fetchAction(step.Uses, actionsCache)
	if err != nil {
		return r.stepFailure(workflowSvc, stepID, err.Error())
	}

	manifest, err := parseActionManifest(actionDir)
	if err != nil {
		return r.stepFailure(workflowSvc, stepID, err.Error())
	}

	inputEnv := buildInputEnvVars(step.With, manifest.Inputs, repoVars, repoSecrets)
	mergedEnv := mergeStringMaps(envMap, inputEnv)

	// Set GITHUB_ACTION to the action ref so the toolkit can identify itself.
	mergedEnv["GITHUB_ACTION"] = step.Uses

	// Inject GITHUB_TOKEN from secrets if available.
	if mergedEnv["GITHUB_TOKEN"] == "" {
		if tok, ok := repoSecrets["GITHUB_TOKEN"]; ok && tok != "" {
			mergedEnv["GITHUB_TOKEN"] = tok
		}
	}

	using := strings.ToLower(manifest.Runs.Using)
	switch {
	case strings.HasPrefix(using, "node"):
		mainFile := manifest.Runs.Main
		if mainFile == "" {
			return r.stepFailure(workflowSvc, stepID, "action has no 'main' entrypoint")
		}
		script := "node " + shellQuote(filepath.Join(actionDir, mainFile))
		return r.executeRunLocally(ctx, stepID, script, mergedEnv, secretValues, workspaceDir, workflowSvc)

	case using == "docker":
		return r.executeDockerActionLocally(ctx, stepID, manifest, actionDir, workspaceDir, mergedEnv, secretValues, workflowSvc)

	case using == "composite":
		return r.executeCompositeActionLocally(ctx, stepID, manifest, workspaceDir, actionsCache, mergedEnv, secretValues, workflowSvc, repoVars, repoSecrets)

	default:
		return r.stepFailure(workflowSvc, stepID, fmt.Sprintf("unsupported action type %q in %s", manifest.Runs.Using, step.Uses))
	}
}

// executeDockerActionLocally runs a Docker action using the host docker CLI (local mode).
func (r *WorkflowRunner) executeDockerActionLocally(
	ctx context.Context,
	stepID string,
	manifest *ActionManifest,
	actionDir, workspaceDir string,
	envMap map[string]string,
	secretValues []string,
	workflowSvc *WorkflowService,
) error {
	imageRef := manifest.Runs.Image
	var containerImage string

	switch {
	case strings.HasPrefix(imageRef, "docker://"):
		containerImage = strings.TrimPrefix(imageRef, "docker://")
		pullOut, err := exec.CommandContext(ctx, "docker", "pull", containerImage).CombinedOutput()
		workflowSvc.AppendStepLog(stepID, logSafeLines(string(pullOut)))
		if err != nil {
			return r.stepFailure(workflowSvc, stepID, fmt.Sprintf("docker pull %s failed", containerImage))
		}

	case imageRef == "Dockerfile" || strings.HasSuffix(strings.ToLower(imageRef), "/dockerfile"):
		containerImage = "-action-" + strings.ToLower(strings.ReplaceAll(filepath.Base(actionDir), " ", "-"))
		buildOut, err := exec.CommandContext(ctx, "docker", "build", "-t", containerImage, "-f", filepath.Join(actionDir, imageRef), actionDir).CombinedOutput()
		workflowSvc.AppendStepLog(stepID, logSafeLines(maskSecrets(string(buildOut), secretValues)))
		if err != nil {
			return r.stepFailure(workflowSvc, stepID, fmt.Sprintf("docker build failed: %v", err))
		}

	default:
		return r.stepFailure(workflowSvc, stepID, fmt.Sprintf("unsupported docker image ref: %s", imageRef))
	}

	dockerArgs := []string{"run", "--rm",
		"-v", workspaceDir + ":/github/workspace",
		"-w", "/github/workspace",
	}
	if pids := r.cfg.WorkflowContainerPidsLimit; pids > 0 {
		dockerArgs = append(dockerArgs, "--pids-limit", fmt.Sprintf("%d", pids))
	}
	if r.cfg.WorkflowContainerNoNewPrivileges {
		dockerArgs = append(dockerArgs, "--security-opt", "no-new-privileges:true")
	}
	if r.cfg.WorkflowContainerDropAllCaps {
		dockerArgs = append(dockerArgs, "--cap-drop", "ALL")
	}
	for k, v := range envMap {
		dockerArgs = append(dockerArgs, "-e", k+"="+v)
	}
	if manifest.Runs.Entrypoint != "" {
		dockerArgs = append(dockerArgs, "--entrypoint", manifest.Runs.Entrypoint)
	}
	dockerArgs = append(dockerArgs, containerImage)
	for _, a := range manifest.Runs.Args {
		dockerArgs = append(dockerArgs, resolveExpressions(a, nil, nil))
	}

	cmd := exec.CommandContext(ctx, "docker", dockerArgs...)
	cmd.Dir = workspaceDir
	cmd.Env = append(os.Environ(), envMapToList(envMap)...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	runErr := cmd.Run()
	workflowSvc.AppendStepLog(stepID, logSafeLines(maskSecrets(out.String(), secretValues)))

	exitCode := 0
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}
	workflowSvc.UpdateStepStatus(stepID, statusFromCode(exitCode), &exitCode)
	if exitCode != 0 {
		return fmt.Errorf("docker action exited with code %d", exitCode)
	}
	return nil
}

// executeCompositeActionLocally runs a composite action's steps in local (no-Docker) mode.
func (r *WorkflowRunner) executeCompositeActionLocally(
	ctx context.Context,
	stepID string,
	manifest *ActionManifest,
	workspaceDir, actionsCache string,
	envMap map[string]string,
	secretValues []string,
	workflowSvc *WorkflowService,
	repoVars, repoSecrets map[string]string,
) error {
	workflowSvc.AppendStepLog(stepID, fmt.Sprintf("Running composite action (%d steps)...\n", len(manifest.Runs.Steps)))
	for i, cs := range manifest.Runs.Steps {
		stepEnv := mergeStringMaps(envMap, resolveEnvExpressions(cs.Env, repoVars, repoSecrets))
		log.Printf("composite action step %d: uses=%q run=%q", i+1, cs.Uses, cs.Run)

		if cs.Uses != "" {
			nestedStep := WorkflowStepDef{
				Name: cs.Name,
				Uses: cs.Uses,
				With: cs.With,
				Env:  cs.Env,
			}
			if err := r.executeExternalActionLocally(ctx, stepID, nestedStep, stepEnv, secretValues, workspaceDir, actionsCache, workflowSvc, repoVars, repoSecrets); err != nil {
				return err
			}
		} else if cs.Run != "" {
			resolved := resolveExpressions(cs.Run, repoVars, repoSecrets)
			if err := r.executeRunLocally(ctx, stepID, resolved, stepEnv, secretValues, workspaceDir, workflowSvc); err != nil {
				return err
			}
		}
	}
	return nil
}

// stepFailure logs an error message and marks the step as failed.
func (r *WorkflowRunner) stepFailure(workflowSvc *WorkflowService, stepID string, reason string) error {
	workflowSvc.AppendStepLog(stepID, "Error: "+reason+"\n")
	one := 1
	workflowSvc.UpdateStepStatus(stepID, "failure", &one)
	return fmt.Errorf("%s", reason)
}

// copyDir recursively copies the src directory tree into dst.
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// shellQuote wraps a path in single quotes for safe use in a shell command.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

// tarDirectory creates an in-memory tar archive of the given directory using only stdlib.
func tarDirectory(srcDir string) (io.ReadCloser, error) {
	pr, pw := io.Pipe()
	go func() {
		tw := tar.NewWriter(pw)
		err := filepath.Walk(srcDir, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			rel, err := filepath.Rel(srcDir, path)
			if err != nil {
				return err
			}
			if rel == "." {
				return nil
			}
			hdr, err := tar.FileInfoHeader(info, "")
			if err != nil {
				return err
			}
			hdr.Name = rel
			if info.IsDir() {
				hdr.Name += "/"
			}
			if err := tw.WriteHeader(hdr); err != nil {
				return err
			}
			if !info.IsDir() {
				f, err := os.Open(path)
				if err != nil {
					return err
				}
				defer f.Close()
				_, err = io.Copy(tw, f)
				return err
			}
			return nil
		})
		tw.Close()
		pw.CloseWithError(err)
	}()
	return pr, nil
}

// strslice is a type alias for []string used in Docker container configs.
type strslice = []string

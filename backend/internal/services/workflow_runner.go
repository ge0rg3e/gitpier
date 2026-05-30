package services

import (
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
	"time"

	dockercontainer "github.com/docker/docker/api/types/container"
	dockerimage "github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	dockerclient "github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"

	"gitpier/internal/config"
	"gitpier/internal/models"
)

// WorkflowRunner executes workflow jobs inside Docker containers via a Docker daemon
// (expected to be a Docker-in-Docker sidecar accessible at cfg.DockerHost).
type WorkflowRunner struct {
	docker            *dockerclient.Client
	cfg               *config.Config
	releaseSvc        *ReleaseService
	runnerImagePullMu sync.Mutex
	runnerImagePulled bool
}

// SetReleaseService wires the ReleaseService so the runner can upload assets
// via the gitpier/upload-release-asset built-in action.
func (r *WorkflowRunner) SetReleaseService(svc *ReleaseService) {
	r.releaseSvc = svc
}

// NewWorkflowRunner connects to the Docker daemon with exponential backoff.
// When DockerHost is empty, the default local socket is tried once.
// If Docker is unavailable, a runner is still returned but jobs are blocked
// later by RunJob for security reasons.
func NewWorkflowRunner(cfg *config.Config) (*WorkflowRunner, error) {
	// When no explicit host is configured, try the default local socket once.
	if cfg.DockerHost == "" {
		opts := []dockerclient.Opt{dockerclient.WithAPIVersionNegotiation()}
		cli, err := dockerclient.NewClientWithOpts(opts...)
		if err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			_, pingErr := cli.Ping(ctx)
			cancel()
			if pingErr == nil {
				log.Printf("workflow runner connected to local Docker daemon")
				return &WorkflowRunner{docker: cli, cfg: cfg}, nil
			}
			cli.Close()
			log.Printf("workflow runner: local Docker not available (%v)", pingErr)
		}
		return &WorkflowRunner{docker: nil, cfg: cfg}, nil
	}

	var cli *dockerclient.Client
	var err error

	for attempt := 0; attempt < 10; attempt++ {
		cli, err = dockerclient.NewClientWithOpts(
			dockerclient.WithHost(cfg.DockerHost),
			dockerclient.WithAPIVersionNegotiation(),
		)
		if err != nil {
			time.Sleep(time.Duration(1<<attempt) * 200 * time.Millisecond)
			continue
		}
		// Quick ping to verify connectivity
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		_, pingErr := cli.Ping(ctx)
		cancel()
		if pingErr == nil {
			log.Printf("workflow runner connected to Docker daemon at %s", cfg.DockerHost)
			return &WorkflowRunner{docker: cli, cfg: cfg}, nil
		}
		cli.Close()
		log.Printf("workflow runner: docker not ready (attempt %d/10): %v", attempt+1, pingErr)
		time.Sleep(time.Duration(1<<attempt) * 500 * time.Millisecond)
	}

	log.Printf("workflow runner: could not connect to Docker daemon at %s", cfg.DockerHost)
	return &WorkflowRunner{docker: nil, cfg: cfg}, nil
}

// ensureDocker ensures the runner has a live Docker client, attempting a
// reconnect when the client is nil or stale.
func (r *WorkflowRunner) ensureDocker(ctx context.Context) error {
	if r.docker != nil {
		pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		_, err := r.docker.Ping(pingCtx)
		cancel()
		if err == nil {
			return nil
		}
		_ = r.docker.Close()
		r.docker = nil
	}

	opts := []dockerclient.Opt{dockerclient.WithAPIVersionNegotiation()}
	if r.cfg.DockerHost != "" {
		opts = append([]dockerclient.Opt{
			dockerclient.WithHost(r.cfg.DockerHost),
			dockerclient.WithAPIVersionNegotiation(),
		})
	}

	cli, err := dockerclient.NewClientWithOpts(opts...)
	if err != nil {
		return err
	}
	pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	_, pingErr := cli.Ping(pingCtx)
	cancel()
	if pingErr != nil {
		_ = cli.Close()
		return pingErr
	}
	r.docker = cli
	return nil
}

// RunJob clones the repository, spins up a container, executes every step,
// and cleans up when done. It updates job/step status in the DB via workflowSvc.
func (r *WorkflowRunner) RunJob(
	ctx context.Context,
	job *models.WorkflowJob,
	jobDef WorkflowJobDef,
	workflowEnv map[string]string,
	repoVars map[string]string,
	repoSecrets map[string]string,
	ownerUsername, repoName, commitSHA, refName, event string,
	workflowSvc *WorkflowService,
) error {
	workflowSvc.UpdateJobStatus(job.ID, "running")

	workspaceRoot := r.cfg.WorkflowWorkspacePath
	artifactsDir := filepath.Join(workspaceRoot, "artifacts", fmt.Sprintf("run%s", job.ID))
	// Use a temp workspace for job execution
	workspaceDir := filepath.Join(workspaceRoot, "tmp", fmt.Sprintf("run%s", job.ID))
	// Shared actions cache so action repos are only cloned once across all runs
	actionsCache := filepath.Join(workspaceRoot, "actions-cache")
	// Remove any leftover workspace from a previous (possibly failed) run.
	// Do NOT pre-create workspaceDir â€” git clone requires the target to be absent or empty.
	os.RemoveAll(workspaceDir)
	_ = os.MkdirAll(filepath.Join(workspaceRoot, "tmp"), 0755)
	os.MkdirAll(actionsCache, 0755)

	// Clone the bare repo into the temp workspace first, then set up helper files.
	repoPath := r.cfg.ReposPath + "/" + ownerUsername + "/" + repoName + ".git"
	if err := r.cloneRepo(repoPath, workspaceDir, commitSHA); err != nil {
		return r.failJob(workflowSvc, job.ID, fmt.Sprintf("git clone failed: %v", err))
	}

	// Create .github/ helper files inside the cloned workspace.
	ghFilePaths := setupGithubFiles(workspaceDir)

	// Build merged env: workflow â†’ job â†’ CI defaults
	// Resolve ${{ vars.X }} and ${{ secrets.X }} expressions first, then inject
	// all variables and secrets as plain env vars so they're available without
	// any special syntax too (mirrors GitHub Actions behaviour).
	resolvedWorkflowEnv := resolveEnvExpressions(workflowEnv, repoVars, repoSecrets)
	resolvedJobEnv := resolveEnvExpressions(jobDef.Env, repoVars, repoSecrets)

	mergedEnv := buildEnv(resolvedWorkflowEnv, resolvedJobEnv, ownerUsername, repoName, commitSHA, refName, event, ghFilePaths)

	// Inject all variables (vars.*) and secrets (*) into the environment
	for k, v := range repoVars {
		mergedEnv[k] = v
	}
	for k, v := range repoSecrets {
		mergedEnv[k] = v
	}

	// Collect secret values for log masking
	secretValues := make([]string, 0, len(repoSecrets))
	for _, v := range repoSecrets {
		if v != "" {
			secretValues = append(secretValues, v)
		}
	}

	// If Docker is unavailable, DO NOT run workflows - it's a security critical failure.
	// Running arbitrary user code directly on the host is extremely dangerous.
	if err := r.ensureDocker(ctx); err != nil {
		return r.failJob(workflowSvc, job.ID, "Docker daemon unavailable - workflows cannot run for security reasons. Please ensure Docker is running and the server has access.")
	}
	if err := r.ensureRunnerImagePulled(ctx); err != nil {
		return r.failJob(workflowSvc, job.ID, fmt.Sprintf("failed to pull runner image %s: %v", r.cfg.WorkflowRunnerImage, err))
	}

	// Create the runner container
	containerID, err := r.createContainer(ctx, workspaceDir, actionsCache, mergedEnv)
	if err != nil {
		return r.failJob(workflowSvc, job.ID, fmt.Sprintf("failed to create container: %v", err))
	}
	defer r.removeContainer(ctx, containerID)

	// Start container
	if err := r.docker.ContainerStart(ctx, containerID, dockercontainer.StartOptions{}); err != nil {
		return r.failJob(workflowSvc, job.ID, fmt.Sprintf("failed to start container: %v", err))
	}

	// Load step rows (in order)
	var steps []models.WorkflowStep
	if err := workflowSvc.db.Where("job_id = ?", job.ID).Order("created_at asc").Find(&steps).Error; err != nil {
		return r.failJob(workflowSvc, job.ID, "failed to load steps")
	}

	jobFailed := false

	for i, step := range steps {
		if i >= len(jobDef.Steps) {
			break
		}
		stepDef := jobDef.Steps[i]

		if jobFailed {
			workflowSvc.UpdateStepStatus(step.ID, "skipped", nil)
			continue
		}

		workflowSvc.UpdateStepStatus(step.ID, "running", nil)

		resolvedStepEnv := resolveEnvExpressions(stepDef.Env, repoVars, repoSecrets)
		stepEnv := mergeStringMaps(mergedEnv, resolvedStepEnv)
		var stepErr error

		if stepDef.Uses != "" {
			stepErr = r.executeUses(ctx, containerID, step.ID, stepDef, stepEnv, secretValues, workspaceDir, actionsCache, workflowSvc, repoVars, repoSecrets)
		} else if stepDef.Run != "" {
			resolvedRun := resolveExpressions(stepDef.Run, repoVars, repoSecrets)
			stepErr = r.executeRun(ctx, containerID, step.ID, resolvedRun, stepEnv, secretValues, workflowSvc)
		} else {
			workflowSvc.UpdateStepStatus(step.ID, "skipped", nil)
			continue
		}

		// Read GITHUB_ENV written by the step and propagate to subsequent steps.
		if envFile, ok := mergedEnv["GITHUB_ENV"]; ok {
			// Map container path back to host path
			hostEnvFile := strings.Replace(envFile, "/workspace/.github/", filepath.Join(workspaceDir, ".github")+"/", 1)
			if updates := readAndClearEnvFile(hostEnvFile); len(updates) > 0 {
				for k, v := range updates {
					mergedEnv[k] = v
				}
			}
		}

		if stepErr != nil {
			jobFailed = true
			code := 1
			workflowSvc.UpdateStepStatus(step.ID, "failure", &code)
		}
	}

	finalStatus := "success"
	if jobFailed {
		finalStatus = "failure"
	}
	workflowSvc.UpdateJobStatus(job.ID, finalStatus)
	// After job, copy outputs to artifactsDir
	copyJobOutputsToArtifacts(workspaceDir, artifactsDir)
	return nil
}

// ensureRunnerImagePulled pulls the configured runner image once per process start.
// This targets whichever daemon the runner is connected to (e.g. dind).
func (r *WorkflowRunner) ensureRunnerImagePulled(ctx context.Context) error {
	r.runnerImagePullMu.Lock()
	defer r.runnerImagePullMu.Unlock()
	if r.runnerImagePulled {
		return nil
	}

	imageRef := strings.TrimSpace(r.cfg.WorkflowRunnerImage)
	if imageRef == "" {
		return fmt.Errorf("workflow runner image is empty")
	}
	rc, err := r.docker.ImagePull(ctx, imageRef, dockerimage.PullOptions{})
	if err != nil {
		return err
	}
	defer rc.Close()
	_, _ = io.Copy(io.Discard, rc)
	r.runnerImagePulled = true
	return nil
}

// copyJobOutputsToArtifacts copies only non-dot, non-source, non-doc files from workspace to artifacts
func copyJobOutputsToArtifacts(workspaceDir, artifactsDir string) {
	os.MkdirAll(artifactsDir, 0755)
	files, _ := filepath.Glob(filepath.Join(workspaceDir, "*"))
	for _, f := range files {
		if info, err := os.Stat(f); err == nil && !info.IsDir() {
			base := filepath.Base(f)
			if base == ".git" || strings.HasPrefix(base, ".") {
				continue
			}
			if strings.HasSuffix(base, ".go") || strings.HasSuffix(base, ".md") ||
				strings.HasSuffix(base, ".mod") || strings.HasSuffix(base, ".sum") {
				continue
			}
			dstPath := filepath.Join(artifactsDir, base)
			if _, err := os.Stat(dstPath); err == nil {
				continue
			}
			src, _ := os.Open(f)
			dst, _ := os.Create(dstPath)
			io.Copy(dst, src)
			src.Close()
			dst.Close()
		}
	}
}

func (r *WorkflowRunner) cloneRepo(repoPath, workspaceDir, commitSHA string) error {
	// Validate commitSHA to prevent flag injection into git checkout.
	if _, err := safeSHA(commitSHA); err != nil {
		return fmt.Errorf("invalid commit SHA: %w", err)
	}
	// Clone from the bare repo on the local filesystem
	cloneCmd := exec.Command("git", "clone", "file://"+repoPath, workspaceDir)
	if out, err := cloneCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	// Checkout specific commit/branch
	checkoutCmd := exec.Command("git", "-C", workspaceDir, "checkout", commitSHA)
	if out, err := checkoutCmd.CombinedOutput(); err != nil {
		// Non-fatal if commit doesn't detach cleanly; log and continue
		log.Printf("git checkout %s warning: %s", commitSHA, strings.TrimSpace(string(out)))
	}
	// Strip origin URL to prevent leaking host path in .git/config
	stripCmd := exec.Command("git", "-C", workspaceDir, "remote", "set-url", "origin", "git@localhost:repository.git")
	stripCmd.Run()
	return nil
}

func (r *WorkflowRunner) createContainer(ctx context.Context, workspaceDir, actionsCache string, envMap map[string]string) (string, error) {
	runnerEnv := mergeStringMaps(envMap, map[string]string{})

	memory := int64(500 * 1024 * 1024) // 500 MB default
	nanoCPUs := int64(500_000_000)     // 0.5 CPU

	cfg := &dockercontainer.Config{
		Image:      r.cfg.WorkflowRunnerImage,
		Cmd:        []string{"/bin/sh", "-c", "while true; do sleep 30; done"},
		WorkingDir: "/workspace",
		User:       "0:0",
		Env:        nil,
	}

	mounts := []mount.Mount{
		{
			Type:   mount.TypeBind,
			Source: workspaceDir,
			Target: "/workspace",
		},
	}
	// GitHub Actions compatibility: provide a writable HOME path that marketplace
	// actions (including docker/login-action) can use for auth/config files.
	ghHomeHost := filepath.Join(workspaceDir, ".github", "home")
	_ = os.MkdirAll(ghHomeHost, 0777)
	_ = os.Chmod(ghHomeHost, 0777)
	mounts = append(mounts, mount.Mount{
		Type:   mount.TypeBind,
		Source: ghHomeHost,
		Target: "/github/home",
	})
	if strings.TrimSpace(runnerEnv["HOME"]) == "" {
		runnerEnv["HOME"] = "/github/home"
	}
	// Mount the shared actions cache so Node.js actions are accessible at /actions-cache/
	if actionsCache != "" {
		os.MkdirAll(actionsCache, 0755)
		mounts = append(mounts, mount.Mount{
			Type:   mount.TypeBind,
			Source: actionsCache,
			Target: "/actions-cache",
		})
	}
	socketMounted := false
	// Mounting /var/run/docker.sock into untrusted workflow containers enables host-level
	// Docker daemon control (container escape equivalent). Keep this disabled by default.
	if r.cfg.WorkflowAllowDockerSocket {
		const dockerSocketPath = "/var/run/docker.sock"
		if _, err := os.Stat(dockerSocketPath); err == nil {
			mounts = append(mounts, mount.Mount{
				Type:   mount.TypeBind,
				Source: dockerSocketPath,
				Target: dockerSocketPath,
			})
			socketMounted = true
			if strings.TrimSpace(runnerEnv["DOCKER_HOST"]) == "" {
				runnerEnv["DOCKER_HOST"] = "unix:///var/run/docker.sock"
			}
		}
	}
	// GitHub Actions compatibility: if DOCKER_HOST is configured for the runner
	// daemon (e.g. tcp://dind:2375), pass it into job containers so standard
	// workflows can run `docker` CLI without custom env overrides.
	if !socketMounted && strings.TrimSpace(runnerEnv["DOCKER_HOST"]) == "" && strings.TrimSpace(r.cfg.DockerHost) != "" {
		runnerEnv["DOCKER_HOST"] = strings.TrimSpace(r.cfg.DockerHost)
	}
	cfg.Env = envMapToList(runnerEnv)

	pidsLimit := int64(r.cfg.WorkflowContainerPidsLimit)
	if pidsLimit <= 0 {
		pidsLimit = 256
	}

	securityOpt := []string{}
	if r.cfg.WorkflowContainerNoNewPrivileges {
		securityOpt = append(securityOpt, "no-new-privileges:true")
	}

	capDrop := []string{}
	if r.cfg.WorkflowContainerDropAllCaps {
		capDrop = append(capDrop, "ALL")
	}

	hostCfg := &dockercontainer.HostConfig{
		Mounts: mounts,
		Resources: dockercontainer.Resources{
			Memory:    memory,
			NanoCPUs:  nanoCPUs,
			PidsLimit: &pidsLimit,
		},
		NetworkMode:    dockercontainer.NetworkMode(r.cfg.WorkflowContainerNetworkMode),
		ReadonlyRootfs: r.cfg.WorkflowContainerReadOnlyRootfs,
		SecurityOpt:    securityOpt,
		CapDrop:        capDrop,
		Privileged:     false,
		Tmpfs: map[string]string{
			"/tmp":     "rw,noexec,nosuid,nodev,size=128m",
			"/run":     "rw,noexec,nosuid,nodev,size=16m",
			"/var/tmp": "rw,noexec,nosuid,nodev,size=128m",
		},
	}

	resp, err := r.docker.ContainerCreate(ctx, cfg, hostCfg, nil, nil, "")
	if err != nil {
		return "", err
	}
	return resp.ID, nil
}

func (r *WorkflowRunner) removeContainer(ctx context.Context, containerID string) {
	timeout := 5
	_ = r.docker.ContainerStop(ctx, containerID, dockercontainer.StopOptions{Timeout: &timeout})
	_ = r.docker.ContainerRemove(ctx, containerID, dockercontainer.RemoveOptions{Force: true})
}

func (r *WorkflowRunner) executeRun(ctx context.Context, containerID string, stepID string, script string, envMap map[string]string, secretValues []string, workflowSvc *WorkflowService) error {
	envList := envMapToList(envMap)

	execResp, err := r.docker.ContainerExecCreate(ctx, containerID, dockercontainer.ExecOptions{
		Cmd:          []string{"/bin/bash", "-ce", script},
		WorkingDir:   "/workspace",
		AttachStdout: true,
		AttachStderr: true,
		Env:          envList,
	})
	if err != nil {
		return fmt.Errorf("exec create: %w", err)
	}

	hijack, err := r.docker.ContainerExecAttach(ctx, execResp.ID, dockercontainer.ExecAttachOptions{})
	if err != nil {
		return fmt.Errorf("exec attach: %w", err)
	}
	defer hijack.Close()

	var out bytes.Buffer
	if _, err := stdcopy.StdCopy(&out, &out, hijack.Reader); err != nil && err != io.EOF {
		log.Printf("stdcopy error: %v", err)
	}

	raw := maskSecrets(out.String(), secretValues)
	cleanLog, _, _ := parseLegacyWorkflowCommands(raw, secretValues)
	workflowSvc.AppendStepLog(stepID, logSafeLines(cleanLog))

	// Retrieve exit code
	inspect, err := r.docker.ContainerExecInspect(ctx, execResp.ID)
	if err != nil {
		return fmt.Errorf("exec inspect: %w", err)
	}

	exitCode := inspect.ExitCode
	workflowSvc.UpdateStepStatus(stepID, statusFromCode(exitCode), &exitCode)

	if exitCode != 0 {
		return fmt.Errorf("step exited with code %d", exitCode)
	}
	return nil
}

// executeUses dispatches built-in `uses:` action handlers; falls back to fetching
// arbitrary GitHub Actions for unknown action refs.
func (r *WorkflowRunner) executeUses(ctx context.Context, containerID string, stepID string, step WorkflowStepDef, envMap map[string]string, secretValues []string, workspaceDir, actionsCache string, workflowSvc *WorkflowService, repoVars, repoSecrets map[string]string) error {
	// Normalise: strip @version suffix for matching
	action := step.Uses
	if idx := strings.Index(action, "@"); idx != -1 {
		action = action[:idx]
	}
	action = strings.ToLower(action)

	switch action {
	case "actions/checkout":
		// Repo is already cloned before the container starts â€“ this is a no-op.
		workflowSvc.AppendStepLog(stepID, "Repository already checked out.\n")
		zero := 0
		workflowSvc.UpdateStepStatus(stepID, "success", &zero)
		return nil

	case "actions/setup-go":
		return r.executeSetupTool(ctx, containerID, stepID, "go", step.With["go-version"], envMap, secretValues, workflowSvc)

	case "actions/setup-node":
		ver := step.With["node-version"]
		if ver == "" {
			ver = step.With["node-version-file"]
		}
		return r.executeSetupTool(ctx, containerID, stepID, "node", ver, envMap, secretValues, workflowSvc)

	case "actions/setup-python":
		return r.executeSetupTool(ctx, containerID, stepID, "python", step.With["python-version"], envMap, secretValues, workflowSvc)

	case "actions/setup-java":
		ver := step.With["java-version"]
		if ver == "" {
			ver = "temurin-21"
		}
		return r.executeSetupTool(ctx, containerID, stepID, "java", ver, envMap, secretValues, workflowSvc)

	case "actions/setup-dotnet":
		return r.executeSetupTool(ctx, containerID, stepID, "dotnet", step.With["dotnet-version"], envMap, secretValues, workflowSvc)

	case "actions/cache",
		"actions/upload-artifact",
		"actions/download-artifact":
		msg := fmt.Sprintf("Action '%s' is not supported in gitpier and was skipped.\n", step.Uses)
		workflowSvc.AppendStepLog(stepID, msg)
		zero := 0
		workflowSvc.UpdateStepStatus(stepID, "success", &zero)
		return nil

	case "gitpier/upload-release-asset":
		return r.executeUploadReleaseAsset(ctx, stepID, step, workspaceDir, envMap, workflowSvc)

	default:
		// For any other action, attempt to fetch and execute it from GitHub.
		return r.executeExternalAction(ctx, containerID, stepID, step, envMap, secretValues, workspaceDir, actionsCache, workflowSvc, repoVars, repoSecrets)
	}
}

func (r *WorkflowRunner) executeSetupTool(ctx context.Context, containerID string, stepID string, tool, version string, envMap map[string]string, secretValues []string, workflowSvc *WorkflowService) error {
	if version == "" || version == "*" || version == "latest" {
		version = "latest"
	}
	spec := fmt.Sprintf("%s@%s", tool, version)
	script := fmt.Sprintf(`mise use --global %s && mise reshim`, spec)
	return r.executeRun(ctx, containerID, stepID, script, envMap, secretValues, workflowSvc)
}

// runJobLocally executes all steps for a job directly on the host without Docker.
func (r *WorkflowRunner) runJobLocally(
	ctx context.Context,
	job *models.WorkflowJob,
	jobDef WorkflowJobDef,
	mergedEnv map[string]string,
	repoVars map[string]string,
	repoSecrets map[string]string,
	secretValues []string,
	workspaceDir, artifactsDir, actionsCache string,
	workflowSvc *WorkflowService,
) error {
	// Override workspace path for local execution
	mergedEnv["GITHUB_WORKSPACE"] = workspaceDir

	// Set up .github/ helper files and override the container-side paths
	// with real host-side paths for local execution.
	localGhPaths := setupGithubFilesLocal(workspaceDir)
	for k, v := range localGhPaths {
		mergedEnv[k] = v
	}

	var steps []models.WorkflowStep
	if err := workflowSvc.db.Where("job_id = ?", job.ID).Order("created_at asc").Find(&steps).Error; err != nil {
		return r.failJob(workflowSvc, job.ID, "failed to load steps")
	}

	jobFailed := false

	for i, step := range steps {
		if i >= len(jobDef.Steps) {
			break
		}
		stepDef := jobDef.Steps[i]

		if jobFailed {
			workflowSvc.UpdateStepStatus(step.ID, "skipped", nil)
			continue
		}

		workflowSvc.UpdateStepStatus(step.ID, "running", nil)

		resolvedStepEnv := resolveEnvExpressions(stepDef.Env, repoVars, repoSecrets)
		stepEnv := mergeStringMaps(mergedEnv, resolvedStepEnv)
		var stepErr error

		if stepDef.Uses != "" {
			stepErr = r.executeUsesLocally(ctx, step.ID, stepDef, stepEnv, secretValues, workspaceDir, actionsCache, workflowSvc, repoVars, repoSecrets)
		} else if stepDef.Run != "" {
			resolvedRun := resolveExpressions(stepDef.Run, repoVars, repoSecrets)
			stepErr = r.executeRunLocally(ctx, step.ID, resolvedRun, stepEnv, secretValues, workspaceDir, workflowSvc)
		} else {
			workflowSvc.UpdateStepStatus(step.ID, "skipped", nil)
			continue
		}

		// Read GITHUB_ENV written by the step and propagate to subsequent steps.
		if envFile, ok := mergedEnv["GITHUB_ENV"]; ok {
			if updates := readAndClearEnvFile(envFile); len(updates) > 0 {
				for k, v := range updates {
					mergedEnv[k] = v
				}
			}
		}

		if stepErr != nil {
			jobFailed = true
			code := 1
			workflowSvc.UpdateStepStatus(step.ID, "failure", &code)
		}
	}

	finalStatus := "success"
	if jobFailed {
		finalStatus = "failure"
	}

	workflowSvc.UpdateJobStatus(job.ID, finalStatus)
	return nil
}

// executeRunLocally runs a shell script directly on the host in the workspace directory.
func (r *WorkflowRunner) executeRunLocally(
	ctx context.Context,
	stepID string,
	script string,
	envMap map[string]string,
	secretValues []string,
	workspaceDir string,
	workflowSvc *WorkflowService,
) error {
	// Replace the Docker-specific /workspace path with the actual workspace dir so
	// workflows that hardcode /workspace work correctly in local (no-Docker) mode.
	script = strings.ReplaceAll(script, "/workspace", workspaceDir)

	shell := "/bin/sh"
	if sh, err := exec.LookPath("bash"); err == nil {
		shell = sh
	}
	cmd := exec.CommandContext(ctx, shell, "-ce", script)
	cmd.Dir = workspaceDir

	// Build env with Go bin in PATH for locally installed tools
	baseEnv := os.Environ()
	// Add GOBIN/GOPATH/bin to PATH for go install'd binaries
	goBin := os.Getenv("GOBIN")
	if goBin == "" {
		goBin = filepath.Join(os.Getenv("HOME"), "go", "bin")
	}
	pathEntry := "PATH=" + goBin + ":" + os.Getenv("PATH")
	baseEnv = append(baseEnv, pathEntry)

	cmd.Env = append(baseEnv, envMapToList(envMap)...)

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	runErr := cmd.Run()
	raw := maskSecrets(out.String(), secretValues)
	cleanLog, _, _ := parseLegacyWorkflowCommands(raw, secretValues)
	workflowSvc.AppendStepLog(stepID, logSafeLines(cleanLog))

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
		return fmt.Errorf("step exited with code %d", exitCode)
	}
	return nil
}

// executeUsesLocally dispatches built-in uses: actions in local (no-Docker) mode.
// Unknown actions are fetched from GitHub and executed locally.
func (r *WorkflowRunner) executeUsesLocally(
	ctx context.Context,
	stepID string,
	step WorkflowStepDef,
	envMap map[string]string,
	secretValues []string,
	workspaceDir, actionsCache string,
	workflowSvc *WorkflowService,
	repoVars, repoSecrets map[string]string,
) error {
	action := step.Uses
	if idx := strings.Index(action, "@"); idx != -1 {
		action = action[:idx]
	}
	action = strings.ToLower(action)

	zero := 0
	switch action {
	case "actions/checkout":
		workflowSvc.AppendStepLog(stepID, "Repository already checked out.\n")
		workflowSvc.UpdateStepStatus(stepID, "success", &zero)
		return nil

	case "actions/cache",
		"actions/upload-artifact",
		"actions/download-artifact":
		msg := fmt.Sprintf("Action '%s' is not supported and was skipped.\n", step.Uses)
		workflowSvc.AppendStepLog(stepID, msg)
		workflowSvc.UpdateStepStatus(stepID, "success", &zero)
		return nil

	case "gitpier/upload-release-asset":
		return r.executeUploadReleaseAsset(ctx, stepID, step, workspaceDir, envMap, workflowSvc)

	case "actions/setup-go",
		"actions/setup-node",
		"actions/setup-python",
		"actions/setup-java",
		"actions/setup-dotnet":
		msg := fmt.Sprintf("Action '%s' skipped in local mode â€” tool must already be installed on the host.\n", step.Uses)
		workflowSvc.AppendStepLog(stepID, msg)
		workflowSvc.UpdateStepStatus(stepID, "success", &zero)
		return nil

	default:
		// For any other action, attempt to fetch and execute it from GitHub.
		return r.executeExternalActionLocally(ctx, stepID, step, envMap, secretValues, workspaceDir, actionsCache, workflowSvc, repoVars, repoSecrets)
	}
}

func (r *WorkflowRunner) failJob(workflowSvc *WorkflowService, jobID string, reason string) error {
	var steps []models.WorkflowStep
	workflowSvc.db.Where("job_id = ?", jobID).Order("created_at asc").Find(&steps)
	workflowSvc.markJobFailed(jobID, steps, reason)
	return fmt.Errorf("%s", reason)
}

// setupGithubFiles creates the .github/ directory in workspaceDir and the
// standard helper files (env, output, step_summary, path).  Returns their
// paths as a map of env var name â†’ container-internal path.
func setupGithubFiles(workspaceDir string) map[string]string {
	ghDir := filepath.Join(workspaceDir, ".github")
	os.MkdirAll(ghDir, 0777)
	_ = os.Chmod(ghDir, 0777)

	files := map[string]string{
		"GITHUB_ENV":          filepath.Join(ghDir, "env"),
		"GITHUB_OUTPUT":       filepath.Join(ghDir, "output"),
		"GITHUB_STEP_SUMMARY": filepath.Join(ghDir, "step_summary"),
		"GITHUB_PATH":         filepath.Join(ghDir, "path"),
	}
	for _, path := range files {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			os.WriteFile(path, nil, 0666) //nolint:errcheck
		}
		_ = os.Chmod(path, 0666)
	}

	// Return container-visible paths (workspace mounted at /workspace)
	containerPaths := map[string]string{
		"GITHUB_ENV":          "/workspace/.github/env",
		"GITHUB_OUTPUT":       "/workspace/.github/output",
		"GITHUB_STEP_SUMMARY": "/workspace/.github/step_summary",
		"GITHUB_PATH":         "/workspace/.github/path",
		"RUNNER_TEMP":         "/workspace/.github/tmp",
		"RUNNER_TOOL_CACHE":   "/workspace/.github/tool_cache",
	}
	os.MkdirAll(filepath.Join(ghDir, "tmp"), 0777)
	os.MkdirAll(filepath.Join(ghDir, "tool_cache"), 0777)
	_ = os.Chmod(filepath.Join(ghDir, "tmp"), 0777)
	_ = os.Chmod(filepath.Join(ghDir, "tool_cache"), 0777)
	return containerPaths
}

// setupGithubFilesLocal is like setupGithubFiles but returns host-side paths (local mode).
func setupGithubFilesLocal(workspaceDir string) map[string]string {
	ghDir := filepath.Join(workspaceDir, ".github")
	os.MkdirAll(ghDir, 0777)
	_ = os.Chmod(ghDir, 0777)
	os.MkdirAll(filepath.Join(ghDir, "tmp"), 0777)
	os.MkdirAll(filepath.Join(ghDir, "tool_cache"), 0777)
	_ = os.Chmod(filepath.Join(ghDir, "tmp"), 0777)
	_ = os.Chmod(filepath.Join(ghDir, "tool_cache"), 0777)
	for _, name := range []string{"env", "output", "step_summary", "path"} {
		p := filepath.Join(ghDir, name)
		if _, err := os.Stat(p); os.IsNotExist(err) {
			os.WriteFile(p, nil, 0666) //nolint:errcheck
		}
		_ = os.Chmod(p, 0666)
	}
	return map[string]string{
		"GITHUB_ENV":          filepath.Join(ghDir, "env"),
		"GITHUB_OUTPUT":       filepath.Join(ghDir, "output"),
		"GITHUB_STEP_SUMMARY": filepath.Join(ghDir, "step_summary"),
		"GITHUB_PATH":         filepath.Join(ghDir, "path"),
		"RUNNER_TEMP":         filepath.Join(ghDir, "tmp"),
		"RUNNER_TOOL_CACHE":   filepath.Join(ghDir, "tool_cache"),
	}
}

// readAndClearEnvFile reads the GITHUB_ENV file (multiline format) and
// clears it so subsequent steps start fresh.  Returns any new env vars found.
//
// The file format is either:
//
//	NAME=VALUE\n
//
// or (for multi-line values):
//
//	NAME<<DELIMITER\n
//	VALUE\n
//	DELIMITER\n
func readAndClearEnvFile(path string) map[string]string {
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return nil
	}
	// Truncate for next step
	os.WriteFile(path, nil, 0644) //nolint:errcheck

	result := make(map[string]string)
	lines := strings.Split(string(data), "\n")
	i := 0
	for i < len(lines) {
		line := lines[i]
		i++
		if line == "" {
			continue
		}
		if hd := strings.Index(line, "<<"); hd != -1 {
			// Heredoc multi-line value
			name := line[:hd]
			delimiter := line[hd+2:]
			var val strings.Builder
			for i < len(lines) {
				if lines[i] == delimiter {
					i++
					break
				}
				if val.Len() > 0 {
					val.WriteByte('\n')
				}
				val.WriteString(lines[i])
				i++
			}
			result[name] = val.String()
		} else if eq := strings.Index(line, "="); eq != -1 {
			result[line[:eq]] = line[eq+1:]
		}
	}
	return result
}

// parseLegacyWorkflowCommands scans action output for legacy ::set-env:: and
// ::add-mask:: commands and returns env updates and new secret values to mask.
// Lines that are pure workflow commands are stripped from the returned clean log.
func parseLegacyWorkflowCommands(raw string, secretValues []string) (cleanLog string, envUpdates map[string]string, newSecrets []string) {
	envUpdates = make(map[string]string)
	var logLines []string
	for _, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "::set-env name=") && strings.Contains(trimmed, "::") {
			// ::set-env name=KEY::VALUE
			rest := trimmed[len("::set-env name="):]
			if idx := strings.Index(rest, "::"); idx != -1 {
				key := rest[:idx]
				val := rest[idx+2:]
				envUpdates[key] = val
			}
			// Don't add to log â€” it's an internal command
			continue
		}
		if strings.HasPrefix(trimmed, "::add-mask::") {
			secret := trimmed[len("::add-mask::"):]
			if secret != "" && secret != "***" {
				alreadyKnown := false
				for _, s := range secretValues {
					if s == secret {
						alreadyKnown = true
						break
					}
				}
				if !alreadyKnown {
					newSecrets = append(newSecrets, secret)
				}
			}
			continue
		}
		logLines = append(logLines, line)
	}
	cleanLog = strings.Join(logLines, "\n")
	return
}

func buildEnv(workflowEnv, jobEnv map[string]string, ownerUsername, repoName, commitSHA, refName, event string, ghFilePaths map[string]string) map[string]string {
	owner := strings.TrimSpace(ownerUsername)
	repo := strings.TrimSpace(repoName)
	if owner == "" && repo != "" {
		owner = repo
	}
	if owner == "" {
		owner = "gitpier"
	}
	if repo == "" {
		repo = "repository"
	}

	refPrefix := "refs/heads/"
	if strings.TrimSpace(event) == "release" {
		refPrefix = "refs/tags/"
	}
	if strings.TrimSpace(event) == "" {
		event = "push"
	}

	env := map[string]string{
		"CI":                "true",
		"GITHUB_ACTIONS":    "true",
		"gitpier":           "true",
		"GITHUB_REPOSITORY": owner + "/" + repo,
		"GITHUB_ACTOR":      owner,
		"GITHUB_REF":        refPrefix + refName,
		"GITHUB_REF_NAME":   refName,
		"GITHUB_SHA":        commitSHA,
		"GITHUB_EVENT_NAME": event,
		"GITHUB_WORKSPACE":  "/workspace",
		"RUNNER_OS":         "Linux",
		"RUNNER_ARCH":       "X64",
	}
	for k, v := range ghFilePaths {
		env[k] = v
	}
	for k, v := range workflowEnv {
		env[k] = v
	}
	for k, v := range jobEnv {
		env[k] = v
	}
	return env
}

func mergeStringMaps(base, overlay map[string]string) map[string]string {
	merged := make(map[string]string, len(base)+len(overlay))
	for k, v := range base {
		merged[k] = v
	}
	for k, v := range overlay {
		merged[k] = v
	}
	return merged
}

func envMapToList(env map[string]string) []string {
	list := make([]string, 0, len(env))
	for k, v := range env {
		list = append(list, k+"="+v)
	}
	return list
}

func statusFromCode(code int) string {
	if code == 0 {
		return "success"
	}
	return "failure"
}

// resolveExpressions replaces ${{ vars.NAME }} and ${{ secrets.NAME }} in a string.
func resolveExpressions(s string, vars, secrets map[string]string) string {
	if !strings.Contains(s, "${{") {
		return s
	}
	// Replace ${{ vars.NAME }} â†’ variable value
	for k, v := range vars {
		s = strings.ReplaceAll(s, "${{ vars."+k+" }}", v)
		s = strings.ReplaceAll(s, "${{vars."+k+"}}", v)
	}
	// Replace ${{ secrets.NAME }} â†’ secret value
	for k, v := range secrets {
		s = strings.ReplaceAll(s, "${{ secrets."+k+" }}", v)
		s = strings.ReplaceAll(s, "${{secrets."+k+"}}", v)
	}
	return s
}

// resolveEnvExpressions resolves ${{ vars.X }} / ${{ secrets.X }} in all env map values.
func resolveEnvExpressions(env, vars, secrets map[string]string) map[string]string {
	if len(env) == 0 {
		return env
	}
	result := make(map[string]string, len(env))
	for k, v := range env {
		result[k] = resolveExpressions(v, vars, secrets)
	}
	return result
}

// maskSecrets replaces every occurrence of a secret value with *** in the given text.
// Empty secret values are never masked (avoids masking entire output).
func maskSecrets(text string, secretValues []string) string {
	for _, v := range secretValues {
		if v != "" {
			text = strings.ReplaceAll(text, v, "***")
		}
	}
	return text
}

// executeUploadReleaseAsset implements the gitpier/upload-release-asset built-in action.
// It reads `tag`, `path` (glob), and optional `name` from step.With, then calls
// ReleaseService.UploadAssetFromWorkspace. Owner and repo are extracted from the
// GITHUB_REPOSITORY env var set by the runner (format: "owner/repo").
func (r *WorkflowRunner) executeUploadReleaseAsset(
	ctx context.Context,
	stepID string,
	step WorkflowStepDef,
	workspaceDir string,
	envMap map[string]string,
	workflowSvc *WorkflowService,
) error {
	zero := 0

	if r.releaseSvc == nil {
		msg := "gitpier/upload-release-asset: release service not configured, skipping.\n"
		workflowSvc.AppendStepLog(stepID, msg)
		workflowSvc.UpdateStepStatus(stepID, "success", &zero)
		return nil
	}

	tag := step.With["tag"]
	path := step.With["path"]
	name := step.With["name"]

	if tag == "" || path == "" {
		err := fmt.Errorf("gitpier/upload-release-asset: 'tag' and 'path' inputs are required")
		workflowSvc.AppendStepLog(stepID, err.Error()+"\n")
		code := 1
		workflowSvc.UpdateStepStatus(stepID, "failure", &code)
		return err
	}

	// Parse owner/repo from GITHUB_REPOSITORY (set by buildEnv as "owner/repo")
	ghRepo := envMap["GITHUB_REPOSITORY"]
	parts := strings.SplitN(ghRepo, "/", 2)
	if len(parts) != 2 {
		err := fmt.Errorf("gitpier/upload-release-asset: could not determine repository from environment")
		workflowSvc.AppendStepLog(stepID, err.Error()+"\n")
		code := 1
		workflowSvc.UpdateStepStatus(stepID, "failure", &code)
		return err
	}
	ownerUsername := parts[0]
	repoName := parts[1]

	// The workspace path in the container is /workspace; replace with real path for local/docker
	resolvedPath := strings.ReplaceAll(path, "/workspace", workspaceDir)

	workflowSvc.AppendStepLog(stepID, fmt.Sprintf("Uploading release asset(s) for tag %s: %s\n", tag, path))

	if err := r.releaseSvc.UploadAssetFromWorkspace(ctx, ownerUsername, repoName, tag, resolvedPath, name, workspaceDir); err != nil {
		workflowSvc.AppendStepLog(stepID, fmt.Sprintf("Error: %v\n", err))
		code := 1
		workflowSvc.UpdateStepStatus(stepID, "failure", &code)
		return err
	}

	workflowSvc.AppendStepLog(stepID, "Asset(s) uploaded successfully.\n")
	workflowSvc.UpdateStepStatus(stepID, "success", &zero)
	return nil
}

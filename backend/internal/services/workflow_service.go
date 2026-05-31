package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gitpier/internal/config"
	"gitpier/internal/models"

	"gorm.io/gorm"
)

// WorkflowService manages workflow runs, jobs, steps, and usage limits.
type WorkflowService struct {
	db            *gorm.DB
	gitSvc        *GitService
	repoSvc       *RepoService
	runner        *WorkflowRunner
	cfg           *config.Config
	sem           chan struct{} // concurrency limiter
	repoEnvSvc    *RepoEnvService
	workspacePath string
}

func NewWorkflowService(db *gorm.DB, gitSvc *GitService, repoSvc *RepoService, runner *WorkflowRunner, cfg *config.Config, repoEnvSvc *RepoEnvService) *WorkflowService {
	sem := make(chan struct{}, cfg.WorkflowMaxConcurrentRuns)
	return &WorkflowService{
		db:            db,
		gitSvc:        gitSvc,
		repoSvc:       repoSvc,
		runner:        runner,
		cfg:           cfg,
		sem:           sem,
		repoEnvSvc:    repoEnvSvc,
		workspacePath: cfg.WorkflowWorkspacePath,
	}
}

// CancelStaleRuns marks any runs stuck in pending/running as cancelled (e.g. after a restart).
func (s *WorkflowService) CancelStaleRuns() {
	s.db.Model(&models.WorkflowRun{}).
		Where("status IN ?", []string{"pending", "running"}).
		Updates(map[string]interface{}{"status": "cancelled", "updated_at": time.Now()})

	s.db.Model(&models.WorkflowJob{}).
		Where("status IN ?", []string{"pending", "running"}).
		Updates(map[string]interface{}{"status": "cancelled"})

	s.db.Model(&models.WorkflowStep{}).
		Where("status IN ?", []string{"pending", "running"}).
		Updates(map[string]interface{}{"status": "cancelled"})
}

// TriggerWorkflows finds matching workflow files for an event and spawns runs asynchronously.
func (s *WorkflowService) TriggerWorkflows(ctx context.Context, repoID string, ownerUsername, repoName, event, refName, commitSHA, eventAction string) error {
	if err := s.ensureActionsMinutesAvailable(repoID); err != nil {
		log.Printf("workflow minutes limit reached for repo %s: %v", repoID, err)
		return err
	}

	repoPath := s.repoSvc.RepoPath(ownerUsername, repoName)

	files, err := FindWorkflowFiles(s.gitSvc, repoPath, commitSHA, event, refName, eventAction)
	if err != nil || len(files) == 0 {
		return nil
	}

	for _, wf := range files {
		run := &models.WorkflowRun{
			RepoID:       repoID,
			WorkflowName: wf.Def.Name,
			WorkflowFile: wf.Path,
			Event:        event,
			Branch:       refName,
			CommitSHA:    commitSHA,
			Status:       "pending",
		}
		if run.WorkflowName == "" {
			run.WorkflowName = wf.Path
		}
		if err := s.db.Create(run).Error; err != nil {
			log.Printf("failed to create workflow run: %v", err)
			continue
		}

		// Create job + step rows
		jobIDs := make(map[string]string)
		for jobKey, jobDef := range wf.Def.Jobs {
			jobName := jobDef.Name
			if jobName == "" {
				jobName = jobKey
			}
			job := &models.WorkflowJob{
				RunID:  run.ID,
				Name:   jobName,
				Status: "pending",
			}
			if err := s.db.Create(job).Error; err != nil {
				log.Printf("failed to create workflow job: %v", err)
				continue
			}
			jobIDs[jobKey] = job.ID

			for i, stepDef := range jobDef.Steps {
				step := &models.WorkflowStep{
					JobID:  job.ID,
					Name:   StepDisplayName(stepDef, i),
					Status: "pending",
				}
				if err := s.db.Create(step).Error; err != nil {
					log.Printf("failed to create workflow step: %v", err)
				}
			}
		}

		// Kick off run asynchronously
		capturedRun := run
		capturedWF := wf
		capturedOwner := ownerUsername
		capturedRepo := repoName
		capturedCommit := commitSHA
		capturedRefName := refName
		go s.executeRun(capturedRun, capturedWF, capturedOwner, capturedRepo, capturedCommit, capturedRefName)
	}

	return nil
}

func (s *WorkflowService) executeRun(run *models.WorkflowRun, wf WorkflowFile, ownerUsername, repoName, commitSHA, refName string) {
	runStarted := time.Now()
	defer func() {
		// Bill whole minutes using floor semantics so short runs (< 60s) cost 0 minutes.
		elapsedSeconds := time.Since(runStarted).Seconds()
		if elapsedSeconds < 0 {
			elapsedSeconds = 0
		}
		minutes := int(elapsedSeconds / 60.0)
		if err := s.addActionsMinutesUsage(run.RepoID, minutes); err != nil {
			log.Printf("failed to add actions minutes usage for repo %s: %v", run.RepoID, err)
		}
	}()

	// Acquire concurrency slot
	s.sem <- struct{}{}
	defer func() { <-s.sem }()

	if err := s.ensureActionsMinutesAvailable(run.RepoID); err != nil {
		s.updateRunStatus(run.ID, "cancelled")
		return
	}

	s.updateRunStatus(run.ID, "running")

	overallSuccess := true

	// Load repository variables and secrets for this run
	repoVars := map[string]string{}
	repoSecrets := map[string]string{}
	if s.repoEnvSvc != nil {
		repoVars = s.repoEnvSvc.GetVarsMap(run.RepoID)
		repoSecrets = s.repoEnvSvc.GetSecretsMap(run.RepoID)
	}

	// Load jobs for this run in order
	var jobs []models.WorkflowJob
	if err := s.db.Where("run_id = ?", run.ID).Order("id asc").Find(&jobs).Error; err != nil {
		s.updateRunStatus(run.ID, "failure")
		return
	}

	// Build job lookup
	jobByName := make(map[string]*models.WorkflowJob, len(jobs))
	// jobDefsByName is keyed by persisted job display name.
	jobDefsByName := make(map[string]WorkflowJobDef, len(jobs))
	// jobNameByKey maps workflow YAML job keys to persisted job names.
	jobNameByKey := make(map[string]string, len(jobs))
	for _, job := range jobs {
		jobByName[job.Name] = &job
	}
	for _, jwk := range orderedJobDefs(wf.Def.Jobs) {
		name := jwk.def.Name
		if name == "" {
			name = jwk.key
		}
		jobDefsByName[name] = jwk.def
		jobNameByKey[jwk.key] = name
	}
	// Fallback: if a persisted job name equals the YAML key directly.
	for _, job := range jobs {
		if _, ok := jobDefsByName[job.Name]; ok {
			continue
		}
		if def, ok := wf.Def.Jobs[job.Name]; ok {
			jobDefsByName[job.Name] = def
			jobNameByKey[job.Name] = job.Name
		}
	}

	// Build dependency graph and in-degree counts
	deps := make(map[string][]string) // job -> jobs it depends on
	inDegree := make(map[string]int, len(jobs))
	for _, job := range jobs {
		inDegree[job.Name] = 0
	}
	for _, job := range jobs {
		def, ok := jobDefsByName[job.Name]
		if !ok || len(def.Needs) == 0 {
			continue
		}
		for _, need := range def.Needs {
			needName := need
			if mapped, ok := jobNameByKey[need]; ok {
				needName = mapped
			}
			if _, exists := jobByName[needName]; exists {
				deps[needName] = append(deps[needName], job.Name)
				inDegree[job.Name]++
			}
		}
	}

	// Track completion
	results := make(map[string]string, len(jobs))
	var stateMu sync.Mutex

	// Process jobs in topological waves
	completed := make(map[string]bool)
	for len(completed) < len(jobs) {
		// Find all jobs with no remaining dependencies
		var ready []string
		var skipped []string
		for _, job := range jobs {
			name := job.Name
			if completed[name] {
				continue
			}
			def := jobDefsByName[name]
			if len(def.Needs) == 0 {
				ready = append(ready, name)
				continue
			}
			// Check if all dependencies succeeded
			allSucceeded := true
			allDone := true
			for _, need := range def.Needs {
				needName := need
				if mapped, ok := jobNameByKey[need]; ok {
					needName = mapped
				}
				if !completed[needName] {
					allDone = false
					break
				}
				if results[needName] != "success" {
					allSucceeded = false
				}
			}
			if allDone {
				if allSucceeded {
					ready = append(ready, name)
				} else {
					skipped = append(skipped, name)
				}
			}
		}

		if len(ready) == 0 && len(skipped) == 0 {
			// Circular dependency or all blocked - break
			break
		}

		// Mark skipped jobs
		for _, name := range skipped {
			job := jobByName[name]
			s.db.Model(&models.WorkflowJob{}).Where("id = ?", job.ID).Updates(map[string]interface{}{
				"status":      "skipped",
				"started_at":  time.Now(),
				"finished_at": time.Now(),
			})
			stateMu.Lock()
			results[name] = "skipped"
			completed[name] = true
			stateMu.Unlock()
		}

		// Run ready jobs in parallel
		var wg sync.WaitGroup
		for _, jobName := range ready {
			job := jobByName[jobName]
			def := jobDefsByName[jobName]

			wg.Add(1)
			go func(j *models.WorkflowJob, def WorkflowJobDef, jobName string) {
				defer wg.Done()

				// Load steps
				var steps []models.WorkflowStep
				if err := s.db.Where("job_id = ?", j.ID).Order("id asc").Find(&steps).Error; err != nil {
					s.markJobFailed(j.ID, steps, "failed to load steps")
					stateMu.Lock()
					results[jobName] = "failure"
					stateMu.Unlock()
					return
				}

				if s.runner == nil {
					s.markJobFailed(j.ID, steps, "workflow runner not initialised")
					stateMu.Lock()
					results[jobName] = "failure"
					stateMu.Unlock()
					return
				}

				err := s.runner.RunJob(context.Background(), j, def, wf.Def.Env, repoVars, repoSecrets, ownerUsername, repoName, commitSHA, refName, run.Event, s)
				stateMu.Lock()
				if err != nil {
					log.Printf("job %s (%s) error: %v", j.ID, j.Name, err)
					results[jobName] = "failure"
				} else {
					var updatedJob models.WorkflowJob
					s.db.Where("id = ?", j.ID).First(&updatedJob)
					if updatedJob.Status == "failure" {
						results[jobName] = "failure"
					} else {
						results[jobName] = "success"
					}
				}
				stateMu.Unlock()
			}(job, def, jobName)
		}

		wg.Wait()

		// Mark as completed
		for _, name := range ready {
			stateMu.Lock()
			if results[name] == "" {
				results[name] = "success" // default if no error
			}
			completed[name] = true
			stateMu.Unlock()
		}
	}

	// Mark jobs that never ran as skipped
	for _, job := range jobs {
		if !completed[job.Name] {
			s.db.Model(&models.WorkflowJob{}).Where("id = ?", job.ID).Updates(map[string]interface{}{
				"status":      "skipped",
				"started_at":  time.Now(),
				"finished_at": time.Now(),
			})
			stateMu.Lock()
			results[job.Name] = "skipped"
			stateMu.Unlock()
		}
	}

	// Determine final status
	for _, status := range results {
		if status == "failure" {
			overallSuccess = false
			break
		}
	}

	finalStatus := "success"
	if !overallSuccess {
		finalStatus = "failure"
	}
	s.updateRunStatus(run.ID, finalStatus)
}

// jobWithKey holds a job definition with its key for dependency lookup.
type jobWithKey struct {
	key string
	def WorkflowJobDef
}

// orderedJobDefs returns job definitions in insertion order (map iteration is random in Go,
// so we use alphabetical order as a stable fallback â€“ mirrors common CI behaviour).
// Returns both the definitions and their keys for dependency matching.
func orderedJobDefs(jobs map[string]WorkflowJobDef) []jobWithKey {
	keys := make([]string, 0, len(jobs))
	for k := range jobs {
		keys = append(keys, k)
	}
	// Stable sort: alphabetical
	for i := 0; i < len(keys)-1; i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[i] > keys[j] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	result := make([]jobWithKey, 0, len(keys))
	for _, k := range keys {
		result = append(result, jobWithKey{key: k, def: jobs[k]})
	}
	return result
}

func (s *WorkflowService) markJobFailed(jobID string, steps []models.WorkflowStep, reason string) {
	now := time.Now()
	s.db.Model(&models.WorkflowJob{}).Where("id = ?", jobID).Updates(map[string]interface{}{
		"status":      "failure",
		"started_at":  now,
		"finished_at": now,
	})
	for _, step := range steps {
		s.db.Model(&models.WorkflowStep{}).Where("id = ?", step.ID).Updates(map[string]interface{}{
			"status":      "failure",
			"log":         reason,
			"started_at":  now,
			"finished_at": now,
		})
	}
}

func (s *WorkflowService) UpdateJobStatus(jobID string, status string) {
	updates := map[string]interface{}{"status": status}
	if status == "running" {
		now := time.Now()
		updates["started_at"] = now
	} else if status == "success" || status == "failure" || status == "cancelled" {
		now := time.Now()
		updates["finished_at"] = now
	}
	s.db.Model(&models.WorkflowJob{}).Where("id = ?", jobID).Updates(updates)
}

func (s *WorkflowService) UpdateStepStatus(stepID string, status string, exitCode *int) {
	updates := map[string]interface{}{"status": status}
	if exitCode != nil {
		updates["exit_code"] = *exitCode
	}
	if status == "running" {
		now := time.Now()
		updates["started_at"] = now
	} else if status != "pending" {
		now := time.Now()
		updates["finished_at"] = now
	}
	s.db.Model(&models.WorkflowStep{}).Where("id = ?", stepID).Updates(updates)
}

// AppendStepLog appends text to the step's log field (thread-safe via DB row lock via UPDATE).
// For high-volume logs, a dedicated log table would be better; this suffices for our scale.
var stepLogMu sync.Mutex

func (s *WorkflowService) AppendStepLog(stepID string, text string) {
	stepLogMu.Lock()
	defer stepLogMu.Unlock()
	s.db.Exec("UPDATE workflow_steps SET log = COALESCE(log, '') || ? WHERE id = ?", text, stepID)
}

func (s *WorkflowService) updateRunStatus(runID string, status string) {
	s.db.Model(&models.WorkflowRun{}).Where("id = ?", runID).Updates(map[string]interface{}{
		"status":     status,
		"updated_at": time.Now(),
	})
}

func (s *WorkflowService) usageScopeForRepo(repoID string) (string, string, error) {
	var repo models.Repository
	if err := s.db.Select("id", "owner_id", "org_id").Where("id = ?", repoID).First(&repo).Error; err != nil {
		return "", "", err
	}
	if repo.OrgID != nil {
		return "org", *repo.OrgID, nil
	}
	return "user", repo.OwnerID, nil
}

func (s *WorkflowService) ensureActionsMinutesAvailable(repoID string) error {
	used, _, _, err := s.GetActionsUsageForRepo(repoID)
	if err != nil {
		return err
	}
	if used >= s.cfg.WorkflowMinutesLimitPerMonth {
		return fmt.Errorf("monthly actions minutes limit (%d) reached", s.cfg.WorkflowMinutesLimitPerMonth)
	}
	return nil
}

func (s *WorkflowService) addActionsMinutesUsage(repoID string, minutes int) error {
	if minutes <= 0 {
		return nil
	}
	scopeType, scopeID, err := s.usageScopeForRepo(repoID)
	if err != nil {
		return err
	}
	month := time.Now().UTC().Format("2006-01")

	result := s.db.Exec(`
		INSERT INTO workflow_minutes_usages (scope_type, scope_id, month, minutes_used)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (scope_type, scope_id, month) DO UPDATE
		  SET minutes_used = workflow_minutes_usages.minutes_used + EXCLUDED.minutes_used
	`, scopeType, scopeID, month, minutes)

	return result.Error
}

func (s *WorkflowService) GetActionsUsageForRepo(repoID string) (usedMinutes int, limitMinutes int, month string, err error) {
	scopeType, scopeID, err := s.usageScopeForRepo(repoID)
	if err != nil {
		return 0, 0, "", err
	}
	return s.getActionsUsageByScope(scopeType, scopeID)
}

func (s *WorkflowService) GetActionsUsageForUser(userID string) (usedMinutes int, limitMinutes int, month string, err error) {
	return s.getActionsUsageByScope("user", userID)
}

func (s *WorkflowService) GetActionsUsageForOrg(orgID string) (usedMinutes int, limitMinutes int, month string, err error) {
	return s.getActionsUsageByScope("org", orgID)
}

func (s *WorkflowService) getActionsUsageByScope(scopeType string, scopeID string) (usedMinutes int, limitMinutes int, month string, err error) {

	month = time.Now().UTC().Format("2006-01")
	usedMinutes = 0
	var row models.WorkflowMinutesUsage
	qErr := s.db.Where("scope_type = ? AND scope_id = ? AND month = ?", scopeType, scopeID, month).First(&row).Error
	if qErr == nil {
		usedMinutes = row.MinutesUsed
	} else if !errors.Is(qErr, gorm.ErrRecordNotFound) {
		return 0, 0, "", qErr
	}
	if usedMinutes < 0 {
		usedMinutes = 0
	}

	return usedMinutes, s.cfg.WorkflowMinutesLimitPerMonth, month, nil
}

func (s *WorkflowService) GetRunsByRepo(repoID string, limit, offset int) ([]models.WorkflowRun, int64, error) {
	var runs []models.WorkflowRun
	var total int64

	q := s.db.Model(&models.WorkflowRun{}).Where("repo_id = ?", repoID)
	q.Count(&total)

	err := q.Order("id desc").Limit(limit).Offset(offset).Find(&runs).Error
	return runs, total, err
}

func (s *WorkflowService) GetRun(runID string) (*models.WorkflowRun, error) {
	var run models.WorkflowRun
	err := s.db.
		Preload("Jobs", func(db *gorm.DB) *gorm.DB { return db.Order("id asc") }).
		Preload("Jobs.Steps", func(db *gorm.DB) *gorm.DB { return db.Order("id asc") }).
		Where("workflow_runs.id = ?", runID).
		First(&run).Error
	if err != nil {
		return nil, err
	}
	return &run, nil
}

// ListDispatchableWorkflows returns all workflow file paths present at the given ref.
// Any workflow can be manually dispatched regardless of its trigger configuration.
func (s *WorkflowService) ListDispatchableWorkflows(ownerUsername, repoName, ref string) ([]string, error) {
	repoPath := s.repoSvc.RepoPath(ownerUsername, repoName)
	files, err := FindAllWorkflowFiles(s.gitSvc, repoPath, ref)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(files))
	for _, f := range files {
		paths = append(paths, f.Path)
	}
	return paths, nil
}

// DispatchWorkflow manually triggers workflow_dispatch workflows in the repo at the given ref.
// If workflowFile is non-empty, only that specific workflow file is triggered.
// Returns the number of runs created.
func (s *WorkflowService) DispatchWorkflow(ctx context.Context, repoID string, ownerUsername, repoName, ref, workflowFile string) (int, error) {
	if err := s.ensureActionsMinutesAvailable(repoID); err != nil {
		return 0, err
	}

	repoPath := s.repoSvc.RepoPath(ownerUsername, repoName)

	// Resolve the ref to a concrete commit SHA.
	commit, err := s.gitSvc.GetHeadCommit(repoPath, ref)
	if err != nil {
		return 0, fmt.Errorf("could not resolve ref %q: %w", ref, err)
	}
	commitSHA := commit.SHA

	var files []WorkflowFile
	if workflowFile != "" {
		// Load the specific workflow file directly, bypassing trigger matching.
		content, err := s.gitSvc.GetBlob(repoPath, commitSHA, workflowFile)
		if err != nil {
			return 0, fmt.Errorf("workflow file %q not found on ref %q", workflowFile, ref)
		}
		wf, err := ParseWorkflow(content)
		if err != nil {
			return 0, fmt.Errorf("invalid workflow file %q: %w", workflowFile, err)
		}
		files = []WorkflowFile{{Path: workflowFile, Content: content, Def: wf}}
	} else {
		files, err = FindAllWorkflowFiles(s.gitSvc, repoPath, commitSHA)
		if err != nil || len(files) == 0 {
			return 0, nil
		}
	}

	created := 0
	for _, wf := range files {
		run := &models.WorkflowRun{
			RepoID:       repoID,
			WorkflowName: wf.Def.Name,
			WorkflowFile: wf.Path,
			Event:        "workflow_dispatch",
			Branch:       ref,
			CommitSHA:    commitSHA,
			Status:       "pending",
		}
		if run.WorkflowName == "" {
			run.WorkflowName = wf.Path
		}
		if err := s.db.Create(run).Error; err != nil {
			log.Printf("failed to create dispatch workflow run: %v", err)
			continue
		}

		jobIDs := make(map[string]string)
		for jobKey, jobDef := range wf.Def.Jobs {
			jobName := jobDef.Name
			if jobName == "" {
				jobName = jobKey
			}
			job := &models.WorkflowJob{
				RunID:  run.ID,
				Name:   jobName,
				Status: "pending",
			}
			if err := s.db.Create(job).Error; err != nil {
				log.Printf("failed to create dispatch workflow job: %v", err)
				continue
			}
			jobIDs[jobKey] = job.ID

			for i, stepDef := range jobDef.Steps {
				step := &models.WorkflowStep{
					JobID:  job.ID,
					Name:   StepDisplayName(stepDef, i),
					Status: "pending",
				}
				if err := s.db.Create(step).Error; err != nil {
					log.Printf("failed to create dispatch workflow step: %v", err)
				}
			}
		}

		capturedRun := run
		capturedWF := wf
		go s.executeRun(capturedRun, capturedWF, ownerUsername, repoName, commitSHA, ref)
		created++
	}

	return created, nil
}

// CancelRun marks a run and all its pending/running jobs+steps as cancelled.
func (s *WorkflowService) CancelRun(runID string) error {
	now := time.Now()
	if err := s.db.Model(&models.WorkflowRun{}).Where("id = ? AND status IN ?", runID, []string{"pending", "running"}).
		Updates(map[string]interface{}{"status": "cancelled", "updated_at": now}).Error; err != nil {
		return err
	}

	var jobIDs []string
	s.db.Model(&models.WorkflowJob{}).Where("run_id = ?", runID).Pluck("id", &jobIDs)

	if len(jobIDs) > 0 {
		s.db.Model(&models.WorkflowJob{}).
			Where("id IN ? AND status IN ?", jobIDs, []string{"pending", "running"}).
			Updates(map[string]interface{}{"status": "cancelled", "finished_at": now})

		s.db.Model(&models.WorkflowStep{}).
			Where("job_id IN ? AND status IN ?", jobIDs, []string{"pending", "running"}).
			Updates(map[string]interface{}{"status": "cancelled", "finished_at": now})
	}

	return nil
}

// RerunWorkflow resets an existing run, clearing logs and restarting.
func (s *WorkflowService) RerunWorkflow(ctx context.Context, runID string) (string, error) {
	var run models.WorkflowRun
	if err := s.db.Where("id = ?", runID).First(&run).Error; err != nil {
		return "", err
	}
	if err := s.ensureActionsMinutesAvailable(run.RepoID); err != nil {
		return "", err
	}

	// Load jobs and steps
	var jobs []models.WorkflowJob
	s.db.Where("run_id = ?", runID).Find(&jobs)

	// Reset all jobs to pending
	now := time.Now()
	for i := range jobs {
		jobs[i].Status = "pending"
		jobs[i].StartedAt = nil
		jobs[i].FinishedAt = nil
		s.db.Save(&jobs[i])

		// Reset all steps to pending and clear log
		s.db.Model(&models.WorkflowStep{}).
			Where("job_id = ?", jobs[i].ID).
			Updates(map[string]interface{}{
				"status":      "pending",
				"log":         "",
				"exit_code":   nil,
				"started_at":  nil,
				"finished_at": nil,
			})
	}

	// Reset run status
	s.db.Model(&models.WorkflowRun{}).Where("id = ?", runID).Updates(map[string]interface{}{
		"status":     "pending",
		"updated_at": now,
	})

	// Start execution asynchronously
	repo, err := s.repoSvc.GetByID(context.Background(), run.RepoID)
	if err != nil {
		return "", err
	}
	content, err := s.gitSvc.GetBlob(s.repoSvc.RepoPath(repo.Owner.Username, repo.Name), run.CommitSHA, run.WorkflowFile)
	if err != nil {
		return "", fmt.Errorf("workflow file not found: %w", err)
	}
	wf, err := ParseWorkflow(content)
	if err != nil {
		return "", fmt.Errorf("invalid workflow file: %w", err)
	}

	go s.executeRun(&run, WorkflowFile{Path: run.WorkflowFile, Content: content, Def: wf}, repo.Owner.Username, repo.Name, run.CommitSHA, run.Branch)

	return runID, nil
}

// DeleteRun deletes a workflow run and all its jobs and steps.
func (s *WorkflowService) DeleteRun(runID string) error {
	// Get all job IDs for this run
	var jobIDs []string
	s.db.Model(&models.WorkflowJob{}).Where("run_id = ?", runID).Pluck("id", &jobIDs)

	// Delete steps
	if len(jobIDs) > 0 {
		s.db.Where("job_id IN ?", jobIDs).Delete(&models.WorkflowStep{})
	}

	// Delete jobs
	s.db.Where("run_id = ?", runID).Delete(&models.WorkflowJob{})

	// Delete run
	s.db.Where("id = ?", runID).Delete(&models.WorkflowRun{})

	// Clean up stored workspace files from .data
	if s.workspacePath != "" {
		// Clean up run-level directories
		workspaceDir := filepath.Join(s.workspacePath, "tmp", fmt.Sprintf("run%s", runID))
		artifactsDir := filepath.Join(s.workspacePath, "artifacts", fmt.Sprintf("run%s", runID))
		os.RemoveAll(workspaceDir)
		os.RemoveAll(artifactsDir)

		// Clean up job-level directories
		for _, jobID := range jobIDs {
			jobWorkspaceDir := filepath.Join(s.workspacePath, "tmp", fmt.Sprintf("run%s", jobID))
			jobArtifactsDir := filepath.Join(s.workspacePath, "artifacts", fmt.Sprintf("run%s", jobID))
			os.RemoveAll(jobWorkspaceDir)
			os.RemoveAll(jobArtifactsDir)
		}
	}

	return nil
}

// logSafeLines removes null bytes (which break PostgreSQL text columns) and
// ANSI/VT100 escape sequences so logs render cleanly in the UI.
func logSafeLines(s string) string {
	s = strings.ReplaceAll(s, "\x00", "")
	// Strip ANSI escape sequences: CSI sequences \x1b[ ... final-byte and OSC \x1b] ... \x07/ST
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		if s[i] == '\x1b' && i+1 < len(s) {
			switch s[i+1] {
			case '[': // CSI sequence: \x1b[ ... [0x40-0x7e]
				j := i + 2
				for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
					j++
				}
				if j < len(s) {
					j++ // consume the final byte
				}
				i = j
				continue
			case ']': // OSC sequence: \x1b] ... \x07 or \x1b\\
				j := i + 2
				for j < len(s) {
					if s[j] == '\x07' {
						j++
						break
					}
					if s[j] == '\x1b' && j+1 < len(s) && s[j+1] == '\\' {
						j += 2
						break
					}
					j++
				}
				i = j
				continue
			case '(', ')', '*', '+', '-', '.', '/': // Charset designations
				i += 3
				continue
			default: // two-char escape (e.g. \x1bM reverse-index)
				i += 2
				continue
			}
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

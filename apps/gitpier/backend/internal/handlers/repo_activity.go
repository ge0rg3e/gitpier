package handlers

import (
	"gitpier/internal/models"
	"gitpier/internal/services"
	"sync"
)

const (
	repoListActivityDays    = 21
	repoListActivityWorkers = 6
)

func attachRepoActivitySeries(repoSvc *services.RepoService, gitSvc *services.GitService, namespace string, repos []models.Repository) {
	if len(repos) == 0 || namespace == "" {
		return
	}

	sem := make(chan struct{}, repoListActivityWorkers)
	var wg sync.WaitGroup

	for i := range repos {
		repo := &repos[i]
		wg.Add(1)
		sem <- struct{}{}

		go func(repo *models.Repository) {
			defer wg.Done()
			defer func() { <-sem }()

			series, err := gitSvc.GetRecentActivitySeries(repoSvc.RepoPath(namespace, repo.Name), repoListActivityDays)
			if err != nil || len(series) == 0 {
				repo.ActivitySeries = make([]int, repoListActivityDays)
				return
			}

			repo.ActivitySeries = series
		}(repo)
	}

	wg.Wait()
}

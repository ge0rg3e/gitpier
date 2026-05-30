package handlers

import (
	"fmt"
	"gitpier/internal/cache"
	"net/http"
	"strconv"
	"time"

	"gitpier/internal/models"
	"gitpier/internal/services"

	"github.com/labstack/echo/v4"
)

type SearchHandler struct {
	searchSvc *services.SearchService
	repoSvc   *services.RepoService
	cache     cache.Store
}

func NewSearchHandler(searchSvc *services.SearchService, repoSvc *services.RepoService, cacheStore cache.Store) *SearchHandler {
	return &SearchHandler{searchSvc: searchSvc, repoSvc: repoSvc, cache: cacheStore}
}

func (h *SearchHandler) Search(c echo.Context) error {
	q := c.QueryParam("q")
	if len(q) < 2 {
		return echo.NewHTTPError(http.StatusBadRequest, "query must be at least 2 characters")
	}
	if len(q) > 200 {
		return echo.NewHTTPError(http.StatusBadRequest, "query too long")
	}

	searchType := c.QueryParam("type")
	if searchType == "" {
		searchType = "repos"
	}

	limit := 20
	if l := c.QueryParam("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 50 {
			limit = n
		}
	}
	offset := 0
	if o := c.QueryParam("offset"); o != "" {
		if n, err := strconv.Atoi(o); err == nil && n >= 0 {
			offset = n
		}
	}

	ctx := c.Request().Context()

	switch searchType {
	case "repos":
		key := fmt.Sprintf("cache:search:repos:%s:%d:%d", q, limit, offset)
		body, err := h.cache.RememberJSON(ctx, key, 30*time.Second, func() (interface{}, error) {
			return h.searchSvc.SearchRepos(ctx, q, limit, offset)
		})
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "search failed")
		}
		return c.Blob(http.StatusOK, echo.MIMEApplicationJSONCharsetUTF8, body)

	case "users":
		key := fmt.Sprintf("cache:search:users:%s:%d:%d", q, limit, offset)
		body, err := h.cache.RememberJSON(ctx, key, 30*time.Second, func() (interface{}, error) {
			return h.searchSvc.SearchUsers(ctx, q, limit, offset)
		})
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "search failed")
		}
		return c.Blob(http.StatusOK, echo.MIMEApplicationJSONCharsetUTF8, body)

	case "orgs":
		key := fmt.Sprintf("cache:search:orgs:%s:%d:%d", q, limit, offset)
		body, err := h.cache.RememberJSON(ctx, key, 30*time.Second, func() (interface{}, error) {
			return h.searchSvc.SearchOrgs(ctx, q, limit, offset)
		})
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "search failed")
		}
		return c.Blob(http.StatusOK, echo.MIMEApplicationJSONCharsetUTF8, body)

	case "code", "files":
		owner := c.QueryParam("owner")
		repoName := c.QueryParam("repo")
		ref := c.QueryParam("ref")

		if owner == "" || repoName == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "owner and repo are required for code/file search")
		}

		repo, err := h.repoSvc.GetByOwnerAndName(ctx, owner, repoName)
		if err != nil {
			return echo.NewHTTPError(http.StatusNotFound, "repository not found")
		}

		// Access control: private repos require authentication
		if repo.IsPrivate {
			currentUser, _ := c.Get("user").(*models.User)
			if currentUser == nil || !h.repoSvc.HasAccess(repo, currentUser.ID, false) {
				return echo.NewHTTPError(http.StatusNotFound, "repository not found")
			}
		}

		repoPath := h.repoSvc.RepoPath(owner, repoName)

		if searchType == "code" {
			result, err := h.searchSvc.SearchCode(repoPath, ref, q)
			if err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, "code search failed")
			}
			return c.JSON(http.StatusOK, result)
		}

		result, err := h.searchSvc.SearchFiles(repoPath, ref, q)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "file search failed")
		}
		return c.JSON(http.StatusOK, result)

	default:
		return echo.NewHTTPError(http.StatusBadRequest, "invalid search type")
	}
}

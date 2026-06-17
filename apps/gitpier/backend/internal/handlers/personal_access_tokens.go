package handlers

import (
	"errors"
	"net/http"
	"time"

	"gitpier/internal/models"
	"gitpier/internal/services"

	"github.com/labstack/echo/v4"
)

type PersonalAccessTokenHandler struct {
	tokenSvc *services.PersonalAccessTokenService
}

func NewPersonalAccessTokenHandler(tokenSvc *services.PersonalAccessTokenService) *PersonalAccessTokenHandler {
	return &PersonalAccessTokenHandler{tokenSvc: tokenSvc}
}

func (h *PersonalAccessTokenHandler) List(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)

	tokens, err := h.tokenSvc.List(c.Request().Context(), currentUser.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list tokens")
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"tokens": tokens})
}

func (h *PersonalAccessTokenHandler) Create(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)

	var req struct {
		Name      string   `json:"name"`
		Scopes    []string `json:"scopes"`
		ExpiresAt string   `json:"expires_at"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	var expiresAt *time.Time
	if req.ExpiresAt != "" {
		parsed, err := time.Parse(time.RFC3339, req.ExpiresAt)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "expires_at must be RFC3339")
		}
		if !parsed.After(time.Now().UTC()) {
			return echo.NewHTTPError(http.StatusBadRequest, "expires_at must be in the future")
		}
		expiresAt = &parsed
	}

	created, err := h.tokenSvc.Create(c.Request().Context(), services.CreatePersonalAccessTokenInput{
		UserID:    currentUser.ID,
		Name:      req.Name,
		Scopes:    req.Scopes,
		ExpiresAt: expiresAt,
	})
	if err != nil {
		if errors.Is(err, services.ErrInvalidTokenScope) {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid token scope")
		}
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return c.JSON(http.StatusCreated, map[string]interface{}{
		"token":  created.Plaintext,
		"record": created.TokenRecord,
	})
}

func (h *PersonalAccessTokenHandler) Delete(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)
	tokenID := c.Param("id")
	if tokenID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid token ID")
	}

	if err := h.tokenSvc.Delete(c.Request().Context(), tokenID, currentUser.ID); err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "token not found")
	}

	return c.NoContent(http.StatusNoContent)
}

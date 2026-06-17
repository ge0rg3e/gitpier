package handlers

import (
	"errors"
	"gitpier/internal/models"
	"net/http"

	"gitpier/internal/services"

	"github.com/labstack/echo/v4"
)

type SSHKeyHandler struct {
	sshKeySvc *services.SSHKeyService
}

func NewSSHKeyHandler(sshKeySvc *services.SSHKeyService) *SSHKeyHandler {
	return &SSHKeyHandler{sshKeySvc: sshKeySvc}
}

func (h *SSHKeyHandler) List(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)

	keys, err := h.sshKeySvc.List(c.Request().Context(), currentUser.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list SSH keys")
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"keys": keys})
}

func (h *SSHKeyHandler) Add(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)

	var req struct {
		Title string `json:"title"`
		Key   string `json:"key"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	if req.Title == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "title is required")
	}
	if req.Key == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "key is required")
	}

	key, err := h.sshKeySvc.Add(c.Request().Context(), services.AddSSHKeyInput{
		UserID: currentUser.ID,
		Title:  req.Title,
		Key:    req.Key,
	})
	if err != nil {
		if errors.Is(err, services.ErrSSHKeyExists) {
			return echo.NewHTTPError(http.StatusConflict, "SSH key already added")
		}
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return c.JSON(http.StatusCreated, key)
}

func (h *SSHKeyHandler) Delete(c echo.Context) error {
	currentUser := c.Get("user").(*models.User)

	keyID := c.Param("id")
	if keyID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid key ID")
	}

	if err := h.sshKeySvc.Delete(c.Request().Context(), keyID, currentUser.ID); err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "key not found")
	}

	return c.NoContent(http.StatusNoContent)
}

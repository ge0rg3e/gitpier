package handlers

import (
	"net/http"
	"strings"

	"gitpier/internal/models"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

type FeedbackHandler struct {
	db *gorm.DB
}

func NewFeedbackHandler(db *gorm.DB) *FeedbackHandler {
	return &FeedbackHandler{db: db}
}

func (h *FeedbackHandler) Create(c echo.Context) error {
	var req struct {
		Category string `json:"category"`
		Message  string `json:"message"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	category := strings.TrimSpace(strings.ToLower(req.Category))
	if category != "bug" && category != "feature" && category != "other" {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid feedback category")
	}
	message := strings.TrimSpace(req.Message)
	if message == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "feedback message is required")
	}
	if len(message) > 4000 {
		return echo.NewHTTPError(http.StatusBadRequest, "feedback message is too long")
	}

	feedback := &models.Feedback{
		Category: category,
		Message:  message,
		Status:   models.FeedbackStatusNew,
	}

	if user, ok := c.Get("user").(*models.User); ok && user != nil {
		feedback.UserID = &user.ID
	}

	if err := h.db.WithContext(c.Request().Context()).Create(feedback).Error; err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to submit feedback")
	}
	return c.JSON(http.StatusCreated, map[string]interface{}{"feedback": feedback})
}

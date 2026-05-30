package handlers

import (
	"net/url"
	"strings"

	"github.com/labstack/echo/v4"
)

var configuredPublicBaseURL string

// ConfigurePublicBaseURL sets the canonical external base URL used in responses.
// Expected format: scheme://host[:port], usually sourced from APP_URL.
func ConfigurePublicBaseURL(raw string) {
	configuredPublicBaseURL = normalizePublicBaseURL(raw)
}

func normalizePublicBaseURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	u, err := url.Parse(trimmed)
	if err != nil || u.Host == "" {
		return ""
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return ""
	}
	return strings.TrimRight(u.Scheme+"://"+u.Host, "/")
}

func publicBaseURL(c echo.Context) string {
	return configuredPublicBaseURL
}

func publicHost(c echo.Context) string {
	if configuredPublicBaseURL != "" {
		if u, err := url.Parse(configuredPublicBaseURL); err == nil && u.Host != "" {
			return u.Host
		}
	}
	return c.Request().Host
}

// toAbsoluteURL returns an absolute URL for path-like values while keeping
// existing absolute URLs unchanged.
func toAbsoluteURL(c echo.Context, raw string) string {
	url := strings.TrimSpace(raw)
	if url == "" {
		return ""
	}
	if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		return url
	}
	if !strings.HasPrefix(url, "/") {
		url = "/" + url
	}
	base := publicBaseURL(c)
	if base == "" {
		return url
	}
	return base + url
}

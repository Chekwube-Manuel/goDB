package main

import (
	_ "embed"
	"net/http"
)

//go:embed dashboard.html
var dashboardHTML []byte

// handleDashboard serves the minimal admin dashboard (cloud or laptop mode).
// The page itself is behind the same basic auth as the rest of the API.
func (s *service) handleDashboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(dashboardHTML)
}

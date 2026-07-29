package main

import (
	"context"
	"net/http"
	"time"
)

func (s *service) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	status := map[string]any{"service": s.cfg.AppName, "status": "ok"}
	if err := s.db.PingContext(ctx); err != nil {
		status["database"] = "down"
		status["status"] = "error"
		writeJSON(w, http.StatusServiceUnavailable, status)
		return
	}
	status["database"] = "up"
	if err := s.rdb.Ping(ctx).Err(); err != nil {
		status["redis"] = "down"
		status["status"] = "error"
		writeJSON(w, http.StatusServiceUnavailable, status)
		return
	}
	status["redis"] = "up"
	writeJSON(w, http.StatusOK, status)
}

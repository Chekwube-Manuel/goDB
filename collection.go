package main

import (
	"encoding/json"
	"net/http"
	"strings"
)

func (s *service) createCollection(w http.ResponseWriter, r *http.Request, tenantSlug string) {
	var payload struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if strings.TrimSpace(payload.Name) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "collection name is required"})
		return
	}
	ctx := r.Context()
	if _, err := s.db.ExecContext(ctx, `INSERT INTO collections (tenant_slug, name) VALUES ($1, $2) ON CONFLICT (tenant_slug, name) DO NOTHING`, tenantSlug, payload.Name); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"status": "created", "tenant": tenantSlug, "collection": payload.Name})
}

func (s *service) listCollections(w http.ResponseWriter, r *http.Request, tenantSlug string) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT name FROM collections WHERE tenant_slug = $1 ORDER BY created_at DESC`, tenantSlug)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		names = append(names, name)
	}
	writeJSON(w, http.StatusOK, map[string]any{"tenant": tenantSlug, "collections": names})
}

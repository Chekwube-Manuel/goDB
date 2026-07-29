package main

import (
	"encoding/json"
	"net/http"
	"strings"
)

func (s *service) createTenant(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Slug string `json:"slug"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if strings.TrimSpace(payload.Slug) == "" || strings.TrimSpace(payload.Name) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "slug and name are required"})
		return
	}
	ctx := r.Context()
	if _, err := s.db.ExecContext(ctx, `INSERT INTO tenants (slug, name) VALUES ($1, $2) ON CONFLICT (slug) DO NOTHING`, payload.Slug, payload.Name); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"status": "created", "tenant": payload.Slug})
}

func (s *service) listTenants(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT slug, name, created_at FROM tenants ORDER BY created_at DESC`)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	var tenants []tenant
	for rows.Next() {
		var item tenant
		if err := rows.Scan(&item.Slug, &item.Name, &item.CreatedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		tenants = append(tenants, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"tenants": tenants})
}

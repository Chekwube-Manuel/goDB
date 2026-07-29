package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

func (s *service) registerNode(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Name     string `json:"name"`
		Endpoint string `json:"endpoint"`
		Token    string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if strings.TrimSpace(payload.Name) == "" || strings.TrimSpace(payload.Endpoint) == "" || strings.TrimSpace(payload.Token) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name, endpoint, and token are required"})
		return
	}
	hash, err := hashPassword(payload.Token)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	ctx := r.Context()
	var id int64
	err = s.db.QueryRowContext(ctx, `INSERT INTO data_nodes (name, endpoint, token_hash, status, last_seen_at) VALUES ($1, $2, $3, 'online', NOW()) ON CONFLICT (name) DO UPDATE SET endpoint = EXCLUDED.endpoint, token_hash = EXCLUDED.token_hash, status = 'online', last_seen_at = NOW() RETURNING id`, payload.Name, payload.Endpoint, hash).Scan(&id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"status": "registered", "node_id": id, "name": payload.Name, "endpoint": payload.Endpoint})
}

func (s *service) listNodes(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT id, name, endpoint, status, created_at, last_seen_at FROM data_nodes ORDER BY created_at DESC`)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	var nodes []dataNode
	for rows.Next() {
		var node dataNode
		if err := rows.Scan(&node.ID, &node.Name, &node.Endpoint, &node.Status, &node.CreatedAt, &node.LastSeenAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		nodes = append(nodes, node)
	}
	writeJSON(w, http.StatusOK, map[string]any{"nodes": nodes})
}

func (s *service) handleNodeHeartbeat(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if strings.TrimSpace(payload.Name) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}
	if payload.Status == "" {
		payload.Status = "online"
	}
	ctx := r.Context()
	if _, err := s.db.ExecContext(ctx, `UPDATE data_nodes SET status = $1, last_seen_at = NOW() WHERE name = $2`, payload.Status, payload.Name); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "updated", "name": payload.Name})
}

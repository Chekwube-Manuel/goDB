package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

func (s *service) provisionDatabase(w http.ResponseWriter, r *http.Request, nodeName string) {
	var payload provisioningRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if payload.DatabaseID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "database_id is required"})
		return
	}
	payload.DatabaseID = normalizeDatabaseID(payload.DatabaseID)
	ctx := r.Context()
	var nodeID int64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM data_nodes WHERE name = $1`, nodeName).Scan(&nodeID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "node not found"})
		return
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO provisioned_databases (node_id, database_id, region, status) VALUES ($1, $2, $3, 'provisioned') ON CONFLICT (database_id) DO NOTHING`, nodeID, payload.DatabaseID, payload.Region); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"status": "provisioned", "node": nodeName, "database_id": payload.DatabaseID, "region": payload.Region})
}

func (s *service) handleNodeProvision(w http.ResponseWriter, r *http.Request) {
	var payload provisioningRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if strings.TrimSpace(payload.DatabaseID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "database_id is required"})
		return
	}
	payload.DatabaseID = normalizeDatabaseID(payload.DatabaseID)
	if s.cfg.NodeEndpoint != "" {
		if err := s.forwardProvisionRequest(r.Context(), payload); err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"status": "forwarded", "database_id": payload.DatabaseID, "node_endpoint": s.cfg.NodeEndpoint})
		return
	}
	ctx := r.Context()
	if err := s.provisionLocalDatabase(ctx, payload); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"status": "provisioned", "database_id": payload.DatabaseID, "region": payload.Region})
}

func (s *service) handleDashboardProvision(w http.ResponseWriter, r *http.Request) {
	var payload provisioningRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	payload.DatabaseID = normalizeDatabaseID(payload.DatabaseID)
	ctx := r.Context()
	if _, err := s.db.ExecContext(ctx, `INSERT INTO provisioned_databases (node_id, database_id, region, status) VALUES (1, $1, $2, 'provisioning') ON CONFLICT (database_id) DO NOTHING`, payload.DatabaseID, payload.Region); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "accepted", "database_id": payload.DatabaseID, "region": payload.Region})
}

func (s *service) listProvisionedDatabases(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT p.database_id, p.region, p.status, p.created_at, n.name FROM provisioned_databases p JOIN data_nodes n ON p.node_id = n.id ORDER BY p.created_at DESC`)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	var items []map[string]any
	for rows.Next() {
		var databaseID, region, status, nodeName string
		var createdAt time.Time
		if err := rows.Scan(&databaseID, &region, &status, &createdAt, &nodeName); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		items = append(items, map[string]any{"database_id": databaseID, "region": region, "status": status, "created_at": createdAt, "node": nodeName})
	}
	writeJSON(w, http.StatusOK, map[string]any{"databases": items})
}

func normalizeDatabaseID(name string) string {
	normalized := strings.ToLower(strings.TrimSpace(name))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	normalized = strings.ReplaceAll(normalized, " ", "_")
	filtered := make([]rune, 0, len(normalized))
	for _, r := range normalized {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			filtered = append(filtered, r)
		}
	}
	if len(filtered) == 0 {
		return "db"
	}
	return string(filtered)
}

func (s *service) provisionLocalDatabase(ctx context.Context, payload provisioningRequest) error {
	if _, err := s.db.ExecContext(ctx, `INSERT INTO provisioned_databases (node_id, database_id, region, status) VALUES (1, $1, $2, 'provisioned') ON CONFLICT (database_id) DO NOTHING`, payload.DatabaseID, payload.Region); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS local_database_catalog (
		id SERIAL PRIMARY KEY,
		database_id TEXT NOT NULL UNIQUE,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO local_database_catalog (database_id) VALUES ($1) ON CONFLICT (database_id) DO NOTHING`, payload.DatabaseID); err != nil {
		return err
	}
	return nil
}

func (s *service) forwardProvisionRequest(ctx context.Context, payload provisioningRequest) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	endpoint := strings.TrimRight(s.cfg.NodeEndpoint, "/") + "/node"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if s.cfg.NodeToken != "" {
		req.Header.Set("Authorization", "Bearer "+s.cfg.NodeToken)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("node provisioning returned status %d", resp.StatusCode)
	}
	return nil
}

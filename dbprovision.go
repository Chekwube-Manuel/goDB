package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

// provisionDatabase handles `POST /nodes/<name>`: provision a database on a
// specific node. In cloud mode it forwards the request to that node's
// registered endpoint; in laptop/standalone mode it provisions locally.
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

	// Single-node deployment: the node name maps to this machine.
	if s.cfg.NodeEndpoint == "" {
		if err := s.provisionLocalDatabase(ctx, payload); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"status": "provisioned", "node": nodeName, "database_id": payload.DatabaseID, "region": payload.Region})
		return
	}

	var nodeID int64
	var nodeEndpoint string
	err := s.db.QueryRowContext(ctx, `SELECT id, endpoint FROM data_nodes WHERE name = $1`, nodeName).Scan(&nodeID, &nodeEndpoint)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "node not found"})
		return
	}
	if err := s.forwardProvisionRequestTo(ctx, nodeEndpoint, payload); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO provisioned_databases (node_id, database_id, region, status) VALUES ($1, $2, $3, 'provisioned')
		ON CONFLICT (database_id) DO UPDATE SET node_id = EXCLUDED.node_id, region = EXCLUDED.region, status = 'provisioned'`,
		nodeID, payload.DatabaseID, payload.Region); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"status": "provisioned", "node": nodeName, "database_id": payload.DatabaseID, "region": payload.Region})
}

// handleNodeProvision is the `/node` endpoint. On the laptop it provisions
// locally; on the cloud it forwards the request to the configured node.
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
	ctx := r.Context()

	if s.cfg.NodeEndpoint != "" {
		if err := s.forwardProvisionRequestTo(ctx, s.cfg.NodeEndpoint, payload); err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"status": "forwarded", "database_id": payload.DatabaseID, "node_endpoint": s.cfg.NodeEndpoint})
		return
	}

	if err := s.provisionLocalDatabase(ctx, payload); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"status": "provisioned", "database_id": payload.DatabaseID, "region": payload.Region})
}

// handleDashboardProvision handles `POST /databases/provision`. On a laptop
// (or standalone) it provisions locally; on the cloud it resolves the target
// node (explicit node_name, else the configured/single node, else the most
// recent online node) and forwards the provisioning request to it.
func (s *service) handleDashboardProvision(w http.ResponseWriter, r *http.Request) {
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
	ctx := r.Context()

	// Laptop / standalone: provision on this machine.
	if s.cfg.NodeEndpoint == "" {
		if err := s.provisionLocalDatabase(ctx, payload); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"status": "provisioned", "database_id": payload.DatabaseID, "region": payload.Region})
		return
	}

	nodeID, endpoint, err := s.resolveProvisionTarget(ctx, payload.NodeName)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := s.forwardProvisionRequestTo(ctx, endpoint, payload); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO provisioned_databases (node_id, database_id, region, status) VALUES ($1, $2, $3, 'provisioned')
		ON CONFLICT (database_id) DO UPDATE SET node_id = EXCLUDED.node_id, region = EXCLUDED.region, status = 'provisioned'`,
		nodeID, payload.DatabaseID, payload.Region); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"status": "provisioned", "database_id": payload.DatabaseID, "region": payload.Region, "node_id": nodeID})
}

// resolveProvisionTarget finds the node (id + endpoint) a dashboard provision
// request should be sent to: explicit node_name, else the configured
// NODE_ENDPOINT's registered node, else the most recent online node.
func (s *service) resolveProvisionTarget(ctx context.Context, nodeName string) (int64, string, error) {
	var nodeID int64
	var endpoint string
	if nodeName != "" {
		err := s.db.QueryRowContext(ctx, `SELECT id, endpoint FROM data_nodes WHERE name = $1`, nodeName).Scan(&nodeID, &endpoint)
		if err != nil {
			return 0, "", fmt.Errorf("node %q not found", nodeName)
		}
		return nodeID, endpoint, nil
	}
	if s.cfg.NodeEndpoint != "" {
		err := s.db.QueryRowContext(ctx, `SELECT id, endpoint FROM data_nodes WHERE endpoint = $1 ORDER BY last_seen_at DESC LIMIT 1`, s.cfg.NodeEndpoint).Scan(&nodeID, &endpoint)
		if err != nil {
			return 0, "", fmt.Errorf("no registered node matches the configured NODE_ENDPOINT")
		}
		return nodeID, endpoint, nil
	}
	err := s.db.QueryRowContext(ctx, `SELECT id, endpoint FROM data_nodes WHERE status = 'online' ORDER BY last_seen_at DESC LIMIT 1`).Scan(&nodeID, &endpoint)
	if err != nil {
		return 0, "", fmt.Errorf("no online node available")
	}
	return nodeID, endpoint, nil
}

func (s *service) listProvisionedDatabases(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT p.database_id, p.region, p.status, p.created_at, COALESCE(n.name, 'unknown') FROM provisioned_databases p LEFT JOIN data_nodes n ON p.node_id = n.id ORDER BY p.created_at DESC`)
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

// provisionLocalDatabase provisions a real PostgreSQL database on this
// machine. It registers (or refreshes) this machine as a node so the
// provisioned_databases foreign key is satisfied, then creates the database.
func (s *service) provisionLocalDatabase(ctx context.Context, payload provisioningRequest) error {
	var nodeID int64
	err := s.db.QueryRowContext(ctx, `INSERT INTO data_nodes (name, endpoint, token_hash, status, last_seen_at)
		VALUES ($1, 'local', '', 'online', NOW())
		ON CONFLICT (name) DO UPDATE SET status = 'online', last_seen_at = NOW()
		RETURNING id`, s.cfg.NodeName).Scan(&nodeID)
	if err != nil {
		return err
	}

	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS local_database_catalog (
		id SERIAL PRIMARY KEY,
		database_id TEXT NOT NULL UNIQUE,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`); err != nil {
		return err
	}

	if err := s.createDatabase(ctx, payload.DatabaseID); err != nil {
		return err
	}

	if _, err := s.db.ExecContext(ctx, `INSERT INTO provisioned_databases (node_id, database_id, region, status) VALUES ($1, $2, $3, 'provisioned')
		ON CONFLICT (database_id) DO UPDATE SET node_id = EXCLUDED.node_id, region = EXCLUDED.region, status = 'provisioned'`,
		nodeID, payload.DatabaseID, payload.Region); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO local_database_catalog (database_id) VALUES ($1) ON CONFLICT (database_id) DO NOTHING`, payload.DatabaseID); err != nil {
		return err
	}
	return nil
}

// createDatabase creates a PostgreSQL database by name. databaseID is
// normalized to [a-z0-9_] so it is safe to quote as an identifier.
// An already-existing database is not an error.
func (s *service) createDatabase(ctx context.Context, databaseID string) error {
	_, err := s.db.ExecContext(ctx, `CREATE DATABASE "`+databaseID+`"`)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "42P04" { // duplicate_database
			return nil
		}
		return err
	}
	return nil
}

// forwardProvisionRequestTo sends a provisioning request to a node endpoint.
func (s *service) forwardProvisionRequestTo(ctx context.Context, endpoint string, payload provisioningRequest) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	nodeURL := strings.TrimRight(endpoint, "/") + "/node"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, nodeURL, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if s.cfg.NodeToken != "" {
		req.Header.Set("Authorization", "Bearer "+s.cfg.NodeToken)
	}
	resp, err := nodeHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("node provisioning returned status %d", resp.StatusCode)
	}
	return nil
}

package main

import (
	"net/http"
	"strings"
)

const (
	dataPrefix = "/tenants/"
)

func (s *service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.setCORSHeaders(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// ── Node routes (laptop agent → cloud, or direct node-to-node) ──
	if strings.HasPrefix(r.URL.Path, "/node") {
		if s.isNodeAuthenticated(r) {
			if r.Method == http.MethodPost {
				s.handleNodeProvision(w, r)
				return
			}
			if r.Method == http.MethodGet {
				writeJSON(w, http.StatusOK, map[string]string{"status": "node-ready"})
				return
			}
		}
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "node auth required"})
		return
	}

	// ── CLOUD MODE: forward data operations to laptop ──
	// When NodeEndpoint is set, the cloud does NOT handle data locally.
	// It only manages tenants, nodes, and routing — the actual data lives
	// on the laptop. Laptop node endpoints receive the forwarded requests.
	if s.cfg.NodeEndpoint != "" {
		// Cloud handles these directly (metadata operations)
		if r.URL.Path == "/health" || r.URL.Path == "/health/" {
			s.handleHealth(w, r)
			return
		}

		// Cloud-side tenant/node management stays local
		isCloudOp := false
		switch {
		case r.URL.Path == "/tenants" && r.Method == http.MethodPost:
			isCloudOp = true
		case r.URL.Path == "/tenants" && r.Method == http.MethodGet:
			isCloudOp = true
		case r.URL.Path == "/nodes" && (r.Method == http.MethodPost || r.Method == http.MethodGet):
			isCloudOp = true
		case strings.HasPrefix(r.URL.Path, "/node/"):
			isCloudOp = true
		case strings.HasPrefix(r.URL.Path, "/databases/"):
			isCloudOp = true
		}

		if isCloudOp {
			s.routeCloudRequest(w, r)
			return
		}

		// All data operations (tenant/collection/record CRUD) get forwarded
		s.forwardRequestAndReply(w, r)
		return
	}

	// ── LAPTOP MODE (or standalone): handle everything locally ──
	s.handleLocalRequest(w, r)
}

// routeCloudRequest handles cloud-side operations that stay on the cloud server.
func (s *service) routeCloudRequest(w http.ResponseWriter, r *http.Request) {
	if !s.limiter.allow(r.RemoteAddr) {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "rate limit exceeded"})
		return
	}
	if !s.isAuthenticated(r) {
		w.Header().Set("WWW-Authenticate", `Basic realm="dbhost"`)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}

	path := strings.Trim(r.URL.Path, "/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		parts = []string{"health"}
	}

	switch {
	case parts[0] == "health":
		s.handleHealth(w, r)
	case parts[0] == "tenants" && r.Method == http.MethodPost:
		s.createTenant(w, r)
	case parts[0] == "tenants" && r.Method == http.MethodGet:
		s.listTenants(w, r)
	case parts[0] == "nodes" && r.Method == http.MethodPost:
		s.registerNode(w, r)
	case parts[0] == "nodes" && r.Method == http.MethodGet:
		s.listNodes(w, r)
	case len(parts) == 2 && parts[0] == "nodes" && r.Method == http.MethodPost:
		s.provisionDatabase(w, r, parts[1])
	case len(parts) == 2 && parts[0] == "databases" && parts[1] == "provision" && r.Method == http.MethodPost:
		s.handleDashboardProvision(w, r)
	case len(parts) == 2 && parts[0] == "databases" && parts[1] == "status" && r.Method == http.MethodGet:
		s.listProvisionedDatabases(w, r)
	default:
		http.NotFound(w, r)
	}
}

// handleLocalRequest handles all requests directly on the laptop (or standalone).
func (s *service) handleLocalRequest(w http.ResponseWriter, r *http.Request) {
	if !s.limiter.allow(r.RemoteAddr) {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "rate limit exceeded"})
		return
	}
	if !s.isAuthenticated(r) {
		w.Header().Set("WWW-Authenticate", `Basic realm="dbhost"`)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}

	path := strings.Trim(r.URL.Path, "/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		parts = []string{"health"}
	}

	switch {
	case len(parts) == 1 && parts[0] == "health":
		s.handleHealth(w, r)
	case len(parts) == 1 && parts[0] == "nodes" && r.Method == http.MethodPost:
		s.registerNode(w, r)
	case len(parts) == 1 && parts[0] == "nodes" && r.Method == http.MethodGet:
		s.listNodes(w, r)
	case len(parts) == 2 && parts[0] == "nodes" && r.Method == http.MethodPost:
		s.provisionDatabase(w, r, parts[1])
	case len(parts) == 2 && parts[0] == "databases" && parts[1] == "provision" && r.Method == http.MethodPost:
		s.handleDashboardProvision(w, r)
	case len(parts) == 2 && parts[0] == "databases" && parts[1] == "status" && r.Method == http.MethodGet:
		s.listProvisionedDatabases(w, r)
	case len(parts) == 1 && parts[0] == "node" && r.Method == http.MethodPost:
		s.handleNodeProvision(w, r)
	case len(parts) == 2 && parts[0] == "node" && parts[1] == "heartbeat" && r.Method == http.MethodPost:
		s.handleNodeHeartbeat(w, r)
	case len(parts) == 1 && parts[0] == "tenants" && r.Method == http.MethodPost:
		s.createTenant(w, r)
	case len(parts) == 1 && parts[0] == "tenants" && r.Method == http.MethodGet:
		s.listTenants(w, r)
	case len(parts) == 2 && parts[0] == "tenants" && r.Method == http.MethodPost:
		s.createCollection(w, r, parts[1])
	case len(parts) == 2 && parts[0] == "tenants" && r.Method == http.MethodGet:
		s.listCollections(w, r, parts[1])
	case len(parts) == 5 && parts[0] == "tenants" && parts[2] == "collections" && parts[4] == "records" && r.Method == http.MethodPost:
		s.storeRecord(w, r, parts[1], parts[3])
	case len(parts) == 5 && parts[0] == "tenants" && parts[2] == "collections" && parts[4] == "records" && r.Method == http.MethodGet:
		s.listRecords(w, r, parts[1], parts[3])
	default:
		http.NotFound(w, r)
	}
}

package main

import (
	"net/http"
	"strings"
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
				if r.URL.Path == "/node/heartbeat" {
					s.handleNodeHeartbeat(w, r)
					return
				}
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

	// ── PROXY MODE (on pxxl.app): no database at all — just forward everything ──
	if s.cfg.ProxyMode {
		if r.URL.Path == "/health" || r.URL.Path == "/health/" {
			writeJSON(w, http.StatusOK, map[string]any{
				"service": s.cfg.AppName,
				"status":  "ok",
				"mode":    "proxy",
			})
			return
		}
		if !s.isAuthenticated(r) {
			w.Header().Set("WWW-Authenticate", `Basic realm="dbhost"`)
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
			return
		}
		s.forwardRequestAndReply(w, r)
		return
	}

	// ── CLOUD MODE: forward data operations to laptop ──
	if s.cfg.NodeEndpoint != "" {
		if r.URL.Path == "/health" || r.URL.Path == "/health/" {
			s.handleHealth(w, r)
			return
		}

		isCloudOp := false
		switch {
		case r.URL.Path == "/" || r.URL.Path == "/dashboard":
			isCloudOp = true
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

		// User data paths are forwarded to the laptop, but the cloud still
		// authenticates the caller first.
		if !s.isAuthenticated(r) {
			w.Header().Set("WWW-Authenticate", `Basic realm="dbhost"`)
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
			return
		}
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

	path := strings.Trim(r.URL.Path, "/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		parts = []string{"dashboard"}
	}

	// Node management routes (registration, list) can be called by the laptop
	// agent with its bearer token; everything else requires admin basic auth.
	authorized := s.isAuthenticated(r)
	if !authorized && parts[0] == "nodes" {
		authorized = s.isNodeAuthenticated(r)
	}
	if !authorized {
		w.Header().Set("WWW-Authenticate", `Basic realm="dbhost"`)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}

	switch {
	case parts[0] == "health":
		s.handleHealth(w, r)
	case parts[0] == "dashboard":
		s.handleDashboard(w, r)
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
	// Accept admin basic auth (direct access) or a trusted node's bearer
	// token (requests forwarded from the cloud).
	if !s.isRequestAuthorized(r) {
		w.Header().Set("WWW-Authenticate", `Basic realm="dbhost"`)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}

	path := strings.Trim(r.URL.Path, "/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		parts = []string{"dashboard"}
	}

	switch {
	case len(parts) == 1 && parts[0] == "health":
		s.handleHealth(w, r)
	case len(parts) == 1 && parts[0] == "dashboard":
		s.handleDashboard(w, r)
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

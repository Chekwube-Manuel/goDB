package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Integration tests exercise the real HTTP handlers against a live
// PostgreSQL + Redis. They are skipped automatically when the services are
// unreachable, so `go test ./...` still passes on machines without them.
//
// Set TEST_DATABASE_URL / TEST_REDIS_ADDR to point at non-default instances.

var testSvc *service

func TestMain(m *testing.M) {
	cfg := config{
		DatabaseURL:     getenv("TEST_DATABASE_URL", "postgres://postgres:postgres@localhost:5432/appdb?sslmode=disable"),
		RedisAddr:       getenv("TEST_REDIS_ADDR", "localhost:6379"),
		AppName:         "dbhost-itest",
		AuthUsername:    "admin",
		AuthPassword:    "pass",
		NodeAuthToken:   "node-secret",
		NodeToken:       "node-secret",
		NodeName:        "itest-node",
		RateLimitBurst:  1000,
		RateLimitWindow: 60_000,
	}
	if svc, err := newService(cfg); err == nil {
		testSvc = svc
	}
	code := m.Run()
	if testSvc != nil {
		_ = testSvc.close()
	}
	os.Exit(code)
}

func requireSvc(t *testing.T) {
	t.Helper()
	if testSvc == nil {
		t.Skip("postgres/redis unavailable; set TEST_DATABASE_URL and TEST_REDIS_ADDR to run integration tests")
	}
}

var basicAdmin = "Basic " + base64.StdEncoding.EncodeToString([]byte("admin:pass"))
var basicWrong = "Basic " + base64.StdEncoding.EncodeToString([]byte("admin:wrong"))
var nodeBearer = "Bearer node-secret"
var wrongBearer = "Bearer wrong"

func apiRequest(t *testing.T, method, path, body string, auth string) *httptest.ResponseRecorder {
	t.Helper()
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, rdr)
	req.RemoteAddr = "127.0.0.1:12345"
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	rr := httptest.NewRecorder()
	testSvc.ServeHTTP(rr, req)
	return rr
}

func TestIntegrationTenantCollectionRecordFlow(t *testing.T) {
	requireSvc(t)
	slug := "itest_" + strconv.FormatInt(time.Now().UnixNano(), 10)

	// Create tenant.
	rr := apiRequest(t, http.MethodPost, "/tenants", fmt.Sprintf(`{"slug":%q,"name":"Itest"}`, slug), basicAdmin)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create tenant: got %d: %s", rr.Code, rr.Body.String())
	}

	// Create collection.
	rr = apiRequest(t, http.MethodPost, "/tenants/"+slug+"/collections", `{"name":"users"}`, basicAdmin)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create collection: got %d: %s", rr.Code, rr.Body.String())
	}

	// Store a record.
	rr = apiRequest(t, http.MethodPost, "/tenants/"+slug+"/collections/users/records", `{"data":{"name":"Alice","email":"alice@example.com"}}`, basicAdmin)
	if rr.Code != http.StatusCreated {
		t.Fatalf("store record: got %d: %s", rr.Code, rr.Body.String())
	}

	// List records.
	rr = apiRequest(t, http.MethodGet, "/tenants/"+slug+"/collections/users/records", "", basicAdmin)
	if rr.Code != http.StatusOK {
		t.Fatalf("list records: got %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Records []struct {
			Payload map[string]any `json:"payload"`
		} `json:"records"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(resp.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(resp.Records))
	}
	if resp.Records[0].Payload["name"] != "Alice" {
		t.Fatalf("unexpected payload: %v", resp.Records[0].Payload)
	}

	// Record into a missing tenant must 404 (validation added in the fix).
	rr = apiRequest(t, http.MethodPost, "/tenants/no_such_tenant/collections/users/records", `{"data":{"x":1}}`, basicAdmin)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("record to missing tenant: expected 404, got %d", rr.Code)
	}

	_, _ = testSvc.db.Exec(`DELETE FROM tenants WHERE slug = $1`, slug)
}

func TestIntegrationProvisionDatabase(t *testing.T) {
	requireSvc(t)
	dbID := "itestdb_" + strconv.FormatInt(time.Now().UnixNano(), 10)

	rr := apiRequest(t, http.MethodPost, "/databases/provision", fmt.Sprintf(`{"database_id":%q,"region":"laptop"}`, dbID), basicAdmin)
	if rr.Code != http.StatusCreated {
		t.Fatalf("provision: got %d: %s", rr.Code, rr.Body.String())
	}

	// Catalog row must exist.
	var count int
	if err := testSvc.db.QueryRow(`SELECT COUNT(*) FROM local_database_catalog WHERE database_id = $1`, dbID).Scan(&count); err != nil {
		t.Fatalf("query catalog: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected catalog row, got %d", count)
	}

	// The real PostgreSQL database must exist (the headline fix).
	var exists bool
	if err := testSvc.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)`, dbID).Scan(&exists); err != nil {
		t.Fatalf("query pg_database: %v", err)
	}
	if !exists {
		t.Fatalf("database %q was not actually created", dbID)
	}

	// Status endpoint reports it.
	rr = apiRequest(t, http.MethodGet, "/databases/status", "", basicAdmin)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), dbID) {
		t.Fatalf("status: got %d (want 200 containing %q): %s", rr.Code, dbID, rr.Body.String())
	}

	// Cleanup.
	_, _ = testSvc.db.Exec(`DELETE FROM provisioned_databases WHERE database_id = $1`, dbID)
	_, _ = testSvc.db.Exec(`DELETE FROM local_database_catalog WHERE database_id = $1`, dbID)
	_, _ = testSvc.db.Exec(`DROP DATABASE "`+dbID+`"`)
}

func TestIntegrationNodeRegisterAndHeartbeat(t *testing.T) {
	requireSvc(t)
	nodeName := "itestnode_" + strconv.FormatInt(time.Now().UnixNano(), 10)

	// Registration via node bearer token (no basic auth) — validates the fix.
	rr := apiRequest(t, http.MethodPost, "/nodes", fmt.Sprintf(`{"name":%q,"endpoint":"https://node.example","token":"node-secret"}`, nodeName), nodeBearer)
	if rr.Code != http.StatusCreated {
		t.Fatalf("register node: got %d: %s", rr.Code, rr.Body.String())
	}

	// Heartbeat with the node token must be accepted and update the node.
	rr = apiRequest(t, http.MethodPost, "/node/heartbeat", fmt.Sprintf(`{"name":%q,"status":"online"}`, nodeName), nodeBearer)
	if rr.Code != http.StatusOK {
		t.Fatalf("heartbeat: got %d: %s", rr.Code, rr.Body.String())
	}
	var seen bool
	if err := testSvc.db.QueryRow(`SELECT status = 'online' FROM data_nodes WHERE name = $1`, nodeName).Scan(&seen); err != nil {
		t.Fatalf("query node: %v", err)
	}
	if !seen {
		t.Fatal("node status was not updated by heartbeat")
	}

	// Wrong token must be rejected.
	rr = apiRequest(t, http.MethodPost, "/node/heartbeat", fmt.Sprintf(`{"name":%q}`, nodeName), wrongBearer)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("heartbeat with wrong token: expected 401, got %d", rr.Code)
	}

	// List (admin basic auth) shows the node.
	rr = apiRequest(t, http.MethodGet, "/nodes", "", basicAdmin)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), nodeName) {
		t.Fatalf("list nodes: got %d (want 200 containing %q): %s", rr.Code, nodeName, rr.Body.String())
	}

	_, _ = testSvc.db.Exec(`DELETE FROM data_nodes WHERE name = $1`, nodeName)
}

func TestIntegrationAuthGates(t *testing.T) {
	requireSvc(t)

	// Anonymous data access is rejected.
	rr := apiRequest(t, http.MethodGet, "/tenants", "", "")
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous: expected 401, got %d", rr.Code)
	}

	// Wrong password is rejected.
	rr = apiRequest(t, http.MethodGet, "/tenants", "", basicWrong)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password: expected 401, got %d", rr.Code)
	}

	// A trusted node's bearer token is accepted on the data plane (this is
	// how the cloud's forwarded requests authenticate against the laptop).
	rr = apiRequest(t, http.MethodGet, "/tenants", "", nodeBearer)
	if rr.Code != http.StatusOK {
		t.Fatalf("node bearer on laptop routes: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

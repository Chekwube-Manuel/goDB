package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestLoadConfigUsesEnvironmentOverrides(t *testing.T) {
	t.Setenv("PORT", "9090")
	t.Setenv("DATABASE_URL", "postgres://demo:demo@localhost:5432/demo")
	t.Setenv("REDIS_ADDR", "redis.local:6380")
	t.Setenv("REDIS_PASSWORD", "secret")
	t.Setenv("REDIS_DB", "2")
	t.Setenv("APP_NAME", "demo-service")

	cfg := loadConfig()
	if cfg.Port != 9090 {
		t.Fatalf("expected port 9090, got %d", cfg.Port)
	}
	if cfg.DatabaseURL != "postgres://demo:demo@localhost:5432/demo" {
		t.Fatalf("unexpected database url: %s", cfg.DatabaseURL)
	}
	if cfg.RedisAddr != "redis.local:6380" {
		t.Fatalf("unexpected redis addr: %s", cfg.RedisAddr)
	}
	if cfg.RedisPassword != "secret" {
		t.Fatalf("unexpected redis password: %s", cfg.RedisPassword)
	}
	if cfg.RedisDB != 2 {
		t.Fatalf("expected redis db 2, got %d", cfg.RedisDB)
	}
	if cfg.AppName != "demo-service" {
		t.Fatalf("unexpected app name: %s", cfg.AppName)
	}
}

func TestGetenvFallsBackToDefault(t *testing.T) {
	_ = os.Unsetenv("PORT")
	cfg := loadConfig()
	if cfg.Port != 8080 {
		t.Fatalf("expected fallback port 8080, got %d", cfg.Port)
	}
}

func TestPasswordHashingAndComparison(t *testing.T) {
	hash, err := hashPassword("strong-pass")
	if err != nil {
		t.Fatalf("hashPassword() error = %v", err)
	}
	if err := comparePassword(hash, "strong-pass"); err != nil {
		t.Fatalf("comparePassword() should accept the original password: %v", err)
	}
	if err := comparePassword(hash, "wrong-pass"); err == nil {
		t.Fatal("comparePassword() should reject the wrong password")
	}
}

func TestRateLimiterBlocksBurstTraffic(t *testing.T) {
	limiter := newRateLimiter(2, 1*60*1000)
	if !limiter.allow("user-1") {
		t.Fatal("first request should be allowed")
	}
	if !limiter.allow("user-1") {
		t.Fatal("second request should be allowed")
	}
	if limiter.allow("user-1") {
		t.Fatal("third request should be blocked")
	}
}

func TestConfigMirrorsNodeTokens(t *testing.T) {
	// Setting only NODE_TOKEN should make both directions use that secret.
	t.Setenv("NODE_TOKEN", "shared-secret")
	t.Setenv("NODE_AUTH_TOKEN", "")
	cfg := loadConfig()
	if cfg.NodeToken != "shared-secret" {
		t.Fatalf("expected NodeToken to be set, got %q", cfg.NodeToken)
	}
	if cfg.NodeAuthToken != "shared-secret" {
		t.Fatalf("expected NodeAuthToken to mirror NodeToken, got %q", cfg.NodeAuthToken)
	}

	// Setting only NODE_AUTH_TOKEN should make both directions use that secret.
	t.Setenv("NODE_TOKEN", "")
	t.Setenv("NODE_AUTH_TOKEN", "laptop-secret")
	cfg = loadConfig()
	if cfg.NodeAuthToken != "laptop-secret" {
		t.Fatalf("expected NodeAuthToken to be set, got %q", cfg.NodeAuthToken)
	}
	if cfg.NodeToken != "laptop-secret" {
		t.Fatalf("expected NodeToken to mirror NodeAuthToken, got %q", cfg.NodeToken)
	}

	// Neither set: fall back to the default secret in both directions.
	t.Setenv("NODE_TOKEN", "")
	t.Setenv("NODE_AUTH_TOKEN", "")
	cfg = loadConfig()
	if cfg.NodeToken != "node-secret" || cfg.NodeAuthToken != "node-secret" {
		t.Fatalf("expected default node-secret for both, got token=%q auth=%q", cfg.NodeToken, cfg.NodeAuthToken)
	}
}

func TestAuthAcceptsBasicAndNodeToken(t *testing.T) {
	svc := &service{cfg: config{AuthUsername: "admin", AuthPassword: "pass", NodeAuthToken: "node-secret"}}

	// Admin basic auth is accepted.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.SetBasicAuth("admin", "pass")
	if !svc.isAuthenticated(req) {
		t.Fatal("valid basic auth should authenticate")
	}
	if !svc.isRequestAuthorized(req) {
		t.Fatal("valid basic auth should be request-authorized")
	}

	// Wrong password is rejected.
	reqBad := httptest.NewRequest(http.MethodGet, "/", nil)
	reqBad.SetBasicAuth("admin", "wrong")
	if svc.isAuthenticated(reqBad) {
		t.Fatal("wrong password should not authenticate")
	}
	if svc.isRequestAuthorized(reqBad) {
		t.Fatal("wrong password should not be request-authorized")
	}

	// A trusted node's bearer token is not basic auth, but must be
	// request-authorized — that is how cloud-forwarded requests reach the laptop.
	reqBearer := httptest.NewRequest(http.MethodGet, "/", nil)
	reqBearer.Header.Set("Authorization", "Bearer node-secret")
	if svc.isAuthenticated(reqBearer) {
		t.Fatal("bearer token should not pass basic auth")
	}
	if !svc.isNodeAuthenticated(reqBearer) {
		t.Fatal("valid node token should authenticate as a node")
	}
	if !svc.isRequestAuthorized(reqBearer) {
		t.Fatal("valid node token should be request-authorized (cloud forwarding)")
	}

	// Wrong node token is rejected everywhere.
	reqBadBearer := httptest.NewRequest(http.MethodGet, "/", nil)
	reqBadBearer.Header.Set("Authorization", "Bearer wrong")
	if svc.isNodeAuthenticated(reqBadBearer) {
		t.Fatal("wrong node token should not authenticate as a node")
	}
	if svc.isRequestAuthorized(reqBadBearer) {
		t.Fatal("wrong node token should not be request-authorized")
	}
}

func TestLaptopModeRejectsAnonymousRequest(t *testing.T) {
	svc := &service{
		cfg:     config{AuthUsername: "admin", AuthPassword: "pass", NodeAuthToken: "node-secret"},
		limiter: newRateLimiter(30, 60_000),
	}
	req := httptest.NewRequest(http.MethodGet, "/tenants", nil)
	rr := httptest.NewRecorder()
	svc.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous request to laptop should be 401, got %d", rr.Code)
	}
}

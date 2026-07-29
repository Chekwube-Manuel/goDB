package main

import (
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

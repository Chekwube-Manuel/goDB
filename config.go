package main

import (
	"os"
	"strconv"
)

func loadConfig() config {
	proxyMode := getenv("PROXY_MODE", "")
	if proxyMode == "" {
		nodeEp := os.Getenv("NODE_ENDPOINT")
		dbURL := os.Getenv("DATABASE_URL")
		proxyMode = "false"
		if nodeEp != "" && dbURL == "" {
			proxyMode = "true"
		}
	}

	// One shared secret authenticates node-to-node traffic in both directions:
	// NODE_TOKEN is what this process sends to its peer, NODE_AUTH_TOKEN is
	// what it accepts from its peer. Mirroring the two means setting a single
	// env var on each side is enough.
	nodeToken := getenv("NODE_TOKEN", "")
	nodeAuthToken := getenv("NODE_AUTH_TOKEN", "")
	if nodeToken == "" {
		nodeToken = nodeAuthToken
	}
	if nodeAuthToken == "" {
		nodeAuthToken = nodeToken
	}
	if nodeToken == "" {
		nodeToken = "node-secret"
		nodeAuthToken = "node-secret"
	}

	return config{
		Port:          getenvInt("PORT", 8080),
		DatabaseURL:   getenv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/appdb?sslmode=disable"),
		RedisAddr:     getenv("REDIS_ADDR", "localhost:6379"),
		RedisPassword: getenv("REDIS_PASSWORD", ""),
		RedisDB:       getenvInt("REDIS_DB", 0),
		AppName:         getenv("APP_NAME", "self-hosted-db"),
		AuthUsername:    getenv("AUTH_USERNAME", "admin"),
		AuthPassword:    getenv("AUTH_PASSWORD", "changeme"),
		AllowedOrigin:   getenv("ALLOWED_ORIGIN", "*"),
		NodeEndpoint:    getenv("NODE_ENDPOINT", ""),
		NodeToken:       nodeToken,
		NodeAuthToken:   nodeAuthToken,
		NodeName:        getenv("NODE_NAME", "laptop-1"),
		RateLimitBurst:  getenvInt("RATE_LIMIT_BURST", 30),
		RateLimitWindow: getenvInt("RATE_LIMIT_WINDOW_MS", 60_000),
		ProxyMode:       proxyMode == "true" || proxyMode == "1",
	}
}

func getenv(key string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

package main

import (
	"encoding/json"
	"time"
)

type config struct {
	Port            int
	DatabaseURL     string
	RedisAddr       string
	RedisPassword   string
	RedisDB         int
	AppName         string
	AuthUsername    string
	AuthPassword    string
	AllowedOrigin   string
	NodeEndpoint    string
	NodeToken       string
	NodeAuthToken   string
	RateLimitBurst  int
	RateLimitWindow int
}

type tenant struct {
	Slug      string    `json:"slug"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type dataNode struct {
	ID         int64     `json:"id"`
	Name       string    `json:"name"`
	Endpoint   string    `json:"endpoint"`
	TokenHash  string    `json:"-"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
	LastSeenAt time.Time `json:"last_seen_at"`
}

type provisioningRequest struct {
	NodeName   string `json:"node_name"`
	DatabaseID string `json:"database_id"`
	Region     string `json:"region"`
}

type recordPayload struct {
	Data map[string]any `json:"data"`
}

type recordResult struct {
	ID         int64           `json:"id"`
	TenantSlug string          `json:"tenant_slug"`
	Collection string          `json:"collection"`
	Payload    json.RawMessage `json:"payload"`
	CreatedAt  time.Time       `json:"created_at"`
}

# goDB — Self-Hosted Database Backend

A database-hosting platform where the **control plane lives in the cloud** but the **data stays on your laptop**. Users interact with a managed API; their actual databases run on PostgreSQL physically connected to your machine.

## Architecture

```
┌─────────────────────────────────┐       ┌──────────────────────────────────┐
│         CLOUD SERVER (VPS)       │       │       YOUR LAPTOP (data plane)   │
│                                  │       │                                  │
│  goDB in cloud mode              │       │  goDB in laptop mode             │
│  - Public API endpoints          │       │  - PostgreSQL (user databases)   │
│  - Tenant management            │◄─────►│  - Redis cache                   │
│  - Node registration            │       │  - Heartbeat agent               │
│  - Dashboard frontend           │       │  - Record CRUD (actual data)     │
│  - Auth & rate limiting         │       │                                  │
│  - Forwards DATA to laptop      │       │  Tunnel: ngrok / Tailscale       │
│                                  │       │                                  │
│  NODE_ENDPOINT=https://laptop    │       │  CLOUD_ENDPOINT=https://cloud    │
│     .ngrok.io                   │       │     .yoursite.com               │
└─────────────────────────────────┘       └──────────────────────────────────┘
```

### How it works

1. **Laptop** runs PostgreSQL, Redis, and `go-db` in laptop mode
2. **Laptop** opens a secure tunnel (ngrok/Tailscale) to make itself reachable
3. **Laptop agent** registers with the cloud and sends heartbeats every 30s
4. **Cloud** receives user requests, authenticates them, routes metadata locally
5. **Cloud** forwards all data operations (record CRUD) to the laptop via the tunnel
6. **User databases** physically live only on your laptop — never in the cloud

### What lives where

| Component | Location |
|-----------|----------|
| Public API endpoints | Cloud |
| Authentication | Cloud |
| Rate limiting | Cloud |
| Tenant management | Cloud |
| Node registry | Cloud |
| Database provisioning orchestration | Cloud |
| **PostgreSQL (user databases)** | **Laptop** |
| **Redis** | **Laptop** |
| **Record storage & retrieval** | **Laptop** |
| **Laptop heartbeat agent** | **Laptop** |

## Quick Start

### Prerequisites

- Go 1.25+
- PostgreSQL 16+
- Redis 7+
- [ngrok](https://ngrok.com/download) (for tunnel) OR Tailscale Funnel

### 1. Run on your Laptop (data plane)

```bash
# Start PostgreSQL and Redis (or use Docker Compose)
docker compose up -d postgres redis

# Run goDB in laptop mode (auto-detected when NODE_ENDPOINT is empty)
export NODE_AUTH_TOKEN=my-secret-node-token
export CLOUD_ENDPOINT=https://your-cloud-server.com  # where the cloud runs
export NODE_NAME=laptop-1
go run .
```

### 2. Expose your laptop (tunnel)

```powershell
# On Windows (PowerShell):
.\setup-tunnel.ps1 -CloudEndpoint https://your-cloud-server.com

# This will:
# 1. Start ngrok tunnels for PostgreSQL and the API
# 2. Show you the public URLs
# 3. Launch goDB in laptop mode with agent
```

### 3. Run on your Cloud Server

```bash
# Run goDB in cloud mode (auto-detected when NODE_ENDPOINT is set)
export DATABASE_URL=postgres://user:pass@localhost:5432/appdb
export REDIS_ADDR=localhost:6379
export AUTH_USERNAME=admin
export AUTH_PASSWORD=changeme

# Point to your laptop tunnel:
export NODE_ENDPOINT=https://your-laptop.ngrok.io
export NODE_TOKEN=my-secret-node-token   # same as NODE_AUTH_TOKEN on laptop

go run .
```

### 4. Use the API

```bash
# Health check
curl -u admin:changeme http://localhost:8080/health

# Create a tenant
curl -u admin:changeme -X POST http://localhost:8080/tenants \
  -H "Content-Type: application/json" \
  -d '{"slug":"myapp","name":"My App"}'

# Create a collection
curl -u admin:changeme -X POST http://localhost:8080/tenants/myapp/collections \
  -H "Content-Type: application/json" \
  -d '{"name":"users"}'

# Store a record (→ forwarded to laptop)
curl -u admin:changeme -X POST http://localhost:8080/tenants/myapp/collections/users/records \
  -H "Content-Type: application/json" \
  -d '{"data":{"name":"Alice","email":"alice@example.com"}}'

# List records
curl -u admin:changeme http://localhost:8080/tenants/myapp/collections/users/records
```

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | HTTP port |
| `DATABASE_URL` | `postgres://postgres:postgres@localhost:5432/appdb?sslmode=disable` | PostgreSQL connection |
| `REDIS_ADDR` | `localhost:6379` | Redis address |
| `REDIS_PASSWORD` | `` | Redis password |
| `AUTH_USERNAME` | `admin` | Basic auth username |
| `AUTH_PASSWORD` | `changeme` | Basic auth password |
| `ALLOWED_ORIGIN` | `*` | CORS origin |
| `NODE_ENDPOINT` | `` | **Cloud mode**: set to laptop's public URL. Empty = laptop/standalone mode |
| `NODE_TOKEN` | mirrors `NODE_AUTH_TOKEN` | Shared secret this process **sends** to its peer (cloud → laptop). Setting just this also sets `NODE_AUTH_TOKEN` |
| `NODE_AUTH_TOKEN` | `node-secret` | Shared secret this process **accepts** from its peer (laptop ← cloud). Setting just this also sets `NODE_TOKEN` |
| `CLOUD_ENDPOINT` | `` | **Laptop mode**: the cloud server URL (for agent registration) |
| `NODE_NAME` | `laptop-1` | Name to register with the cloud |
| `HEARTBEAT_SEC` | `30` | Heartbeat interval in seconds |
| `RATE_LIMIT_BURST` | `30` | Rate limit burst count |
| `RATE_LIMIT_WINDOW_MS` | `60000` | Rate limit window in ms |

### Docker Compose

```bash
# Start everything locally (standalone mode)
docker compose up --build

# On laptop: start only postgres and redis, run the app separately
docker compose up -d postgres redis
go run .
```

## Security Considerations

- The **laptop must be reachable** from the cloud — use ngrok, Tailscale Funnel, or a VPN
- All requests between cloud and laptop use `Bearer` token authentication
- The laptop agent only accepts connections from authenticated cloud servers
- User authentication happens on the cloud — the laptop trusts the cloud's forwarded requests
- **Add HTTPS** in production (ngrok provides this by default)

## Tunnel Options

| Tool | Setup | Notes |
|------|-------|-------|
| **ngrok** | `setup-tunnel.ps1` | Easiest, provides HTTPS URLs. 1GB/month free |
| **Tailscale Funnel** | `tailscale funnel 8080` | Requires Tailscale on both machines. No bandwidth limits |
| **Cloudflare Tunnel** | `cloudflared tunnel` | Good if your domain is on Cloudflare |
| **WireGuard VPN** | Manual VPN setup | Most secure, direct connection. No third party |

## Why This Architecture

- **You own the data** — no third-party database provider has access to your users' data
- **Supabase-like experience** — your users get a managed database UI/API, but the data lives physically on hardware you control
- **Cost** — no per-GB database hosting fees. Just your VPS + laptop electricity
- **Privacy** — great for compliance (GDPR, HIPAA) where data residency matters
- **Simplicity** — a single Go binary that runs in both cloud and laptop modes

## Development

```bash
# Unit tests
go test -v -count=1 ./...

# Build
go build -o dbhost.exe .

# Run tests with coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

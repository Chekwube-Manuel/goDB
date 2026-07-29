# DB Hosting Prototype

This project is designed for a cloud-control-plane / laptop-data-plane architecture. The public API and dashboard live on a cloud server, while the actual user databases remain on your laptop. That gives you a Supabase-like experience where users interact with a managed platform, but the data physically lives on your own hardware.

## What is included

- Go HTTP API for tenant, collection, and record management
- PostgreSQL for durable data storage
- Redis for caching
- Docker Compose for PostgreSQL, Redis, and the app
- Basic HTTP authentication with configurable credentials
- CORS support for a dashboard frontend

## Architecture idea

- Control plane: cloud server hosts the API, auth, dashboard, and orchestration.
- Data plane: your laptop hosts PostgreSQL, Redis, and the user databases.
- The cloud server talks to your laptop over a secure tunnel or VPN.
- The data stays on your laptop, which makes this closer to a self-hosted Supabase-style model than a normal cloud database.

## How this differs from a normal cloud database

- In AWS, Supabase, or Neon, the provider hosts the data plane.
- In your design, you host the data plane yourself on your laptop.
- The cloud server handles routing, auth, billing, and the public API experience.

## Run with Docker Compose

```bash
docker compose up --build
```

The app will be available on:

- http://localhost:8080/health

Use these default credentials:

- Username: admin
- Password: changeme

You can override them with:

```bash
export AUTH_USERNAME=your-user
export AUTH_PASSWORD=your-password
export ALLOWED_ORIGIN=https://your-dashboard.example.com
```

## Local development without Docker

Start PostgreSQL and Redis locally, then run:

```bash
go run .
```

Set the environment variables before running:

```bash
export DATABASE_URL=postgres://postgres:postgres@localhost:5432/appdb?sslmode=disable
export REDIS_ADDR=localhost:6379
export AUTH_USERNAME=admin
export AUTH_PASSWORD=changeme
export ALLOWED_ORIGIN=http://localhost:3000
```

## Notes

This is still a self-hosted prototype for personal or small-team use. It is suitable for a laptop-based deployment with a small number of users, but it still needs hardening for public production use, including backups, monitoring, and HTTPS termination.

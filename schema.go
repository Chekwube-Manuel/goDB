package main

import "context"

func (s *service) initSchema(ctx context.Context) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS tenants (
			slug TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS collections (
			id SERIAL PRIMARY KEY,
			tenant_slug TEXT NOT NULL REFERENCES tenants(slug) ON DELETE CASCADE,
			name TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE(tenant_slug, name)
		)`,
		`CREATE TABLE IF NOT EXISTS records (
			id SERIAL PRIMARY KEY,
			tenant_slug TEXT NOT NULL REFERENCES tenants(slug) ON DELETE CASCADE,
			collection_name TEXT NOT NULL,
			payload JSONB NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS data_nodes (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			endpoint TEXT NOT NULL,
			token_hash TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'offline',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS provisioned_databases (
			id SERIAL PRIMARY KEY,
			node_id INT NOT NULL REFERENCES data_nodes(id) ON DELETE CASCADE,
			database_id TEXT NOT NULL UNIQUE,
			region TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'provisioning',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
	}
	for _, query := range queries {
		if _, err := s.db.ExecContext(ctx, query); err != nil {
			return err
		}
	}
	return nil
}

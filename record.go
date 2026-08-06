package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

func (s *service) storeRecord(w http.ResponseWriter, r *http.Request, tenantSlug, collectionName string) {
	var payload recordPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	data, err := json.Marshal(payload.Data)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "record payload must be JSON serializable"})
		return
	}
	ctx := r.Context()

	// Records are only valid inside an existing tenant + collection.
	var exists bool
	if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(
		SELECT 1 FROM tenants WHERE slug = $1
	) AND EXISTS(
		SELECT 1 FROM collections WHERE tenant_slug = $1 AND name = $2
	)`, tenantSlug, collectionName).Scan(&exists); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if !exists {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "tenant or collection not found"})
		return
	}

	if _, err := s.db.ExecContext(ctx, `INSERT INTO records (tenant_slug, collection_name, payload) VALUES ($1, $2, $3)`, tenantSlug, collectionName, data); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	_ = s.rdb.Set(ctx, cacheKey(tenantSlug, collectionName), "dirty", 5*time.Minute).Err()
	writeJSON(w, http.StatusCreated, map[string]string{"status": "stored", "tenant": tenantSlug, "collection": collectionName})
}

func (s *service) listRecords(w http.ResponseWriter, r *http.Request, tenantSlug, collectionName string) {
	ck := cacheKey(tenantSlug, collectionName)
	ctx := r.Context()
	if cached, err := s.rdb.Get(ctx, ck).Result(); err == nil && cached == "dirty" {
		_ = s.rdb.Del(ctx, ck).Err()
	}

	var records []recordResult
	if cached, err := s.rdb.Get(ctx, ck).Bytes(); err == nil {
		if err := json.Unmarshal(cached, &records); err == nil {
			writeJSON(w, http.StatusOK, map[string]any{"tenant": tenantSlug, "collection": collectionName, "records": records})
			return
		}
	}

	rows, err := s.db.QueryContext(ctx, `SELECT id, tenant_slug, collection_name, payload, created_at FROM records WHERE tenant_slug = $1 AND collection_name = $2 ORDER BY created_at DESC`, tenantSlug, collectionName)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	for rows.Next() {
		var item recordResult
		if err := rows.Scan(&item.ID, &item.TenantSlug, &item.Collection, &item.Payload, &item.CreatedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		records = append(records, item)
	}
	if payload, err := json.Marshal(records); err == nil {
		_ = s.rdb.Set(ctx, ck, payload, 2*time.Minute).Err()
	}
	writeJSON(w, http.StatusOK, map[string]any{"tenant": tenantSlug, "collection": collectionName, "records": records})
}

func cacheKey(tenantSlug, collectionName string) string {
	return fmt.Sprintf("records:%s:%s", tenantSlug, collectionName)
}

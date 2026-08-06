package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// nodeHTTPClient is used for all cloud↔laptop traffic so calls cannot hang
// indefinitely when a node goes offline.
var nodeHTTPClient = &http.Client{Timeout: 30 * time.Second}

// configuredNodeEndpoint returns the node endpoint to forward to: the explicit
// NODE_ENDPOINT config wins, otherwise the most recently seen online node from
// the registry is used (requires a database, i.e. cloud mode).
func (s *service) configuredNodeEndpoint(ctx context.Context) (string, error) {
	if s.cfg.NodeEndpoint != "" {
		return s.cfg.NodeEndpoint, nil
	}
	if s.db == nil {
		return "", fmt.Errorf("node endpoint not configured")
	}
	var endpoint string
	err := s.db.QueryRowContext(ctx,
		`SELECT endpoint FROM data_nodes WHERE status = 'online' ORDER BY last_seen_at DESC LIMIT 1`,
	).Scan(&endpoint)
	if err != nil {
		return "", fmt.Errorf("no online node registered")
	}
	return endpoint, nil
}

// forwardRequest forwards any HTTP request to the configured node endpoint.
// It preserves the method, path, headers, and body so the laptop can handle
// it exactly as if it received the request directly.
func (s *service) forwardRequest(r *http.Request) (*http.Response, error) {
	endpoint, err := s.configuredNodeEndpoint(r.Context())
	if err != nil {
		return nil, err
	}

	targetURL := strings.TrimRight(endpoint, "/") + r.URL.Path
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	var body io.Reader
	if r.Body != nil {
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read request body: %w", err)
		}
		body = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create forwarded request: %w", err)
	}

	for key, vals := range r.Header {
		for _, v := range vals {
			req.Header.Add(key, v)
		}
	}

	if s.cfg.NodeToken != "" {
		req.Header.Set("Authorization", "Bearer "+s.cfg.NodeToken)
	}
	req.Header.Set("X-Forwarded-For", r.RemoteAddr)
	req.Header.Set("X-Forwarded-Host", r.Host)

	resp, err := nodeHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("forward request failed: %w", err)
	}
	return resp, nil
}

// forwardRequestAndReply forwards to the node and writes the laptop's response
// back to the original client, preserving status code and body.
func (s *service) forwardRequestAndReply(w http.ResponseWriter, r *http.Request) {
	resp, err := s.forwardRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error":  "upstream node unreachable",
			"detail": err.Error(),
		})
		return
	}
	defer resp.Body.Close()

	for key, vals := range resp.Header {
		for _, v := range vals {
			w.Header().Add(key, v)
		}
	}

	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// forwardPayloadRequest sends a JSON payload to a specific node path and
// decodes the response into the provided target.
func (s *service) forwardPayloadRequest(ctx context.Context, method, path string, payload, target interface{}) error {
	if s.cfg.NodeEndpoint == "" {
		return fmt.Errorf("node endpoint not configured")
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	endpoint := strings.TrimRight(s.cfg.NodeEndpoint, "/") + path
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if s.cfg.NodeToken != "" {
		req.Header.Set("Authorization", "Bearer "+s.cfg.NodeToken)
	}

	resp, err := nodeHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("forward request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("node returned status %d: %s", resp.StatusCode, string(respBody))
	}

	if target != nil {
		return json.NewDecoder(resp.Body).Decode(target)
	}
	return nil
}

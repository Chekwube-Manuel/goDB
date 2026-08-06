package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

// AgentConfig holds settings for the laptop agent that reports to the cloud.
type AgentConfig struct {
	CloudEndpoint   string `json:"cloud_endpoint"`
	NodeName        string `json:"node_name"`
	NodeToken       string `json:"node_token"`
	HeartbeatSec    int    `json:"heartbeat_sec"`
	MaxRetrySec     int    `json:"max_retry_sec"`
}

func loadAgentConfig() AgentConfig {
	return AgentConfig{
		CloudEndpoint: getenv("CLOUD_ENDPOINT", ""),
		NodeName:      getenv("NODE_NAME", "laptop-1"),
		NodeToken:     getenv("NODE_TOKEN", ""),
		HeartbeatSec:  getenvInt("HEARTBEAT_SEC", 30),
		MaxRetrySec:   getenvInt("MAX_RETRY_SEC", 300),
	}
}

// startAgent begins the background agent loop that registers with the cloud
// and sends periodic heartbeats. Runs in a goroutine.
func startAgent(ctx context.Context, svc *service) {
	ac := loadAgentConfig()
	if ac.CloudEndpoint == "" || ac.NodeToken == "" {
		log.Println("agent: CLOUD_ENDPOINT or NODE_TOKEN not set — running in standalone mode")
		return
	}

	log.Printf("agent: starting laptop agent for node '%s' → %s", ac.NodeName, ac.CloudEndpoint)

	// Initial registration
	if err := agentRegister(ctx, ac); err != nil {
		log.Printf("agent: initial registration failed: %v (will retry)", err)
	}

	// Heartbeat loop
	ticker := time.NewTicker(time.Duration(ac.HeartbeatSec) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("agent: shutting down")
			return
		case <-ticker.C:
			if err := agentHeartbeat(ctx, ac); err != nil {
				log.Printf("agent: heartbeat failed: %v", err)
			} else {
				log.Printf("agent: heartbeat sent (%s online)", ac.NodeName)
			}
		}
	}
}

// agentRegister sends a registration request to the cloud.
func agentRegister(ctx context.Context, ac AgentConfig) error {
	payload := map[string]string{
		"name":     ac.NodeName,
		"endpoint": ac.CloudEndpoint,
		"token":    ac.NodeToken,
	}
	body, _ := json.Marshal(payload)

	endpoint := ac.CloudEndpoint + "/nodes"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("create register request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	// Authenticate with the cloud using the shared node token so registration
	// works without admin basic-auth credentials on the agent.
	req.Header.Set("Authorization", "Bearer "+ac.NodeToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("register request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("register returned %d", resp.StatusCode)
	}
	log.Printf("agent: registered as '%s' on cloud", ac.NodeName)
	return nil
}

// agentHeartbeat sends a heartbeat to the cloud.
func agentHeartbeat(ctx context.Context, ac AgentConfig) error {
	payload := map[string]string{
		"name":   ac.NodeName,
		"status": "online",
	}
	body, _ := json.Marshal(payload)

	endpoint := ac.CloudEndpoint + "/node/heartbeat"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("create heartbeat: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Use node token for inter-server auth
	req.Header.Set("Authorization", "Bearer "+ac.NodeToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("heartbeat request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("heartbeat returned %d", resp.StatusCode)
	}
	return nil
}

package main

import (
	"net/http"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

func hashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func comparePassword(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

func (s *service) isAuthenticated(r *http.Request) bool {
	if s.cfg.AuthUsername == "" || s.cfg.AuthPassword == "" {
		return true
	}
	username, password, ok := r.BasicAuth()
	if !ok {
		return false
	}
	if username != s.cfg.AuthUsername {
		return false
	}
	if s.authPasswordHash == "" {
		return password == s.cfg.AuthPassword
	}
	return comparePassword(s.authPasswordHash, password) == nil
}

func (s *service) isNodeAuthenticated(r *http.Request) bool {
	if s.cfg.NodeAuthToken == "" {
		return true
	}
	authorization := r.Header.Get("Authorization")
	if authorization == "" {
		return false
	}
	return strings.TrimPrefix(authorization, "Bearer ") != authorization && strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer ")) == s.cfg.NodeAuthToken
}

// isRequestAuthorized accepts either admin basic auth or a trusted node's
// bearer token. The laptop (data plane) needs this so requests forwarded by
// the cloud (which arrive with `Authorization: Bearer <node-token>`) pass,
// while direct admin access with basic auth still works.
func (s *service) isRequestAuthorized(r *http.Request) bool {
	return s.isAuthenticated(r) || s.isNodeAuthenticated(r)
}

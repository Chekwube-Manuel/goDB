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

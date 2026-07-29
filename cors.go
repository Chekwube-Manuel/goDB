package main

import (
	"net/http"
	"strings"
)

func (s *service) setCORSHeaders(w http.ResponseWriter, r *http.Request) {
	origin := s.cfg.AllowedOrigin
	if origin == "" {
		origin = "*"
	}
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
	if origin != "*" {
		w.Header().Set("Vary", "Origin")
	}
}

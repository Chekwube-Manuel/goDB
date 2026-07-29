package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	var port int
	var mode string
	flag.IntVar(&port, "port", loadConfig().Port, "HTTP port to listen on")
	flag.StringVar(&mode, "mode", "", "run mode: cloud or laptop (auto-detected from env if empty)")
	flag.Parse()

	cfg := loadConfig()
	cfg.Port = port

	if mode == "" {
		if cfg.NodeEndpoint != "" {
			mode = "cloud"
		} else {
			mode = "laptop"
		}
	}

	svc, err := newService(cfg)
	if err != nil {
		log.Fatalf("failed to initialize backend: %v", err)
	}
	defer func() {
		if err := svc.close(); err != nil {
			log.Printf("shutdown error: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if mode == "laptop" {
		go startAgent(ctx, svc)
	}

	server := &http.Server{
		Addr:              fmt.Sprintf("0.0.0.0:%d", cfg.Port),
		Handler:           svc,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("dbhost [%s mode] listening on %s", mode, server.Addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server failed: %v", err)
		}
	}()

	<-stop
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	_ = server.Shutdown(shutdownCtx)
}

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
	flag.IntVar(&port, "port", loadConfig().Port, "HTTP port to listen on")
	flag.Parse()

	cfg := loadConfig()
	cfg.Port = port

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

	server := &http.Server{
		Addr:              fmt.Sprintf("0.0.0.0:%d", cfg.Port),
		Handler:           svc,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("self-hosted backend listening on %s", server.Addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server failed: %v", err)
		}
	}()

	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}

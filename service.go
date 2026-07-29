package main

import (
	"context"
	"database/sql"
	"errors"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/redis/go-redis/v9"
)

type service struct {
	cfg              config
	db               *sql.DB
	rdb              *redis.Client
	limiter          *rateLimiter
	authPasswordHash string
}

func newService(cfg config) (*service, error) {
	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := rdb.Ping(ctx).Err(); err != nil {
		_ = db.Close()
		_ = rdb.Close()
		return nil, err
	}

	svc := &service{cfg: cfg, db: db, rdb: rdb, limiter: newRateLimiter(cfg.RateLimitBurst, cfg.RateLimitWindow)}
	if err := svc.initSchema(ctx); err != nil {
		_ = db.Close()
		_ = rdb.Close()
		return nil, err
	}
	if cfg.AuthPassword != "" {
		hashed, err := hashPassword(cfg.AuthPassword)
		if err != nil {
			_ = db.Close()
			_ = rdb.Close()
			return nil, err
		}
		svc.authPasswordHash = hashed
	}
	return svc, nil
}

func (s *service) close() error {
	var errs []error
	if s.db != nil {
		errs = append(errs, s.db.Close())
	}
	if s.rdb != nil {
		errs = append(errs, s.rdb.Close())
	}
	for _, err := range errs {
		if err != nil && !errors.Is(err, sql.ErrConnDone) {
			return err
		}
	}
	return nil
}

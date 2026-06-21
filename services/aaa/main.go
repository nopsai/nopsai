package main

import (
	"context"
	"net/http"
	"os"
	"strings"
	"time"

	"nopsai/config"
	"nopsai/pkg/buildinfo"
	"nopsai/pkg/httpapi"
	"nopsai/pkg/servicelog"
	"nopsai/pkg/startupgates"
	"nopsai/services/aaa/pkg/authz"
	"nopsai/services/aaa/pkg/server"
	"nopsai/services/aaa/pkg/store"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

const defaultSharedInternalToken = "dev-default-for-local-only"

func main() {
	if buildinfo.Requested(os.Args[1:]) {
		_ = buildinfo.WriteVersion(os.Stdout, "nopsai-aaa")
		return
	}
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "config.yml"
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load config")
	}

	if err := servicelog.Configure(cfg.LogLevel, cfg.LogFormat); err != nil {
		log.Warn().Str("log_level", cfg.LogLevel).Msg("Invalid log level; defaulting to info")
	}

	if strings.TrimSpace(cfg.DatabaseURL) == "" {
		log.Fatal().Msg("DATABASE_URL must be configured for AAA")
	}
	if strings.TrimSpace(cfg.AAAAddr) == "" {
		cfg.AAAAddr = ":8082"
	}
	if strings.TrimSpace(cfg.AAASharedToken) == "" {
		cfg.AAASharedToken = defaultSharedInternalToken
	}
	if err := startupgates.ValidateAAA(cfg); err != nil {
		log.Fatal().Err(err).Msg("aaa startup gates failed")
	}

	var dbpool *pgxpool.Pool
	for attempt := 0; attempt < 5; attempt++ {
		dbpool, err = pgxpool.New(context.Background(), cfg.DatabaseURL)
		if err == nil {
			err = dbpool.Ping(context.Background())
		}
		if err == nil {
			break
		}
		log.Warn().Err(err).Int("attempt", attempt+1).Msg("failed to connect to database, retrying")
		time.Sleep(3 * time.Second)
	}
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to database")
	}
	defer dbpool.Close()

	backend := store.NewPGStore(dbpool)
	if err := backend.EnsureSchema(context.Background()); err != nil {
		log.Fatal().Err(err).Msg("failed to ensure aaa schema")
	}
	evaluator := authz.NewEvaluator(backend)
	httpServer := httpapi.NewServer(cfg.AAAAddr, server.New(cfg.AAASharedToken, evaluator).Handler())

	log.Info().Str("addr", cfg.AAAAddr).Msg("aaa listening")
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal().Err(err).Msg("aaa server failed")
	}
}

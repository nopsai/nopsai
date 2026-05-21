package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"nopsai/config"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

type setupPreflightCheck struct {
	ID           string            `json:"id"`
	Label        string            `json:"label"`
	Status       string            `json:"status"`
	Message      string            `json:"message"`
	Required     bool              `json:"required"`
	SuggestedEnv map[string]string `json:"suggested_env,omitempty"`
}

type setupPreflightResponse struct {
	Ready       bool                  `json:"ready"`
	CanLogin    bool                  `json:"can_login"`
	Mode        string                `json:"mode"`
	ConfigPath  string                `json:"config_path,omitempty"`
	EnvFilePath string                `json:"env_file_path,omitempty"`
	Checks      []setupPreflightCheck `json:"checks"`
}

func buildSetupPreflightResponse(ctx context.Context, cfg *config.Config, configPath, envFilePath, mode string, db *pgxpool.Pool, dbErr error) setupPreflightResponse {
	if cfg == nil {
		cfg = &config.Config{}
	}
	resp := setupPreflightResponse{
		Ready:       true,
		CanLogin:    true,
		Mode:        mode,
		ConfigPath:  configPath,
		EnvFilePath: envFilePath,
	}
	add := func(check setupPreflightCheck) {
		if check.Required && check.Status == "error" {
			resp.Ready = false
			resp.CanLogin = false
		}
		resp.Checks = append(resp.Checks, check)
	}

	dbURL := strings.TrimSpace(cfg.DatabaseURL)
	switch {
	case dbURL == "":
		add(setupPreflightCheck{
			ID:       "database_url",
			Label:    "Database URL",
			Status:   "error",
			Required: true,
			Message:  "DATABASE_URL is required before the API can authenticate users or store setup state.",
			SuggestedEnv: map[string]string{
				"DATABASE_URL": "postgres://nopsai_user:yoursecurepassword@nopsai-db:5432/nopsai_db",
			},
		})
	case dbErr != nil:
		add(setupPreflightCheck{
			ID:       "database_connection",
			Label:    "Database connection",
			Status:   "error",
			Required: true,
			Message:  fmt.Sprintf("Database is not reachable: %v", dbErr),
			SuggestedEnv: map[string]string{
				"DATABASE_URL": dbURL,
			},
		})
	case db == nil:
		add(setupPreflightCheck{
			ID:       "database_connection",
			Label:    "Database connection",
			Status:   "error",
			Required: true,
			Message:  "Database pool is not initialized.",
		})
	default:
		pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		err := db.Ping(pingCtx)
		cancel()
		if err != nil {
			add(setupPreflightCheck{
				ID:       "database_connection",
				Label:    "Database connection",
				Status:   "error",
				Required: true,
				Message:  fmt.Sprintf("Database is not reachable: %v", err),
			})
		} else {
			add(setupPreflightCheck{
				ID:       "database_connection",
				Label:    "Database connection",
				Status:   "success",
				Required: true,
				Message:  "Database is reachable.",
			})
		}
	}

	if strings.TrimSpace(cfg.MasterKey) == "" {
		add(setupPreflightCheck{
			ID:       "master_key",
			Label:    "Master encryption key",
			Status:   "error",
			Required: true,
			Message:  "NOPSAI_MASTER_KEY is required before encrypted secrets can be used.",
			SuggestedEnv: map[string]string{
				"NOPSAI_MASTER_KEY": "$(openssl rand -base64 32)",
			},
		})
	} else {
		add(setupPreflightCheck{
			ID:       "master_key",
			Label:    "Master encryption key",
			Status:   "success",
			Required: true,
			Message:  "Master encryption key is configured.",
		})
	}

	if strings.TrimSpace(cfg.JWTSigningKey) == "" {
		add(setupPreflightCheck{
			ID:       "jwt_signing_key",
			Label:    "JWT signing key",
			Status:   "error",
			Required: true,
			Message:  "JWT_SIGNING_KEY is required before local login can mint browser sessions.",
			SuggestedEnv: map[string]string{
				"JWT_SIGNING_KEY": "$(openssl rand -base64 48)",
			},
		})
	} else {
		add(setupPreflightCheck{
			ID:       "jwt_signing_key",
			Label:    "JWT signing key",
			Status:   "success",
			Required: true,
			Message:  "JWT signing key is configured.",
		})
	}

	if strings.TrimSpace(cfg.EffectiveServiceJWTSigningKey()) == "" {
		add(setupPreflightCheck{
			ID:       "service_jwt_signing_key",
			Label:    "Service JWT signing key",
			Status:   "warning",
			Required: false,
			Message:  "SERVICE_JWT_SIGNING_KEY is recommended for dispatcher, runner, and agent callbacks. It can fall back to JWT_SIGNING_KEY.",
			SuggestedEnv: map[string]string{
				"SERVICE_JWT_SIGNING_KEY": "$(openssl rand -base64 48)",
			},
		})
	} else {
		add(setupPreflightCheck{
			ID:       "service_jwt_signing_key",
			Label:    "Service JWT signing key",
			Status:   "success",
			Required: false,
			Message:  "Service JWT signing key is available.",
		})
	}

	aaaToken := strings.TrimSpace(cfg.AAASharedToken)
	if aaaToken == "" || aaaToken == "dev-default-for-local-only" {
		add(setupPreflightCheck{
			ID:       "aaa_shared_token",
			Label:    "AAA shared internal token",
			Status:   "warning",
			Required: false,
			Message:  "Set a strong AAA_SHARED_INTERNAL_TOKEN before production use.",
			SuggestedEnv: map[string]string{
				"AAA_SHARED_INTERNAL_TOKEN": "$(openssl rand -base64 32)",
			},
		})
	} else {
		add(setupPreflightCheck{
			ID:       "aaa_shared_token",
			Label:    "AAA shared internal token",
			Status:   "success",
			Required: false,
			Message:  "AAA shared internal token is configured.",
		})
	}

	if strings.TrimSpace(cfg.GitHubWebhookSecret) == "" {
		add(setupPreflightCheck{
			ID:       "github_webhook_secret",
			Label:    "GitHub webhook secret",
			Status:   "warning",
			Required: false,
			Message:  "GitHub webhook signing can be configured in the setup wizard after login.",
			SuggestedEnv: map[string]string{
				"GITHUB_WEBHOOK_SECRET": "$(openssl rand -base64 32)",
			},
		})
	}

	return resp
}

func (a *App) handleSetupPreflight(w http.ResponseWriter, r *http.Request) {
	cfg := a.getConfigSnapshot()
	resp := buildSetupPreflightResponse(r.Context(), &cfg, a.configPath, a.envFilePath, "ready", a.db, nil)
	writeJSON(w, http.StatusOK, resp)
}

func runSetupPreflightOnlyServer(cfg *config.Config, configPath, envFilePath string, db *pgxpool.Pool, dbErr error) {
	addr := strings.TrimSpace(cfg.NopsaiListenAddress)
	if addr == "" {
		addr = "0.0.0.0:8080"
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/setup/preflight", func(w http.ResponseWriter, r *http.Request) {
		resp := buildSetupPreflightResponse(r.Context(), cfg, configPath, envFilePath, "preflight_only", db, dbErr)
		status := http.StatusOK
		if !resp.Ready {
			status = http.StatusServiceUnavailable
		}
		writeJSON(w, status, resp)
	})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		resp := buildSetupPreflightResponse(r.Context(), cfg, configPath, envFilePath, "preflight_only", db, dbErr)
		if !resp.Ready {
			writeJSON(w, http.StatusServiceUnavailable, resp)
			return
		}
		writeJSON(w, http.StatusOK, resp)
	})

	var handler http.Handler = mux
	handler = recoveryMiddleware(handler)
	handler = loggingMiddleware(handler)
	handler = requestIDMiddleware(handler)
	handler = corsMiddleware(handler)

	server := &http.Server{Addr: addr, Handler: handler}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	go func() {
		log.Warn().Str("addr", addr).Msg("NopsAI API started in setup preflight mode")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("Failed to start setup preflight server")
		}
	}()
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}

func connectDatabaseWithRetries(ctx context.Context, databaseURL string, attempts int, delay time.Duration) (*pgxpool.Pool, error) {
	if strings.TrimSpace(databaseURL) == "" {
		return nil, fmt.Errorf("database URL is empty")
	}
	var lastErr error
	for i := 0; i < attempts; i++ {
		dbpool, err := pgxpool.New(ctx, databaseURL)
		if err == nil {
			if err = dbpool.Ping(ctx); err == nil {
				log.Info().Msg("Successfully connected to the database.")
				return dbpool, nil
			}
			dbpool.Close()
		}
		lastErr = err
		if i < attempts-1 {
			log.Warn().Err(err).Msgf("Unable to connect to database. Retrying in %s...", delay)
			time.Sleep(delay)
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("database connection failed")
	}
	return nil, lastErr
}

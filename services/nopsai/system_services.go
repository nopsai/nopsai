package nopsai

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"nopsai/pkg/proto"
)

const serviceHealthTimeout = 900 * time.Millisecond

type systemServiceStatus struct {
	ID        string    `json:"id"`
	Label     string    `json:"label"`
	Status    string    `json:"status"`
	Message   string    `json:"message,omitempty"`
	CheckedAt time.Time `json:"checked_at"`
}

func (a *App) buildSystemServiceStatuses(ctx context.Context, dispatcherStatus *proto.DispatcherStatus, dispatcherErr error) []systemServiceStatus {
	checkedAt := time.Now().UTC()
	cfg := a.getConfigSnapshot()
	services := make([]systemServiceStatus, 0, 6)
	add := func(id, label, status, message string) {
		services = append(services, systemServiceStatus{
			ID:        id,
			Label:     label,
			Status:    status,
			Message:   message,
			CheckedAt: checkedAt,
		})
	}

	add("nopsai-api", "NopsAI API", "ok", "Serving this status request.")

	if a.db == nil {
		add("database", "Database", "error", "Database pool is unavailable.")
	} else {
		dbCtx, cancel := context.WithTimeout(ctx, serviceHealthTimeout)
		err := a.db.Ping(dbCtx)
		cancel()
		if err != nil {
			add("database", "Database", "error", "Database is not reachable.")
		} else {
			add("database", "Database", "ok", "Connected.")
		}
	}

	if dispatcherErr != nil {
		add("dispatcher", "Dispatcher", "error", "Dispatcher status is unavailable.")
		add("runners", "Runners", "error", "Runner capacity cannot be checked while dispatcher is unavailable.")
	} else if dispatcherStatus == nil {
		add("dispatcher", "Dispatcher", "warning", "Dispatcher status has not been loaded.")
		add("runners", "Runners", "warning", "Runner capacity has not been loaded.")
	} else {
		runnerCount := len(dispatcherStatus.GetRunners())
		unreachableCount := runnerUnreachableCount(dispatcherStatus)
		recoveredCount := runnerRecoveredCount(dispatcherStatus, checkedAt)
		add("dispatcher", "Dispatcher", "ok", "Connected.")
		if runnerCount == 0 {
			add("runners", "Runners", "warning", "No runners are registered.")
		} else if unreachableCount > 0 {
			message := fmt.Sprintf("%d runner(s) registered, %d unreachable.", runnerCount, unreachableCount)
			if recoveredCount > 0 {
				message = fmt.Sprintf("%d runner(s) registered, %d unreachable, %d recently reconnected.", runnerCount, unreachableCount, recoveredCount)
			}
			add("runners", "Runners", "warning", message)
		} else if recoveredCount > 0 {
			add("runners", "Runners", "warning", fmt.Sprintf("%d runner(s) registered, %d recently reconnected.", runnerCount, recoveredCount))
		} else {
			add("runners", "Runners", "ok", fmt.Sprintf("%d runner(s) registered.", runnerCount))
		}
	}

	aaaURL := strings.TrimSpace(cfg.AAAAPIURL)
	if aaaURL == "" {
		aaaURL = "http://aaa:8082"
	}
	status, message := a.checkHTTPHealth(ctx, aaaURL, "/healthz")
	add("aaa", "AAA", status, message)

	gitBotURL := strings.TrimSpace(cfg.NopsaiGitBotAPIURL)
	if gitBotURL == "" {
		add("git-bot", "git-bot", "warning", "NopsAI to git-bot URL is not configured.")
	} else {
		status, message := a.checkHTTPHealth(ctx, gitBotURL, "/healthz")
		add("git-bot", "git-bot", status, message)
	}

	return services
}

func (a *App) checkHTTPHealth(ctx context.Context, baseURL, path string) (string, string) {
	target, err := healthURL(baseURL, path)
	if err != nil {
		return "error", "Health endpoint URL is invalid."
	}

	healthCtx, cancel := context.WithTimeout(ctx, serviceHealthTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(healthCtx, http.MethodGet, target, nil)
	if err != nil {
		return "error", "Health request could not be built."
	}

	client := a.httpClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return "error", "Health endpoint is not reachable."
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 160))
		detail := strings.TrimSpace(string(body))
		if detail != "" {
			return "error", fmt.Sprintf("Health endpoint returned HTTP %d: %s", resp.StatusCode, detail)
		}
		return "error", fmt.Sprintf("Health endpoint returned HTTP %d.", resp.StatusCode)
	}
	return "ok", "Reachable."
}

func healthURL(baseURL, path string) (string, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if trimmed == "" {
		return "", fmt.Errorf("base URL is empty")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("base URL must include scheme and host")
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return trimmed + path, nil
}

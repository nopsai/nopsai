package proxy

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var (
	dockerAPIPrefix  = `/(?:v[0-9.]+/)?`
	pingPath         = regexp.MustCompile(`^` + dockerAPIPrefix + `_ping$`)
	versionPath      = regexp.MustCompile(`^` + dockerAPIPrefix + `version$`)
	containerList    = regexp.MustCompile(`^` + dockerAPIPrefix + `containers/json$`)
	containerInspect = regexp.MustCompile(`^` + dockerAPIPrefix + `containers/[^/]+/json$`)
	containerLogs    = regexp.MustCompile(`^` + dockerAPIPrefix + `containers/[^/]+/logs$`)
	runnerContainer  = regexp.MustCompile(`^runner(?:[-_.][A-Za-z0-9][A-Za-z0-9_.-]*)?$`)
)

type Handler struct {
	proxy             *httputil.ReverseProxy
	allowedContainers map[string]struct{}
}

var DefaultAllowedContainers = []string{"nopsai", "nopsai-aaa", "nopsai-dispatcher", "nopsai-git-bot", "nopsai-ui", "nopsai-docker-runner", "nopsai-k8s-runner"}

func New(socketPath string, allowedContainers ...string) *Handler {
	if len(allowedContainers) == 0 {
		allowedContainers = DefaultAllowedContainers
	}
	allowed := make(map[string]struct{}, len(allowedContainers))
	for _, name := range allowedContainers {
		if name = strings.TrimSpace(name); name != "" {
			allowed[name] = struct{}{}
		}
	}
	transport := &http.Transport{
		Proxy:                 nil,
		MaxIdleConns:          20,
		MaxIdleConnsPerHost:   20,
		IdleConnTimeout:       90 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "unix", socketPath)
		},
	}
	reverse := httputil.NewSingleHostReverseProxy(&url.URL{Scheme: "http", Host: "docker"})
	reverse.Transport = transport
	reverse.FlushInterval = -1
	reverse.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, _ error) {
		http.Error(w, "Docker API unavailable", http.StatusBadGateway)
	}
	return &Handler{proxy: reverse, allowedContainers: allowed}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.proxy == nil {
		http.Error(w, "Docker API unavailable", http.StatusServiceUnavailable)
		return
	}
	if err := validateRequest(r, h.allowedContainers); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	h.proxy.ServeHTTP(w, r)
}

func validateRequest(r *http.Request, allowedContainers map[string]struct{}) error {
	if r == nil {
		return errors.New("request is required")
	}
	path := strings.TrimSpace(r.URL.Path)
	if (r.Method == http.MethodGet || r.Method == http.MethodHead) && (pingPath.MatchString(path) || versionPath.MatchString(path)) {
		return validateQuery(r.URL.Query(), nil)
	}
	if r.Method != http.MethodGet {
		return errors.New("Docker API method is not allowed")
	}
	switch {
	case containerList.MatchString(path):
		return validateQuery(r.URL.Query(), map[string]struct{}{"all": {}, "limit": {}, "size": {}, "filters": {}})
	case containerInspect.MatchString(path):
		if err := validateContainerName(path, allowedContainers); err != nil {
			return err
		}
		return validateQuery(r.URL.Query(), map[string]struct{}{"size": {}})
	case containerLogs.MatchString(path):
		if err := validateContainerName(path, allowedContainers); err != nil {
			return err
		}
		return validateQuery(r.URL.Query(), map[string]struct{}{
			"follow": {}, "stdout": {}, "stderr": {}, "since": {}, "until": {}, "timestamps": {}, "tail": {}, "details": {},
		})
	default:
		return errors.New("Docker API path is not allowed")
	}
}

func validateContainerName(path string, allowed map[string]struct{}) error {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	for index, part := range parts {
		if part != "containers" || index+1 >= len(parts) {
			continue
		}
		name, err := url.PathUnescape(parts[index+1])
		if err != nil {
			return errors.New("Docker container name is invalid")
		}
		if _, ok := allowed[name]; !ok {
			if !runnerContainer.MatchString(name) {
				return errors.New("Docker container is not allow-listed")
			}
		}
		return nil
	}
	return errors.New("Docker container name is missing")
}

func validateQuery(query url.Values, allowed map[string]struct{}) error {
	for key := range query {
		if _, ok := allowed[key]; !ok {
			return errors.New("Docker API query parameter is not allowed")
		}
	}
	return nil
}

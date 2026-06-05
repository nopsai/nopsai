package runnerinstall

import (
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"nopsai/config"
)

func RequestExternalBaseURL(r *http.Request) string {
	if r == nil {
		return ""
	}
	proto := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")[0])
	if proto == "" {
		if r.TLS != nil {
			proto = "https"
		} else {
			proto = "http"
		}
	}
	host := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Host"), ",")[0])
	if host == "" {
		host = r.Host
	}
	return proto + "://" + host
}

func ExternalDispatcherAddress(cfg config.Config, r *http.Request) (string, bool, []string) {
	configured := strings.TrimSpace(cfg.DispatcherAddress)
	if configured == "" {
		configured = "localhost:9090"
	}
	host := addressHost(configured)
	port := addressPort(configured, addressPort(cfg.DispatcherListenAddress, "9090"))
	if !isInternalAddressHost(host) {
		return configured, false, nil
	}
	requestHost := requestHostForExternalAddress(r)
	if requestHost == "" {
		return configured, false, []string{"The dispatcher address could not be adapted because the request host was empty."}
	}
	return net.JoinHostPort(requestHost, port), true, nil
}

func LooksInternalAddress(raw string) bool {
	return isInternalAddressHost(addressHost(raw))
}

func requestHostForExternalAddress(r *http.Request) string {
	if r == nil {
		return ""
	}
	for _, raw := range []string{r.Header.Get("X-Forwarded-Host"), r.Host} {
		first := strings.TrimSpace(strings.Split(raw, ",")[0])
		if first == "" {
			continue
		}
		return stripAddressPort(first)
	}
	return ""
}

func addressHost(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.Contains(raw, "://") {
		if parsed, err := url.Parse(raw); err == nil {
			return stripAddressPort(parsed.Host)
		}
	}
	return stripAddressPort(raw)
}

func addressPort(raw, fallback string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	if strings.Contains(raw, "://") {
		if parsed, err := url.Parse(raw); err == nil {
			if port := parsed.Port(); port != "" {
				return port
			}
		}
	}
	if _, port, err := net.SplitHostPort(raw); err == nil && port != "" {
		return port
	}
	lastColon := strings.LastIndex(raw, ":")
	if lastColon >= 0 && lastColon < len(raw)-1 && !strings.Contains(raw[lastColon+1:], "]") {
		candidate := raw[lastColon+1:]
		if _, err := strconv.Atoi(candidate); err == nil {
			return candidate
		}
	}
	return fallback
}

func stripAddressPort(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(raw); err == nil {
		return strings.Trim(host, "[]")
	}
	if strings.HasPrefix(raw, "[") {
		if idx := strings.Index(raw, "]"); idx >= 0 {
			return strings.Trim(raw[1:idx], "[]")
		}
	}
	if idx := strings.LastIndex(raw, ":"); idx > 0 && !strings.Contains(raw[:idx], ":") {
		if _, err := strconv.Atoi(raw[idx+1:]); err == nil {
			return raw[:idx]
		}
	}
	return strings.Trim(raw, "[]")
}

func isInternalAddressHost(host string) bool {
	host = strings.ToLower(strings.Trim(strings.TrimSpace(host), "[]"))
	if host == "" {
		return true
	}
	switch host {
	case "localhost", "127.0.0.1", "::1", "0.0.0.0", "dispatcher", "nopsai-dispatcher", "nopsai":
		return true
	}
	return strings.HasPrefix(host, "127.")
}

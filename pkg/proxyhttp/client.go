package proxyhttp

import (
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// NewInternalAwareClient returns an HTTP client that bypasses configured
// proxies for Docker-style internal hosts while still honoring them for
// external traffic.
func NewInternalAwareClient(timeout time.Duration) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = internalAwareProxy
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}
}

func internalAwareProxy(req *http.Request) (*url.URL, error) {
	if req != nil && shouldBypassProxy(req.URL.Hostname()) {
		return nil, nil
	}
	return http.ProxyFromEnvironment(req)
}

func shouldBypassProxy(host string) bool {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "" {
		return false
	}

	if host == "localhost" {
		return true
	}

	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()
	}

	// Single-label hostnames like "aaa" or "nopsai-git-bot" are Docker service
	// names and should not be routed through external proxies.
	return !strings.Contains(host, ".")
}

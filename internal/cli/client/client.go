package client

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const defaultUserAgent = "nopsai-cli/dev"

type Client struct {
	baseURL    *url.URL
	token      string
	httpClient *http.Client
	userAgent  string
}

type Options struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
	UserAgent  string
}

func New(options Options) (*Client, error) {
	baseURL, err := url.Parse(strings.TrimSpace(options.BaseURL))
	if err != nil {
		return nil, fmt.Errorf("parse API URL: %w", err)
	}
	if (baseURL.Scheme != "http" && baseURL.Scheme != "https") || baseURL.Host == "" {
		return nil, errors.New("API URL must be an absolute http or https URL")
	}
	if baseURL.User != nil || baseURL.RawQuery != "" || baseURL.Fragment != "" {
		return nil, errors.New("API URL cannot include credentials, a query, or a fragment")
	}
	token := strings.TrimSpace(options.Token)
	if strings.ContainsAny(token, "\r\n") {
		return nil, errors.New("API token cannot contain a newline")
	}
	baseURL.Path = strings.TrimRight(baseURL.Path, "/") + "/"
	baseURL.RawPath = ""

	httpClient := options.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{}
	} else {
		clone := *httpClient
		httpClient = &clone
	}
	originalRedirect := httpClient.CheckRedirect
	httpClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) > 0 && !sameOrigin(via[0].URL, req.URL) {
			return errors.New("refusing to forward credentials across API origins")
		}
		if originalRedirect != nil {
			return originalRedirect(req, via)
		}
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		return nil
	}

	userAgent := strings.TrimSpace(options.UserAgent)
	if userAgent == "" {
		userAgent = defaultUserAgent
	}
	return &Client{
		baseURL:    baseURL,
		token:      token,
		httpClient: httpClient,
		userAgent:  userAgent,
	}, nil
}

func (c *Client) Do(req *http.Request) (*http.Response, error) {
	return c.do(req, true)
}

func (c *Client) DoUnauthenticated(req *http.Request) (*http.Response, error) {
	return c.do(req, false)
}

func (c *Client) do(req *http.Request, authenticated bool) (*http.Response, error) {
	if req.URL.IsAbs() || req.URL.Host != "" || !strings.HasPrefix(req.URL.Path, "/") {
		return nil, errors.New("API request path must be absolute and host-free")
	}
	for _, segment := range strings.Split(req.URL.Path, "/") {
		if segment == ".." {
			return nil, errors.New("API request path cannot contain parent traversal")
		}
	}
	target := c.baseURL.ResolveReference(&url.URL{
		Path:     strings.TrimPrefix(req.URL.Path, "/"),
		RawQuery: req.URL.RawQuery,
	})
	request := req.Clone(req.Context())
	request.URL = target
	request.RequestURI = ""
	request.Host = ""
	if request.Header.Get("Accept") == "" {
		request.Header.Set("Accept", "application/json")
	}
	request.Header.Set("User-Agent", c.userAgent)
	if authenticated && c.token != "" {
		request.Header.Set("Authorization", "Bearer "+c.token)
	} else if !authenticated {
		request.Header.Del("Authorization")
	}
	return c.httpClient.Do(request)
}

func (c *Client) NewRequest(method, path string, body io.Reader) (*http.Request, error) {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(path))
	if err != nil {
		return nil, fmt.Errorf("parse API request path: %w", err)
	}
	if !strings.HasPrefix(parsed.Path, "/") {
		return nil, errors.New("API request path must start with /")
	}
	request, err := http.NewRequest(method, parsed.String(), body)
	if err != nil {
		return nil, fmt.Errorf("build API request: %w", err)
	}
	return request, nil
}

func sameOrigin(left, right *url.URL) bool {
	return strings.EqualFold(left.Scheme, right.Scheme) && strings.EqualFold(left.Host, right.Host)
}

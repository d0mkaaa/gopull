package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Auth struct {
	Type  string // none, bearer, basic
	Token string
	User  string
	Pass  string
}

type Options struct {
	SkipTLSVerify    bool
	DisableRedirects bool
	ProxyURL         string
}

type Request struct {
	Method  string
	URL     string
	Headers map[string]string
	Body    string
	Auth    Auth
	Env     map[string]string
	Options Options
}

type Response struct {
	Status     string
	StatusCode int
	Headers    http.Header
	Body       []byte
	Elapsed    time.Duration
	Stream     io.ReadCloser
	StartTime  time.Time
}

func Send(ctx context.Context, req Request) (*Response, error) {
	method := strings.ToUpper(strings.TrimSpace(req.Method))
	if method == "" {
		method = http.MethodGet
	}

	rawURL := substitute(req.URL, req.Env)

	var body io.Reader
	if req.Body != "" {
		body = bytes.NewBufferString(substitute(req.Body, req.Env))
	}

	httpReq, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	for k, v := range req.Headers {
		httpReq.Header.Set(substitute(k, req.Env), substitute(v, req.Env))
	}

	switch req.Auth.Type {
	case "bearer":
		if t := substitute(req.Auth.Token, req.Env); t != "" {
			httpReq.Header.Set("Authorization", "Bearer "+t)
		}
	case "basic":
		httpReq.SetBasicAuth(
			substitute(req.Auth.User, req.Env),
			substitute(req.Auth.Pass, req.Env),
		)
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	if req.Options.SkipTLSVerify {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
	}
	if req.Options.ProxyURL != "" {
		proxyURL, err := url.Parse(req.Options.ProxyURL)
		if err == nil {
			transport.Proxy = http.ProxyURL(proxyURL)
		}
	}

	c := &http.Client{Transport: transport}
	if req.Options.DisableRedirects {
		c.CheckRedirect = func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}

	start := time.Now()
	resp, err := c.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send: %w", err)
	}

	if strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		return &Response{
			Status:     resp.Status,
			StatusCode: resp.StatusCode,
			Headers:    resp.Header,
			Stream:     resp.Body,
			StartTime:  start,
		}, nil
	}

	defer resp.Body.Close()
	buf, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	return &Response{
		Status:     resp.Status,
		StatusCode: resp.StatusCode,
		Headers:    resp.Header,
		Body:       buf,
		Elapsed:    time.Since(start),
		StartTime:  start,
	}, nil
}

func substitute(s string, env map[string]string) string {
	if len(env) == 0 || !strings.Contains(s, "{{") {
		return s
	}
	pairs := make([]string, 0, len(env)*2)
	for k, v := range env {
		pairs = append(pairs, "{{"+k+"}}", v)
	}
	return strings.NewReplacer(pairs...).Replace(s)
}

package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
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
	UseCookieJar     bool
	CABundlePath     string
	ClientCertPath   string
	ClientKeyPath    string
}

type Request struct {
	Method  string
	URL     string
	Headers map[string]string
	Body    string
	Auth    Auth
	Env     map[string]string
	Options Options
	Jar     *cookiejar.Jar
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
	tlsConfig := &tls.Config{}
	if req.Options.SkipTLSVerify {
		tlsConfig.InsecureSkipVerify = true //nolint:gosec
	}
	if req.Options.CABundlePath != "" {
		pool, err := loadCertPool(substitute(req.Options.CABundlePath, req.Env))
		if err != nil {
			return nil, err
		}
		tlsConfig.RootCAs = pool
	}
	if req.Options.ClientCertPath != "" {
		certPath := substitute(req.Options.ClientCertPath, req.Env)
		keyPath := substitute(req.Options.ClientKeyPath, req.Env)
		if keyPath == "" {
			keyPath = certPath
		}
		cert, err := tls.LoadX509KeyPair(certPath, keyPath)
		if err != nil {
			return nil, fmt.Errorf("load client cert: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}
	if req.Options.SkipTLSVerify || req.Options.CABundlePath != "" || req.Options.ClientCertPath != "" {
		transport.TLSClientConfig = tlsConfig
	}
	if req.Options.ProxyURL != "" {
		proxyURL, err := url.Parse(req.Options.ProxyURL)
		if err == nil {
			transport.Proxy = http.ProxyURL(proxyURL)
		}
	} else {
		transport.Proxy = http.ProxyFromEnvironment
	}

	c := &http.Client{Transport: transport}
	if req.Options.UseCookieJar {
		if req.Jar != nil {
			c.Jar = req.Jar
		} else if jar, err := cookiejar.New(nil); err == nil {
			c.Jar = jar
		}
	}
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

func loadCertPool(path string) (*x509.CertPool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read CA bundle: %w", err)
	}
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	if !pool.AppendCertsFromPEM(data) {
		return nil, fmt.Errorf("read CA bundle: no certificates found")
	}
	return pool, nil
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

package core

import (
	"context"
	"net/url"
	"strings"

	"github.com/d0mkaaa/gopull/internal/client"
	"github.com/d0mkaaa/gopull/internal/plugins"
	"github.com/d0mkaaa/gopull/internal/store"
)

type Env struct {
	Values     map[string]string
	SecretKeys map[string]bool
}

type RunOptions struct {
	Plugins *plugins.Runner
}

type RunResult struct {
	Request    store.Request
	Response   *client.Response
	EnvUpdates map[string]string
	PluginLogs []string
}

func RunRequest(ctx context.Context, req store.Request, env Env, opts RunOptions) (*RunResult, error) {
	result := &RunResult{Request: req}
	pluginCtx := plugins.HookContext{Env: env.Values, SecretKeys: env.SecretKeys}
	if opts.Plugins != nil {
		var logs []string
		req, logs = opts.Plugins.RunPreRequest(req, pluginCtx)
		result.PluginLogs = append(result.PluginLogs, logs...)
		result.Request = req
	}

	cr := BuildClientRequest(req, env.Values)
	resp, err := client.Send(ctx, cr)
	if err != nil {
		return result, err
	}
	result.Response = resp
	if resp.Stream != nil {
		return result, nil
	}

	if opts.Plugins != nil {
		snap := plugins.RespSnapshot{
			StatusCode:  resp.StatusCode,
			ElapsedMs:   resp.Elapsed.Milliseconds(),
			SizeBytes:   len(resp.Body),
			Body:        string(resp.Body),
			ContentType: resp.Headers.Get("Content-Type"),
		}
		updates, logs := opts.Plugins.RunPostResponse(req, snap, pluginCtx)
		result.EnvUpdates = updates
		result.PluginLogs = append(result.PluginLogs, logs...)
	}
	return result, nil
}

func BuildClientRequest(req store.Request, envVars map[string]string) client.Request {
	cr := client.Request{
		Method:  req.Method,
		URL:     req.URL,
		Body:    req.Body.Raw,
		Env:     envVars,
		Auth:    client.Auth{Type: req.Auth.Type, Token: req.Auth.Token, User: req.Auth.User, Pass: req.Auth.Pass},
		Headers: make(map[string]string, len(req.Headers)),
		Options: client.Options{
			SkipTLSVerify:    req.Options.SkipTLSVerify,
			DisableRedirects: req.Options.DisableRedirects,
			ProxyURL:         req.Options.ProxyURL,
		},
	}
	for _, h := range req.Headers {
		if h.Enabled {
			cr.Headers[h.Key] = h.Value
		}
	}
	switch req.Body.Mode {
	case "form":
		vals := url.Values{}
		for _, line := range strings.Split(req.Body.Raw, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			idx := strings.Index(line, ":")
			if idx < 0 {
				continue
			}
			vals.Set(strings.TrimSpace(line[:idx]), strings.TrimSpace(line[idx+1:]))
		}
		cr.Body = vals.Encode()
		cr.Headers["Content-Type"] = "application/x-www-form-urlencoded"
	case "graphql":
		cr.Body = req.Body.Raw
		if _, ok := cr.Headers["Content-Type"]; !ok {
			cr.Headers["Content-Type"] = "application/json"
		}
	}
	return cr
}

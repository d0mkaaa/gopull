package core

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
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
	Jar     *cookiejar.Jar
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

	cr, err := BuildClientRequest(req, env.Values)
	if err != nil {
		return result, err
	}
	cr.Jar = opts.Jar
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

func BuildClientRequest(req store.Request, envVars map[string]string) (client.Request, error) {
	cr := client.Request{
		Method:  req.Method,
		URL:     applyPathParams(req.URL, req.Path, envVars),
		Body:    req.Body.Raw,
		Env:     envVars,
		Auth:    client.Auth{Type: req.Auth.Type, Token: req.Auth.Token, User: req.Auth.User, Pass: req.Auth.Pass},
		Headers: make(map[string]string, len(req.Headers)),
		Options: client.Options{
			SkipTLSVerify:    req.Options.SkipTLSVerify,
			DisableRedirects: req.Options.DisableRedirects,
			ProxyURL:         req.Options.ProxyURL,
			UseCookieJar:     req.Options.UseCookieJar,
			CABundlePath:     req.Options.CABundlePath,
			ClientCertPath:   req.Options.ClientCertPath,
			ClientKeyPath:    req.Options.ClientKeyPath,
		},
	}
	for _, h := range req.Headers {
		if h.Enabled {
			cr.Headers[h.Key] = h.Value
		}
	}
	cr.URL = applyQueryParams(cr.URL, req.Query, envVars)
	switch req.Body.Mode {
	case "form":
		vals := url.Values{}
		for _, line := range strings.Split(req.Body.Raw, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			k, v, ok := splitParamLine(line)
			if ok {
				vals.Set(replaceVars(k, envVars), replaceVars(v, envVars))
			}
		}
		cr.Body = vals.Encode()
		cr.Headers["Content-Type"] = "application/x-www-form-urlencoded"
	case "graphql":
		cr.Body = req.Body.Raw
		if _, ok := cr.Headers["Content-Type"]; !ok {
			cr.Headers["Content-Type"] = "application/json"
		}
	case "multipart":
		body, contentType, err := buildMultipartBody(req.Body.Raw, envVars)
		if err != nil {
			return cr, err
		}
		cr.Body = body
		cr.Headers["Content-Type"] = contentType
	case "file":
		path := strings.TrimSpace(replaceVars(req.Body.Raw, envVars))
		if path != "" {
			data, err := os.ReadFile(path)
			if err != nil {
				return cr, fmt.Errorf("read body file: %w", err)
			}
			cr.Body = string(data)
			if _, ok := cr.Headers["Content-Type"]; !ok {
				cr.Headers["Content-Type"] = contentTypeFromPath(path)
			}
		}
	}
	return cr, nil
}

func applyQueryParams(rawURL string, params []store.Param, envVars map[string]string) string {
	if len(params) == 0 {
		return rawURL
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	q := u.Query()
	for _, p := range params {
		if p.Enabled && strings.TrimSpace(p.Key) != "" {
			q.Set(replaceVars(strings.TrimSpace(p.Key), envVars), replaceVars(p.Value, envVars))
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func applyPathParams(rawURL string, params []store.Param, envVars map[string]string) string {
	if len(params) == 0 || !strings.Contains(rawURL, ":") {
		return rawURL
	}
	values := map[string]string{}
	for _, p := range params {
		if p.Enabled && strings.TrimSpace(p.Key) != "" {
			values[strings.TrimSpace(p.Key)] = replaceVars(p.Value, envVars)
		}
	}
	if len(values) == 0 {
		return rawURL
	}
	var b strings.Builder
	for i := 0; i < len(rawURL); i++ {
		if rawURL[i] != ':' {
			b.WriteByte(rawURL[i])
			continue
		}
		if i+1 < len(rawURL) && rawURL[i+1] == ':' {
			b.WriteByte(':')
			i++
			continue
		}
		j := i + 1
		for j < len(rawURL) && isPathParamChar(rawURL[j]) {
			j++
		}
		if j == i+1 {
			b.WriteByte(rawURL[i])
			continue
		}
		key := rawURL[i+1 : j]
		if v, ok := values[key]; ok {
			b.WriteString(url.PathEscape(v))
		} else {
			b.WriteString(rawURL[i:j])
		}
		i = j - 1
	}
	return b.String()
}

func isPathParamChar(b byte) bool {
	return b == '_' || b == '-' || b >= '0' && b <= '9' || b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z'
}

func buildMultipartBody(raw string, envVars map[string]string) (string, string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "file ") {
			k, path, ok := splitParamLine(strings.TrimSpace(strings.TrimPrefix(line, "file ")))
			if !ok {
				continue
			}
			if err := addMultipartFile(w, replaceVars(k, envVars), replaceVars(path, envVars)); err != nil {
				return "", "", err
			}
			continue
		}
		k, v, ok := splitParamLine(line)
		if ok {
			if err := w.WriteField(replaceVars(k, envVars), replaceVars(v, envVars)); err != nil {
				return "", "", err
			}
		}
	}
	if err := w.Close(); err != nil {
		return "", "", err
	}
	return buf.String(), w.FormDataContentType(), nil
}

func addMultipartFile(w *multipart.Writer, key, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open multipart file: %w", err)
	}
	defer f.Close()
	part, err := w.CreateFormFile(key, filepath.Base(path))
	if err != nil {
		return err
	}
	if _, err := io.Copy(part, f); err != nil {
		return err
	}
	return nil
}

func splitParamLine(line string) (string, string, bool) {
	idx := strings.Index(line, "=")
	if idx < 0 {
		idx = strings.Index(line, ":")
	}
	if idx < 0 {
		return "", "", false
	}
	k := strings.TrimSpace(line[:idx])
	v := strings.TrimSpace(line[idx+1:])
	return k, v, k != ""
}

func contentTypeFromPath(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		return "application/json"
	case ".xml":
		return "application/xml"
	case ".txt", ".log":
		return "text/plain; charset=utf-8"
	case ".html", ".htm":
		return "text/html; charset=utf-8"
	default:
		return "application/octet-stream"
	}
}

func replaceVars(s string, env map[string]string) string {
	if len(env) == 0 || !strings.Contains(s, "{{") {
		return s
	}
	pairs := make([]string, 0, len(env)*2)
	for k, v := range env {
		pairs = append(pairs, "{{"+k+"}}", v)
	}
	return strings.NewReplacer(pairs...).Replace(s)
}

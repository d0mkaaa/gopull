// Package plugins runs lifecycle hooks against external executables.
//
// A plugin is any executable placed in ~/.config/gopull/plugins/.
// On startup gopull calls each executable with --manifest to learn
// which hooks it handles.  During a request lifecycle it calls the
// executable again with --hook <name>, passing a JSON payload on
// stdin and reading a JSON result from stdout.
//
// Hook names: pre_request, post_response.  v1 manifests must declare
// permissions for env/secret access and request/env mutation.
//
// Example v1 manifest (stdout of `plugin --manifest`):
//
//	{"name":"aws-sig","version":"1.0.0","api_version":"v1","hooks":["pre_request"],"permissions":["read_env","read_body","read_secrets","write_headers"]}
//
// Example pre_request stdin:
//
//	{"method":"GET","url":"...","headers":[...],"body":{...},"auth":{...},"env":{...}}
//
// Example pre_request stdout (omit unchanged fields):
//
//	{"headers":[{"key":"X-Amz-Date","value":"20240101T000000Z","enabled":true}]}
//
// Example post_response stdout:
//
//	{"env_updates":{"ACCESS_TOKEN":"extracted-value"}}
package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/d0mkaaa/gopull/internal/store"
)

const (
	manifestTimeout = 2 * time.Second
	hookTimeout     = 5 * time.Second
)

const (
	PermissionReadEnv      = "read_env"
	PermissionReadBody     = "read_body"
	PermissionReadSecrets  = "read_secrets"
	PermissionWriteBody    = "write_body"
	PermissionWriteEnv     = "write_env"
	PermissionWriteHeaders = "write_headers"
	PermissionWriteRequest = "write_request"
)

// Manifest is returned by a plugin when called with --manifest.
type Manifest struct {
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	APIVersion  string   `json:"api_version,omitempty"`
	Hooks       []string `json:"hooks"`
	Permissions []string `json:"permissions,omitempty"`
}

// RespSnapshot is the response data passed to post_response hooks.
type RespSnapshot struct {
	StatusCode  int    `json:"status_code"`
	ElapsedMs   int64  `json:"elapsed_ms"`
	SizeBytes   int    `json:"size_bytes"`
	Body        string `json:"body"`
	ContentType string `json:"content_type"`
}

type preRequestPayload struct {
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers []store.Header    `json:"headers"`
	Body    store.Body        `json:"body"`
	Auth    store.Auth        `json:"auth"`
	Env     map[string]string `json:"env"`
}

type preRequestResult struct {
	Method  string         `json:"method,omitempty"`
	URL     string         `json:"url,omitempty"`
	Headers []store.Header `json:"headers,omitempty"`
	Body    *store.Body    `json:"body,omitempty"`
}

type postResponsePayload struct {
	Request  preRequestPayload `json:"request"`
	Response RespSnapshot      `json:"response"`
}

type postResponseResult struct {
	EnvUpdates map[string]string `json:"env_updates,omitempty"`
}

type HookContext struct {
	Env        map[string]string
	SecretKeys map[string]bool
}

type pluginEntry struct {
	path     string
	manifest Manifest
}

// Runner holds the loaded set of plugins for a session.
type Runner struct {
	plugins []pluginEntry
}

// Load scans pluginsDir for executables, calls --manifest on each, and
// returns a Runner ready to fire hooks.  Plugins that fail to respond
// to --manifest are silently skipped.
func Load(pluginsDir string) *Runner {
	r := &Runner{}
	entries, err := os.ReadDir(pluginsDir)
	if err != nil {
		return r
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		path := filepath.Join(pluginsDir, e.Name())
		if !isExecutable(e) {
			continue
		}
		m, err := fetchManifest(path)
		if err != nil {
			continue
		}
		r.plugins = append(r.plugins, pluginEntry{path: path, manifest: m})
	}
	return r
}

// Count returns the number of loaded plugins.
func (r *Runner) Count() int { return len(r.plugins) }

// Names returns the display names of all loaded plugins.
func (r *Runner) Names() []string {
	out := make([]string, len(r.plugins))
	for i, p := range r.plugins {
		out[i] = p.manifest.Name
	}
	return out
}

// RunPreRequest calls every plugin that registered the pre_request hook,
// sequentially, and merges their modifications into req.  Logs collect
// any stderr or error output for display in the UI.
func (r *Runner) RunPreRequest(req store.Request, ctx HookContext) (store.Request, []string) {
	var logs []string
	for _, p := range r.plugins {
		if !hasHook(p.manifest, "pre_request") {
			continue
		}
		payload := buildPreRequestPayload(req, p.manifest, ctx)
		out, stderr, err := callPlugin(p.path, "pre_request", payload)
		if stderr != "" {
			logs = append(logs, fmt.Sprintf("[%s] %s", p.manifest.Name, stderr))
		}
		if err != nil {
			logs = append(logs, fmt.Sprintf("[%s] hook error: %v", p.manifest.Name, err))
			continue
		}
		var res preRequestResult
		if err := json.Unmarshal(out, &res); err != nil {
			logs = append(logs, fmt.Sprintf("[%s] bad output: %v", p.manifest.Name, err))
			continue
		}
		if res.Method != "" && canWriteRequest(p.manifest) {
			req.Method = res.Method
		}
		if res.URL != "" && canWriteRequest(p.manifest) {
			req.URL = res.URL
		}
		if res.Headers != nil && canWriteHeaders(p.manifest) {
			req.Headers = res.Headers
		}
		if res.Body != nil && canWriteBody(p.manifest) {
			req.Body = *res.Body
		}
	}
	return req, logs
}

// RunPostResponse calls every plugin that registered the post_response hook
// and merges any env_updates they return.
func (r *Runner) RunPostResponse(req store.Request, resp RespSnapshot, ctx HookContext) (map[string]string, []string) {
	updates := map[string]string{}
	var logs []string
	for _, p := range r.plugins {
		if !hasHook(p.manifest, "post_response") {
			continue
		}
		payload := postResponsePayload{
			Request:  buildPreRequestPayload(req, p.manifest, ctx),
			Response: resp,
		}
		out, stderr, err := callPlugin(p.path, "post_response", payload)
		if stderr != "" {
			logs = append(logs, fmt.Sprintf("[%s] %s", p.manifest.Name, stderr))
		}
		if err != nil {
			logs = append(logs, fmt.Sprintf("[%s] hook error: %v", p.manifest.Name, err))
			continue
		}
		var res postResponseResult
		if err := json.Unmarshal(out, &res); err != nil {
			logs = append(logs, fmt.Sprintf("[%s] bad output: %v", p.manifest.Name, err))
			continue
		}
		if canWriteEnv(p.manifest) {
			for k, v := range res.EnvUpdates {
				updates[k] = v
			}
		}
	}
	if len(updates) == 0 {
		return nil, logs
	}
	return updates, logs
}

func buildPreRequestPayload(req store.Request, m Manifest, ctx HookContext) preRequestPayload {
	env := pluginEnv(m, ctx)
	auth := req.Auth
	if !canReadSecrets(m) {
		auth.Token = ""
		auth.Pass = ""
	}
	body := req.Body
	if !canReadBody(m) {
		body.Raw = ""
	}
	return preRequestPayload{
		Method:  req.Method,
		URL:     req.URL,
		Headers: req.Headers,
		Body:    body,
		Auth:    auth,
		Env:     env,
	}
}

func pluginEnv(m Manifest, ctx HookContext) map[string]string {
	if len(ctx.Env) == 0 || !canReadEnv(m) {
		return nil
	}
	env := make(map[string]string, len(ctx.Env))
	for k, v := range ctx.Env {
		if ctx.SecretKeys[k] && !canReadSecrets(m) {
			continue
		}
		env[k] = v
	}
	return env
}

func canReadSecrets(m Manifest) bool {
	if m.APIVersion == "" {
		return true
	}
	return hasPermission(m, PermissionReadSecrets)
}

func canReadEnv(m Manifest) bool {
	if m.APIVersion == "" {
		return true
	}
	return hasPermission(m, PermissionReadEnv) || hasPermission(m, PermissionReadSecrets)
}

func canReadBody(m Manifest) bool {
	return m.APIVersion == "" || hasPermission(m, PermissionReadBody)
}

func canWriteBody(m Manifest) bool {
	return m.APIVersion == "" || hasPermission(m, PermissionWriteBody)
}

func canWriteHeaders(m Manifest) bool {
	return m.APIVersion == "" || hasPermission(m, PermissionWriteHeaders)
}

func canWriteEnv(m Manifest) bool {
	return m.APIVersion == "" || hasPermission(m, PermissionWriteEnv)
}

func canWriteRequest(m Manifest) bool {
	return m.APIVersion == "" || hasPermission(m, PermissionWriteRequest)
}

func fetchManifest(path string) (Manifest, error) {
	ctx, cancel := context.WithTimeout(context.Background(), manifestTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "--manifest").Output()
	if err != nil {
		return Manifest{}, err
	}
	var m Manifest
	return m, json.Unmarshal(out, &m)
}

func callPlugin(path, hook string, payload any) ([]byte, string, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), hookTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "--hook", hook)
	cmd.Stdin = bytes.NewReader(data)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, strings.TrimSpace(stderr.String()), err
	}
	return stdout.Bytes(), strings.TrimSpace(stderr.String()), nil
}

func hasHook(m Manifest, hook string) bool {
	for _, h := range m.Hooks {
		if h == hook {
			return true
		}
	}
	return false
}

func hasPermission(m Manifest, permission string) bool {
	for _, p := range m.Permissions {
		if p == permission {
			return true
		}
	}
	return false
}

func isExecutable(e os.DirEntry) bool {
	if runtime.GOOS == "windows" {
		name := strings.ToLower(e.Name())
		return strings.HasSuffix(name, ".exe") ||
			strings.HasSuffix(name, ".cmd") ||
			strings.HasSuffix(name, ".bat")
	}
	info, err := e.Info()
	if err != nil {
		return false
	}
	return info.Mode()&0o111 != 0
}

// Package tests holds integration tests that span multiple internal packages.
// Unit tests live next to the source they cover (Go convention); this package
// is for scenarios that require the full pipeline - store + client + data
// flowing between them - and would feel artificial split across packages.
package tests

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/d0mkaaa/gopull/internal/client"
	"github.com/d0mkaaa/gopull/internal/curlparse"
	"github.com/d0mkaaa/gopull/internal/store"
)

// newStore builds a Store rooted at a fresh temporary directory.
func newStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.NewAt(t.TempDir())
	if err != nil {
		t.Fatalf("store.NewAt: %v", err)
	}
	return s
}

func TestSendRequestAndSaveHistory(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, `{"status":"ok","path":"`+r.URL.Path+`"}`)
	}))
	defer srv.Close()

	s := newStore(t)

	resp, err := client.Send(context.Background(), client.Request{
		Method: "GET",
		URL:    srv.URL + "/api/test",
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: %d", resp.StatusCode)
	}

	// Persist to history.
	entry := store.HistoryEntry{
		Timestamp: time.Now(),
		Request:   store.HistReq{Method: "GET", URL: srv.URL + "/api/test"},
		Response: store.HistResp{
			StatusCode:  resp.StatusCode,
			ElapsedMs:   resp.Elapsed.Milliseconds(),
			SizeBytes:   len(resp.Body),
			Body:        string(resp.Body),
			ContentType: resp.Headers.Get("Content-Type"),
		},
	}
	if err := s.AppendHistory(entry); err != nil {
		t.Fatalf("AppendHistory: %v", err)
	}

	h, err := s.LoadHistory()
	if err != nil {
		t.Fatalf("LoadHistory: %v", err)
	}
	if len(h.Entries) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(h.Entries))
	}
	if h.Entries[0].Response.StatusCode != 200 {
		t.Errorf("history status: %d", h.Entries[0].Response.StatusCode)
	}
	if !strings.Contains(h.Entries[0].Response.Body, "ok") {
		t.Errorf("history body: %q", h.Entries[0].Response.Body)
	}
}

func TestEnvSubstitutionEndToEnd(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"ok":true}`)
	}))
	defer srv.Close()

	s := newStore(t)

	// Persist an environment.
	env := &store.Environment{
		Name: "local",
		Variables: []store.EnvVar{
			{Key: "BASE_URL", Value: srv.URL, Enabled: true},
			{Key: "RESOURCE", Value: "/users/42", Enabled: true},
		},
	}
	if err := s.SaveEnvironment(env); err != nil {
		t.Fatalf("SaveEnvironment: %v", err)
	}

	// Build an env map from enabled variables.
	envMap := make(map[string]string)
	for _, v := range env.Variables {
		if v.Enabled {
			envMap[v.Key] = v.Value
		}
	}

	_, err := client.Send(context.Background(), client.Request{
		Method: "GET",
		URL:    "{{BASE_URL}}{{RESOURCE}}",
		Env:    envMap,
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if gotPath != "/users/42" {
		t.Errorf("path after substitution: got %q, want /users/42", gotPath)
	}
}

func TestCollectionPersistsAndSendsRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		io.WriteString(w, `{"id":99}`)
	}))
	defer srv.Close()

	s := newStore(t)

	col := &store.Collection{Name: "My API", Requests: make(map[string]*store.Request)}
	if err := s.SaveCollection(col); err != nil {
		t.Fatalf("SaveCollection: %v", err)
	}

	req := &store.Request{
		Name:   "Create item",
		Method: "POST",
		URL:    srv.URL + "/items",
		Body:   store.Body{Mode: "raw", Raw: `{"name":"test"}`},
		Headers: []store.Header{
			{Key: "Content-Type", Value: "application/json", Enabled: true},
		},
	}
	if err := s.SaveRequest(col.ID, req); err != nil {
		t.Fatalf("SaveRequest: %v", err)
	}

	// Reload from disk.
	cols, err := s.LoadCollections()
	if err != nil {
		t.Fatalf("LoadCollections: %v", err)
	}
	if len(cols) != 1 || len(cols[0].Requests) != 1 {
		t.Fatalf("unexpected collections/requests: %v", cols)
	}

	loaded := cols[0].Requests[req.ID]
	headers := make(map[string]string)
	for _, h := range loaded.Headers {
		if h.Enabled {
			headers[h.Key] = h.Value
		}
	}

	resp, err := client.Send(context.Background(), client.Request{
		Method:  loaded.Method,
		URL:     loaded.URL,
		Body:    loaded.Body.Raw,
		Headers: headers,
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("status: got %d, want 201", resp.StatusCode)
	}
}

func TestCurlImportToStoreToSend(t *testing.T) {
	var gotHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"ok":true}`)
	}))
	defer srv.Close()

	// User pastes a curl command.
	curlCmd := `curl ` + srv.URL + `/data -H "Authorization: Bearer testtoken" -H "Accept: application/json"`

	parsed, err := curlparse.Parse(curlCmd)
	if err != nil {
		t.Fatalf("curlparse.Parse: %v", err)
	}

	s := newStore(t)
	col, err := s.EnsureDefaultCollection()
	if err != nil {
		t.Fatalf("EnsureDefaultCollection: %v", err)
	}
	if err := s.SaveRequest(col.ID, &parsed); err != nil {
		t.Fatalf("SaveRequest: %v", err)
	}

	// Reload and send.
	cols, _ := s.LoadCollections()
	r := cols[0].Requests[parsed.ID]

	headers := make(map[string]string)
	for _, h := range r.Headers {
		if h.Enabled {
			headers[h.Key] = h.Value
		}
	}

	resp, err := client.Send(context.Background(), client.Request{
		Method:  r.Method,
		URL:     r.URL,
		Headers: headers,
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: %d", resp.StatusCode)
	}
	if gotHeader != "Bearer testtoken" {
		t.Errorf("Authorization header: got %q", gotHeader)
	}
}

func TestPostmanImportRoundTrip(t *testing.T) {
	s := newStore(t)

	pm := []byte(`{
		"info": {"name": "Widgets API"},
		"item": [
			{"name": "List", "request": {"method": "GET", "url": {"raw": "https://api.example.com/widgets"}}},
			{"name": "Create", "request": {
				"method": "POST",
				"url": {"raw": "https://api.example.com/widgets"},
				"header": [{"key": "Content-Type", "value": "application/json", "disabled": false}],
				"body": {"mode": "raw", "raw": "{\"name\":\"sprocket\"}"}
			}}
		]
	}`)

	col, err := store.ImportPostman(pm)
	if err != nil {
		t.Fatalf("ImportPostman: %v", err)
	}
	if err := s.SaveCollection(col); err != nil {
		t.Fatalf("SaveCollection: %v", err)
	}

	cols, err := s.LoadCollections()
	if err != nil {
		t.Fatalf("LoadCollections: %v", err)
	}
	if len(cols) != 1 {
		t.Fatalf("expected 1 collection, got %d", len(cols))
	}
	if cols[0].Name != "Widgets API" {
		t.Errorf("name: %q", cols[0].Name)
	}
	if len(cols[0].Requests) != 2 {
		t.Errorf("requests: %d", len(cols[0].Requests))
	}
}

func TestConfigPersistsAcrossInstances(t *testing.T) {
	base := t.TempDir()

	s1, err := store.NewAt(base)
	if err != nil {
		t.Fatalf("store 1: %v", err)
	}
	if err := s1.SaveConfig(&store.Config{TimeoutSecs: 45, Theme: "gruvbox"}); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	s2, err := store.NewAt(base)
	if err != nil {
		t.Fatalf("store 2: %v", err)
	}
	cfg, err := s2.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.TimeoutSecs != 45 {
		t.Errorf("timeout: got %d, want 45", cfg.TimeoutSecs)
	}
	if cfg.Theme != "gruvbox" {
		t.Errorf("theme: got %q", cfg.Theme)
	}
}

func TestHistoryOrderedNewestFirst(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"path":"`+r.URL.Path+`"}`)
	}))
	defer srv.Close()

	s := newStore(t)

	urls := []string{"/first", "/second", "/third"}
	for _, u := range urls {
		resp, err := client.Send(context.Background(), client.Request{
			Method: "GET", URL: srv.URL + u,
		})
		if err != nil {
			t.Fatalf("Send %s: %v", u, err)
		}
		_ = s.AppendHistory(store.HistoryEntry{
			Request:  store.HistReq{Method: "GET", URL: srv.URL + u},
			Response: store.HistResp{StatusCode: resp.StatusCode},
		})
	}

	h, err := s.LoadHistory()
	if err != nil {
		t.Fatalf("LoadHistory: %v", err)
	}
	if len(h.Entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(h.Entries))
	}
	// Newest (last appended) should be first.
	if !strings.HasSuffix(h.Entries[0].Request.URL, "/third") {
		t.Errorf("first entry should be /third, got %q", h.Entries[0].Request.URL)
	}
}

func TestResponseBodyIsParseable(t *testing.T) {
	type payload struct {
		Status string `json:"status"`
		Count  int    `json:"count"`
	}
	want := payload{Status: "running", Count: 42}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(want)
	}))
	defer srv.Close()

	resp, err := client.Send(context.Background(), client.Request{URL: srv.URL})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	var got payload
	if err := json.Unmarshal(resp.Body, &got); err != nil {
		t.Fatalf("Unmarshal body: %v - body was: %s", err, resp.Body)
	}
	if got.Status != want.Status || got.Count != want.Count {
		t.Errorf("body: got %+v, want %+v", got, want)
	}
}

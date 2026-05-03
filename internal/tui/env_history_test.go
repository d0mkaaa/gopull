package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/d0mkaaa/gopull/internal/store"
)

func TestParseEnvVarLinesSupportsDisabledAndSecret(t *testing.T) {
	got := parseEnvVarLines("BASE_URL=https://api.example.com\nsecret TOKEN=abc\n# DISABLED=yes\n")
	if len(got) != 3 {
		t.Fatalf("got %d vars, want 3", len(got))
	}
	if got[0].Key != "BASE_URL" || !got[0].Enabled || got[0].Secret {
		t.Fatalf("first var wrong: %+v", got[0])
	}
	if got[1].Key != "TOKEN" || !got[1].Secret || !got[1].Enabled {
		t.Fatalf("secret var wrong: %+v", got[1])
	}
	if got[2].Key != "DISABLED" || got[2].Enabled {
		t.Fatalf("disabled var wrong: %+v", got[2])
	}
}

func TestEnvSummaryMasksSecrets(t *testing.T) {
	env := &store.Environment{
		Name: "Prod",
		Variables: []store.EnvVar{
			{Key: "BASE_URL", Value: "https://api.example.com", Enabled: true},
			{Key: "TOKEN", Value: "secret", Enabled: true, Secret: true},
		},
	}
	got := envSummary(env)
	if want := "TOKEN=****"; !contains(got, want) {
		t.Fatalf("summary %q missing %q", got, want)
	}
	if contains(got, "secret") {
		t.Fatalf("summary leaked secret: %q", got)
	}
}

func TestApplyEnvLetsInlineVarsOverrideDotenv(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("TOKEN=from-file\nBASE_URL=https://file.example\n"), 0o644); err != nil {
		t.Fatalf("write dotenv: %v", err)
	}
	m := Model{}
	m = m.applyEnv(&store.Environment{
		ID:         "env1",
		Name:       "Local",
		DotenvPath: envPath,
		Variables: []store.EnvVar{
			{Key: "TOKEN", Value: "inline", Enabled: true, Secret: true},
		},
	})
	if m.envVars["TOKEN"] != "inline" {
		t.Fatalf("TOKEN: got %q", m.envVars["TOKEN"])
	}
	if !m.envSecrets["TOKEN"] {
		t.Fatal("TOKEN should be tracked as secret")
	}
	if m.envVars["BASE_URL"] != "https://file.example" {
		t.Fatalf("BASE_URL: got %q", m.envVars["BASE_URL"])
	}
}

func TestRequestFromHistoryPreservesReplayFields(t *testing.T) {
	entry := store.HistoryEntry{
		Request: store.HistReq{
			Method:   "POST",
			URL:      "https://example.com/users",
			Headers:  map[string]string{"Content-Type": "application/json"},
			Body:     `{"name":"Ada"}`,
			BodyMode: "graphql",
			Auth:     store.Auth{Type: "basic", User: "u", Pass: "p"},
			Options:  store.RequestOptions{DisableRedirects: true},
			Tests:    "assert status == 201",
		},
	}
	req := requestFromHistory(entry)
	if req.Method != "POST" || req.Body.Mode != "graphql" || req.Auth.User != "u" || !req.Options.DisableRedirects || req.Tests == "" {
		t.Fatalf("request not preserved: %+v", req)
	}
	if len(req.Headers) != 1 || req.Headers[0].Key != "Content-Type" {
		t.Fatalf("headers not preserved: %+v", req.Headers)
	}
}

func TestHistoryLoadActionLoadsEditor(t *testing.T) {
	st, err := store.NewAt(t.TempDir())
	if err != nil {
		t.Fatalf("NewAt: %v", err)
	}
	m := New(st, "test")
	entry := store.HistoryEntry{Request: store.HistReq{Method: "PATCH", URL: "https://example.com/users/1"}}

	next, cmd := m.handleHistoryAction(historyActionMsg{action: "load", entry: entry})
	if cmd != nil {
		t.Fatal("load should not return a command")
	}
	got := next.(Model).editor.BuildRequest()
	if got.Method != "PATCH" || got.URL != "https://example.com/users/1" {
		t.Fatalf("editor request: %+v", got)
	}
}

func TestHistorySaveActionPersistsRequest(t *testing.T) {
	st, err := store.NewAt(t.TempDir())
	if err != nil {
		t.Fatalf("NewAt: %v", err)
	}
	m := New(st, "test")
	entry := store.HistoryEntry{Request: store.HistReq{Method: "GET", URL: "https://example.com/ping"}}

	_, cmd := m.handleHistoryAction(historyActionMsg{action: "save", entry: entry})
	if cmd == nil {
		t.Fatal("save should return a command")
	}
	if msg := cmd(); msg == nil {
		t.Fatal("save command returned nil")
	}
	cols, err := st.LoadCollections()
	if err != nil {
		t.Fatalf("LoadCollections: %v", err)
	}
	if len(cols) != 1 || len(cols[0].Requests) != 1 {
		t.Fatalf("saved history request missing: %+v", cols)
	}
}

func TestEnvPickerDeleteActionRemovesEnvironment(t *testing.T) {
	st, err := store.NewAt(t.TempDir())
	if err != nil {
		t.Fatalf("NewAt: %v", err)
	}
	env := &store.Environment{Name: "Local"}
	if err := st.SaveEnvironment(env); err != nil {
		t.Fatalf("SaveEnvironment: %v", err)
	}
	m := New(st, "test")
	m.environments = []*store.Environment{env}
	m.refreshEnvPicker()
	m.envPickerVisible = true

	_, cmd := m.updateEnvPicker(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd == nil {
		t.Fatal("delete should return a command")
	}
	msg, ok := cmd().(environmentsUpdatedMsg)
	if !ok {
		t.Fatalf("delete returned %T", msg)
	}
	if msg.err != nil {
		t.Fatalf("delete failed: %v", msg.err)
	}
	if len(msg.envs) != 0 {
		t.Fatalf("got %d envs, want 0", len(msg.envs))
	}
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }

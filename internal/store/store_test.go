package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// newTestStore creates a Store backed by a temporary directory that is
// automatically removed when the test finishes.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	for _, sub := range []string{"collections", "environments", "plugins", "themes"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			t.Fatalf("setup store dir: %v", err)
		}
	}
	return &Store{dir: dir}
}

func TestStoreCollectionRoundTrip(t *testing.T) {
	s := newTestStore(t)

	c := &Collection{Name: "Pets", Requests: make(map[string]*Request)}
	if err := s.SaveCollection(c); err != nil {
		t.Fatalf("SaveCollection: %v", err)
	}
	if c.ID == "" {
		t.Fatal("ID should be assigned on first save")
	}

	cols, err := s.LoadCollections()
	if err != nil {
		t.Fatalf("LoadCollections: %v", err)
	}
	if len(cols) != 1 {
		t.Fatalf("got %d collections, want 1", len(cols))
	}
	if cols[0].Name != "Pets" {
		t.Fatalf("name: got %q, want %q", cols[0].Name, "Pets")
	}
}

func TestStoreCollectionsSortedAlphabetically(t *testing.T) {
	s := newTestStore(t)

	for _, name := range []string{"Zebra", "Alpha", "Mango"} {
		if err := s.SaveCollection(&Collection{Name: name, Requests: make(map[string]*Request)}); err != nil {
			t.Fatalf("SaveCollection %q: %v", name, err)
		}
	}

	cols, err := s.LoadCollections()
	if err != nil {
		t.Fatalf("LoadCollections: %v", err)
	}
	names := make([]string, len(cols))
	for i, c := range cols {
		names[i] = c.Name
	}
	want := []string{"Alpha", "Mango", "Zebra"}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("sort: got %v, want %v", names, want)
		}
	}
}

func TestStoreCollectionUpdatePreservesID(t *testing.T) {
	s := newTestStore(t)

	c := &Collection{Name: "First", Requests: make(map[string]*Request)}
	if err := s.SaveCollection(c); err != nil {
		t.Fatalf("SaveCollection: %v", err)
	}
	id := c.ID

	c.Name = "Updated"
	if err := s.SaveCollection(c); err != nil {
		t.Fatalf("SaveCollection update: %v", err)
	}
	if c.ID != id {
		t.Fatalf("ID changed on update: was %q, now %q", id, c.ID)
	}

	cols, _ := s.LoadCollections()
	if len(cols) != 1 || cols[0].Name != "Updated" {
		t.Fatalf("expected 1 updated collection, got %v", cols)
	}
}

func TestStoreDeleteCollection(t *testing.T) {
	s := newTestStore(t)

	c := &Collection{Name: "ToDelete", Requests: make(map[string]*Request)}
	s.SaveCollection(c)

	if err := s.DeleteCollection(c.ID); err != nil {
		t.Fatalf("DeleteCollection: %v", err)
	}
	cols, _ := s.LoadCollections()
	if len(cols) != 0 {
		t.Fatalf("collection not deleted, still have %d", len(cols))
	}
}

func TestStoreDeleteCollectionIdempotent(t *testing.T) {
	s := newTestStore(t)
	// Deleting a non-existent collection should not return an error.
	if err := s.DeleteCollection("nonexistent"); err != nil {
		t.Fatalf("DeleteCollection nonexistent: %v", err)
	}
}

func TestStoreSaveRequest(t *testing.T) {
	s := newTestStore(t)

	col := &Collection{Name: "API", Requests: make(map[string]*Request)}
	s.SaveCollection(col)

	req := &Request{Name: "List Users", Method: "GET", URL: "https://api.example.com/users"}
	if err := s.SaveRequest(col.ID, req); err != nil {
		t.Fatalf("SaveRequest: %v", err)
	}
	if req.ID == "" {
		t.Fatal("request ID should be assigned")
	}

	cols, _ := s.LoadCollections()
	if len(cols) != 1 {
		t.Fatalf("want 1 collection, got %d", len(cols))
	}
	loaded := cols[0].Requests[req.ID]
	if loaded == nil {
		t.Fatal("request not found in collection")
	}
	if loaded.URL != req.URL {
		t.Fatalf("URL: got %q, want %q", loaded.URL, req.URL)
	}
}

func TestStoreSaveRequestPreservesIDOnUpdate(t *testing.T) {
	s := newTestStore(t)

	col := &Collection{Name: "API", Requests: make(map[string]*Request)}
	s.SaveCollection(col)

	req := &Request{Name: "First", Method: "GET", URL: "https://example.com"}
	s.SaveRequest(col.ID, req)
	id := req.ID

	req.Name = "Updated"
	s.SaveRequest(col.ID, req)

	if req.ID != id {
		t.Fatalf("request ID changed: was %q, now %q", id, req.ID)
	}

	cols, _ := s.LoadCollections()
	if len(cols[0].Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(cols[0].Requests))
	}
}

func TestStoreDeleteRequest(t *testing.T) {
	s := newTestStore(t)

	col := &Collection{Name: "API", Requests: make(map[string]*Request)}
	s.SaveCollection(col)

	req := &Request{Name: "Temp", Method: "DELETE", URL: "https://example.com/x"}
	s.SaveRequest(col.ID, req)

	if err := s.DeleteRequest(col.ID, req.ID); err != nil {
		t.Fatalf("DeleteRequest: %v", err)
	}

	cols, _ := s.LoadCollections()
	if _, ok := cols[0].Requests[req.ID]; ok {
		t.Fatal("request still present after delete")
	}
	for _, id := range cols[0].Order {
		if id == req.ID {
			t.Fatal("deleted request ID still in Order slice")
		}
	}
}

func TestStoreEnsureDefaultCollection(t *testing.T) {
	s := newTestStore(t)

	col, err := s.EnsureDefaultCollection()
	if err != nil {
		t.Fatalf("EnsureDefaultCollection: %v", err)
	}
	if col == nil || col.ID == "" {
		t.Fatal("returned nil or empty collection")
	}

	// Calling again should return the existing one, not create a second.
	col2, err := s.EnsureDefaultCollection()
	if err != nil {
		t.Fatalf("EnsureDefaultCollection second call: %v", err)
	}
	if col2.ID != col.ID {
		t.Fatalf("second call returned different collection: %q != %q", col2.ID, col.ID)
	}

	cols, _ := s.LoadCollections()
	if len(cols) != 1 {
		t.Fatalf("expected 1 collection after two EnsureDefault calls, got %d", len(cols))
	}
}

func TestStoreEnvironmentRoundTrip(t *testing.T) {
	s := newTestStore(t)

	env := &Environment{
		Name: "Production",
		Variables: []EnvVar{
			{Key: "BASE_URL", Value: "https://api.example.com", Enabled: true},
			{Key: "TOKEN", Value: "secret", Enabled: true, Secret: true},
		},
	}
	if err := s.SaveEnvironment(env); err != nil {
		t.Fatalf("SaveEnvironment: %v", err)
	}
	if env.ID == "" {
		t.Fatal("ID should be assigned")
	}

	envs, err := s.LoadEnvironments()
	if err != nil {
		t.Fatalf("LoadEnvironments: %v", err)
	}
	if len(envs) != 1 {
		t.Fatalf("got %d environments, want 1", len(envs))
	}
	if envs[0].Name != "Production" {
		t.Fatalf("name: got %q", envs[0].Name)
	}
	if len(envs[0].Variables) != 2 {
		t.Fatalf("variables: got %d, want 2", len(envs[0].Variables))
	}
	if !envs[0].Variables[1].Secret {
		t.Fatal("secret flag not persisted")
	}
}

func TestStoreLoadEnvironmentsEmptyDir(t *testing.T) {
	s := newTestStore(t)
	envs, err := s.LoadEnvironments()
	if err != nil {
		t.Fatalf("LoadEnvironments on empty dir: %v", err)
	}
	if len(envs) != 0 {
		t.Fatalf("expected 0 environments, got %d", len(envs))
	}
}

func TestStoreDeleteEnvironment(t *testing.T) {
	s := newTestStore(t)
	env := &Environment{Name: "Local"}
	if err := s.SaveEnvironment(env); err != nil {
		t.Fatalf("SaveEnvironment: %v", err)
	}

	if err := s.DeleteEnvironment(env.ID); err != nil {
		t.Fatalf("DeleteEnvironment: %v", err)
	}
	envs, err := s.LoadEnvironments()
	if err != nil {
		t.Fatalf("LoadEnvironments: %v", err)
	}
	if len(envs) != 0 {
		t.Fatalf("got %d environments, want 0", len(envs))
	}
}

func TestStoreHistoryRoundTrip(t *testing.T) {
	s := newTestStore(t)

	entry := HistoryEntry{
		Timestamp: time.Now(),
		Request:   HistReq{Method: "GET", URL: "https://example.com"},
		Response:  HistResp{StatusCode: 200, ElapsedMs: 42},
	}
	if err := s.AppendHistory(entry); err != nil {
		t.Fatalf("AppendHistory: %v", err)
	}

	h, err := s.LoadHistory()
	if err != nil {
		t.Fatalf("LoadHistory: %v", err)
	}
	if len(h.Entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(h.Entries))
	}
	if h.Entries[0].Response.StatusCode != 200 {
		t.Fatalf("status: got %d", h.Entries[0].Response.StatusCode)
	}
}

func TestStoreHistoryReplayFieldsRoundTrip(t *testing.T) {
	s := newTestStore(t)

	entry := HistoryEntry{
		Request: HistReq{
			Method:   "POST",
			URL:      "https://example.com/login",
			Headers:  map[string]string{"Content-Type": "application/json"},
			Body:     `{"user":"alice"}`,
			BodyMode: "raw",
			Auth:     Auth{Type: "bearer", Token: "{{TOKEN}}"},
			Options:  RequestOptions{TimeoutSecs: 5},
			Tests:    "assert status == 200",
		},
		Response: HistResp{StatusCode: 200},
	}
	if err := s.AppendHistory(entry); err != nil {
		t.Fatalf("AppendHistory: %v", err)
	}

	h, err := s.LoadHistory()
	if err != nil {
		t.Fatalf("LoadHistory: %v", err)
	}
	got := h.Entries[0].Request
	if got.BodyMode != "raw" || got.Auth.Token != "{{TOKEN}}" || got.Options.TimeoutSecs != 5 || got.Tests == "" {
		t.Fatalf("replay fields not preserved: %+v", got)
	}
}

func TestStoreHistoryOldSchemaStillLoads(t *testing.T) {
	s := newTestStore(t)
	path := filepath.Join(s.Dir(), "history.json")
	data := []byte(`{
  "version": 1,
  "entries": [{
    "request": {"method": "GET", "url": "https://example.com", "headers": {"Accept": "application/json"}, "body": ""},
    "response": {"status_code": 200}
  }]
}`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write history: %v", err)
	}

	h, err := s.LoadHistory()
	if err != nil {
		t.Fatalf("LoadHistory: %v", err)
	}
	if len(h.Entries) != 1 || h.Entries[0].Request.URL != "https://example.com" {
		t.Fatalf("old history schema did not load: %+v", h)
	}
}

func TestStoreHistoryNewestFirst(t *testing.T) {
	s := newTestStore(t)

	for i, url := range []string{"https://first.com", "https://second.com"} {
		_ = s.AppendHistory(HistoryEntry{
			Request:  HistReq{Method: "GET", URL: url},
			Response: HistResp{StatusCode: 200 + i},
		})
	}

	h, _ := s.LoadHistory()
	if len(h.Entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(h.Entries))
	}
	// Most recent append should be first.
	if h.Entries[0].Request.URL != "https://second.com" {
		t.Fatalf("expected newest first, got %q", h.Entries[0].Request.URL)
	}
}

func TestStoreHistoryCappedAt500(t *testing.T) {
	s := newTestStore(t)

	for i := 0; i < 505; i++ {
		_ = s.AppendHistory(HistoryEntry{
			Request:  HistReq{Method: "GET", URL: "https://example.com"},
			Response: HistResp{StatusCode: 200},
		})
	}

	h, err := s.LoadHistory()
	if err != nil {
		t.Fatalf("LoadHistory: %v", err)
	}
	if len(h.Entries) > 500 {
		t.Fatalf("history not capped: got %d entries", len(h.Entries))
	}
}

func TestStoreLoadHistoryMissingFile(t *testing.T) {
	s := newTestStore(t)
	h, err := s.LoadHistory()
	if err != nil {
		t.Fatalf("LoadHistory on missing file: %v", err)
	}
	if h == nil || h.Version != 1 {
		t.Fatal("expected default history struct")
	}
}

func TestStoreConfigDefaults(t *testing.T) {
	s := newTestStore(t)

	cfg, err := s.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.TimeoutSecs != 30 {
		t.Fatalf("default timeout: got %d, want 30", cfg.TimeoutSecs)
	}
}

func TestStoreConfigRoundTrip(t *testing.T) {
	s := newTestStore(t)

	if err := s.SaveConfig(&Config{TimeoutSecs: 60, Theme: "nord"}); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	cfg, err := s.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.TimeoutSecs != 60 {
		t.Fatalf("timeout: got %d, want 60", cfg.TimeoutSecs)
	}
	if cfg.Theme != "nord" {
		t.Fatalf("theme: got %q, want %q", cfg.Theme, "nord")
	}
}

func TestStoreConfigZeroTimeoutFallsBackToDefault(t *testing.T) {
	s := newTestStore(t)
	s.SaveConfig(&Config{TimeoutSecs: 0})

	cfg, _ := s.LoadConfig()
	if cfg.TimeoutSecs != 30 {
		t.Fatalf("zero timeout should default to 30, got %d", cfg.TimeoutSecs)
	}
}

func TestStoreStateRoundTrip(t *testing.T) {
	s := newTestStore(t)

	st := &AppState{ActiveCollectionID: "col1", ActiveEnvID: "env1", SeenWelcome: true}
	if err := s.SaveState(st); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	loaded, err := s.LoadState()
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if loaded.ActiveCollectionID != "col1" {
		t.Fatalf("ActiveCollectionID: got %q", loaded.ActiveCollectionID)
	}
	if !loaded.SeenWelcome {
		t.Fatal("SeenWelcome not persisted")
	}
}

func TestStoreStateMissingFileReturnsDefault(t *testing.T) {
	s := newTestStore(t)
	st, err := s.LoadState()
	if err != nil {
		t.Fatalf("LoadState on missing file: %v", err)
	}
	if st == nil || st.Version != 1 {
		t.Fatal("expected default state")
	}
}

func TestWriteAtomicLeavesNoTempFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.json")

	if err := writeAtomic(path, []byte(`{}`)); err != nil {
		t.Fatalf("writeAtomic: %v", err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("temp file not cleaned up")
	}
}

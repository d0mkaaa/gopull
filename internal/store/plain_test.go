package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPlainCollectionRoundTrip(t *testing.T) {
	c := &Collection{
		Name: "Demo API",
		Requests: map[string]*Request{
			"list": {
				Name:   "List users",
				Method: "GET",
				URL:    "https://example.com/users/:id",
				Query:  []Param{{Key: "page", Value: "1", Enabled: true}},
				Path:   []Param{{Key: "id", Value: "42", Enabled: true}},
				Headers: []Header{
					{Key: "Accept", Value: "application/json", Enabled: true},
				},
				Body: Body{Mode: "raw", Raw: `{"ok":true}`},
				Options: RequestOptions{
					UseCookieJar:   true,
					ClientCertPath: "/tmp/client.pem",
				},
				Tests: "assert status == 200",
			},
		},
		Order: []string{"list"},
	}
	dir := filepath.Join(t.TempDir(), "demo")
	if err := ExportPlainCollection(c, dir); err != nil {
		t.Fatal(err)
	}
	got, err := LoadPlainCollection(dir)
	if err != nil {
		t.Fatal(err)
	}
	r := got.Requests["list-users"]
	if r == nil {
		t.Fatalf("request not loaded: %#v", got.Requests)
	}
	if r.Query[0].Value != "1" || r.Path[0].Value != "42" {
		t.Fatalf("params not preserved: %#v %#v", r.Query, r.Path)
	}
	if !r.Options.UseCookieJar || r.Options.ClientCertPath != "/tmp/client.pem" {
		t.Fatalf("options not preserved: %#v", r.Options)
	}
	if r.Tests != "assert status == 200" {
		t.Fatalf("tests: got %q", r.Tests)
	}
}

func TestPlainRequestKeepsBodyModeWhenTestsFollow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "upload.gopull")
	data := "name: Upload\nmethod: POST\nurl: https://example.com\n[body multipart]\nname=alice\n[tests]\nassert status == 200\n"
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := LoadCollectionPath(path)
	if err != nil {
		t.Fatal(err)
	}
	r := c.Requests[c.Order[0]]
	if r.Body.Mode != "multipart" {
		t.Fatalf("body mode: got %q", r.Body.Mode)
	}
	if r.Tests != "assert status == 200" {
		t.Fatalf("tests: got %q", r.Tests)
	}
}

func TestExportPlainCollectionDoesNotOverwriteDuplicateNames(t *testing.T) {
	c := &Collection{
		Name: "Demo",
		Requests: map[string]*Request{
			"a": {Name: "Same", Method: "GET", URL: "https://a.example"},
			"b": {Name: "Same", Method: "GET", URL: "https://b.example"},
		},
		Order: []string{"a", "b"},
	}
	dir := t.TempDir()
	if err := ExportPlainCollection(c, dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "same.gopull")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "same-2.gopull")); err != nil {
		t.Fatal(err)
	}
}

func TestLoadCollectionPathReadsPlainDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, PlainCollectionFile), []byte("name: Plain\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ping.gopull"), []byte("method: GET\nurl: https://example.com\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := LoadCollectionPath(dir)
	if err != nil {
		t.Fatal(err)
	}
	if c.Name != "Plain" || len(c.Order) != 1 {
		t.Fatalf("collection: %#v", c)
	}
}

func TestPlainCollectionPreservesMetaOrder(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, PlainCollectionFile), []byte("name: Ordered\norder: second, first\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "first.gopull"), []byte("name: First\nmethod: GET\nurl: https://example.com/1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "second.gopull"), []byte("name: Second\nmethod: GET\nurl: https://example.com/2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := LoadPlainCollection(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Order; len(got) != 2 || got[0] != "second" || got[1] != "first" {
		t.Fatalf("order = %#v", got)
	}
}

func TestWorkspaceStoreSavesRequestToPlainFile(t *testing.T) {
	dir := t.TempDir()
	if err := InitPlainWorkspace(dir, "Demo", false, false); err != nil {
		t.Fatal(err)
	}
	st := &Store{dir: t.TempDir(), workspaceDir: dir}
	req := &Request{ID: "nested/list-users", Name: "List users", Method: "GET", URL: "https://example.com/users"}
	if err := st.SaveRequest("demo", req); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "nested", "list-users.gopull")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("saved request missing: %v", err)
	}
	c, err := st.LoadCollections()
	if err != nil {
		t.Fatal(err)
	}
	if c[0].Requests["nested/list-users"] == nil {
		t.Fatalf("request not loaded from workspace: %#v", c[0].Requests)
	}
}

func TestWorkspaceRenameKeepsSlugStable(t *testing.T) {
	dir := t.TempDir()
	if err := InitPlainWorkspace(dir, "Demo", false, false); err != nil {
		t.Fatal(err)
	}
	st := &Store{dir: t.TempDir(), workspaceDir: dir}

	// Save a request — slug comes from the name.
	req := &Request{Name: "List users", Method: "GET", URL: "https://example.com/users"}
	if err := st.SaveRequest("demo", req); err != nil {
		t.Fatal(err)
	}

	// Load to discover the assigned ID/slug.
	cols, err := st.LoadCollections()
	if err != nil {
		t.Fatal(err)
	}
	c := cols[0]
	var reqID string
	for id := range c.Requests {
		if c.Requests[id].Name == "List users" {
			reqID = id
			break
		}
	}
	if reqID == "" {
		t.Fatal("saved request not found in collection")
	}

	// Rename via store.
	if err := st.RenameRequest(c.ID, reqID, "List all users"); err != nil {
		t.Fatalf("rename: %v", err)
	}

	// File at original slug must still exist (slug-stable).
	slug := reqID
	if idx := len(filepath.FromSlash(slug)); idx > 0 {
		slug = filepath.FromSlash(reqID)
	}
	if _, err := os.Stat(filepath.Join(dir, slug+".gopull")); err != nil {
		t.Fatalf("original slug file gone after rename: %v", err)
	}

	// Reload — request name must be updated, ID must be unchanged.
	cols2, err := st.LoadCollections()
	if err != nil {
		t.Fatal(err)
	}
	r2 := cols2[0].Requests[reqID]
	if r2 == nil {
		t.Fatalf("request lost after rename: %#v", cols2[0].Requests)
	}
	if r2.Name != "List all users" {
		t.Fatalf("name not updated after rename: got %q", r2.Name)
	}

	// No orphan files: only one .gopull file should exist (plus example.gopull from init).
	var gopullFiles []string
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, _ error) error {
		if !d.IsDir() && filepath.Ext(path) == ".gopull" {
			gopullFiles = append(gopullFiles, path)
		}
		return nil
	})
	if len(gopullFiles) != 2 { // example.gopull + our request
		t.Fatalf("expected 2 .gopull files, got %d: %v", len(gopullFiles), gopullFiles)
	}
}

func TestExportThenWorkspaceSaveRoundTrip(t *testing.T) {
	// Export a collection to a directory, then open as workspace, save a new
	// request, and reload — verifying IDs and filenames stay consistent.
	c := &Collection{
		Name: "API",
		Requests: map[string]*Request{
			"hex123abc": {Name: "Get item", Method: "GET", URL: "https://example.com/item"},
		},
		Order: []string{"hex123abc"},
	}
	dir := filepath.Join(t.TempDir(), "ws")
	if err := ExportPlainCollection(c, dir); err != nil {
		t.Fatal(err)
	}

	// After export the filename is slug-based (from name), not hex.
	if _, err := os.Stat(filepath.Join(dir, "get-item.gopull")); err != nil {
		t.Fatalf("expected slug-based filename after export: %v", err)
	}

	// Open as workspace and save a new request.
	st := &Store{dir: t.TempDir(), workspaceDir: dir}
	req := &Request{Name: "Post item", Method: "POST", URL: "https://example.com/item"}
	if err := st.SaveRequest("api", req); err != nil {
		t.Fatalf("save new request: %v", err)
	}

	// Reload — both requests should be present with slug IDs.
	cols, err := st.LoadCollections()
	if err != nil {
		t.Fatal(err)
	}
	if len(cols[0].Requests) != 2 {
		t.Fatalf("expected 2 requests, got %d: %#v", len(cols[0].Requests), cols[0].Requests)
	}
	// Original request now has a slug ID (from filename), not hex.
	if cols[0].Requests["get-item"] == nil {
		t.Fatalf("original request not found by slug ID: %#v", cols[0].Requests)
	}
}

func TestInitPlainWorkspaceWritesScaffold(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "api")
	if err := InitPlainWorkspace(dir, "API", true, false); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{PlainCollectionFile, "example.gopull", ".gitignore", ".env.example"} {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Fatalf("%s missing: %v", rel, err)
		}
	}
}

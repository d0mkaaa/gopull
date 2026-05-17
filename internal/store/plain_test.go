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

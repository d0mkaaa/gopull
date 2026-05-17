package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/d0mkaaa/gopull/internal/store"
)

func TestBuildClientRequestEncodesFormBody(t *testing.T) {
	req := store.Request{
		Method: "POST",
		URL:    "https://example.com/login",
		Body: store.Body{
			Mode: "form",
			Raw:  "user: alice\npassword: secret\n# ignored: yes",
		},
	}
	got, err := BuildClientRequest(req, nil)
	if err != nil {
		t.Fatal(err)
	}

	if got.Body != "password=secret&user=alice" {
		t.Fatalf("body: got %q", got.Body)
	}
	if got.Headers["Content-Type"] != "application/x-www-form-urlencoded" {
		t.Fatalf("content type: got %q", got.Headers["Content-Type"])
	}
}

func TestBuildClientRequestDefaultsGraphQLContentType(t *testing.T) {
	req := store.Request{
		Method: "POST",
		URL:    "https://example.com/graphql",
		Body:   store.Body{Mode: "graphql", Raw: `{"query":"{ viewer { id } }"}`},
	}
	got, err := BuildClientRequest(req, nil)
	if err != nil {
		t.Fatal(err)
	}

	if got.Body != req.Body.Raw {
		t.Fatalf("body: got %q", got.Body)
	}
	if got.Headers["Content-Type"] != "application/json" {
		t.Fatalf("content type: got %q", got.Headers["Content-Type"])
	}
}

func TestBuildClientRequestPreservesExplicitGraphQLContentType(t *testing.T) {
	req := store.Request{
		Method:  "POST",
		URL:     "https://example.com/graphql",
		Headers: []store.Header{{Key: "Content-Type", Value: "application/graphql", Enabled: true}},
		Body:    store.Body{Mode: "graphql", Raw: "query { viewer { id } }"},
	}
	got, err := BuildClientRequest(req, nil)
	if err != nil {
		t.Fatal(err)
	}

	if got.Headers["Content-Type"] != "application/graphql" {
		t.Fatalf("content type: got %q", got.Headers["Content-Type"])
	}
}

func TestBuildClientRequestAppliesQueryAndPathParams(t *testing.T) {
	req := store.Request{
		Method: "GET",
		URL:    "https://example.com/users/:id?keep=1",
		Path:   []store.Param{{Key: "id", Value: "a b", Enabled: true}},
		Query:  []store.Param{{Key: "page", Value: "2", Enabled: true}},
	}
	got, err := BuildClientRequest(req, nil)
	if err != nil {
		t.Fatal(err)
	}

	if got.URL != "https://example.com/users/a%20b?keep=1&page=2" {
		t.Fatalf("url: got %q", got.URL)
	}
}

func TestBuildClientRequestSubstitutesParamsBeforeEscaping(t *testing.T) {
	req := store.Request{
		Method: "GET",
		URL:    "https://example.com/users/:id",
		Path:   []store.Param{{Key: "id", Value: "{{USER_ID}}", Enabled: true}},
		Query:  []store.Param{{Key: "q", Value: "{{QUERY}}", Enabled: true}},
	}
	got, err := BuildClientRequest(req, map[string]string{"USER_ID": "a b", "QUERY": "one two"})
	if err != nil {
		t.Fatal(err)
	}
	if got.URL != "https://example.com/users/a%20b?q=one+two" {
		t.Fatalf("url: got %q", got.URL)
	}
}

func TestBuildClientRequestReadsFileBody(t *testing.T) {
	path := filepath.Join(t.TempDir(), "body.json")
	if err := os.WriteFile(path, []byte(`{"ok":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	req := store.Request{Method: "POST", URL: "https://example.com", Body: store.Body{Mode: "file", Raw: path}}
	got, err := BuildClientRequest(req, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Body != `{"ok":true}` || got.Headers["Content-Type"] != "application/json" {
		t.Fatalf("file body: %#v", got)
	}
}

func TestBuildClientRequestBuildsMultipartBody(t *testing.T) {
	path := filepath.Join(t.TempDir(), "avatar.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	req := store.Request{
		Method: "POST",
		URL:    "https://example.com",
		Body:   store.Body{Mode: "multipart", Raw: "name=alice\nfile avatar=" + path},
	}
	got, err := BuildClientRequest(req, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got.Headers["Content-Type"], "multipart/form-data; boundary=") {
		t.Fatalf("content type: %q", got.Headers["Content-Type"])
	}
	if !strings.Contains(got.Body, `name="name"`) || !strings.Contains(got.Body, `name="avatar"; filename="avatar.txt"`) {
		t.Fatalf("body: %q", got.Body)
	}
}

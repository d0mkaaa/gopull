package core

import (
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
	got := BuildClientRequest(req, nil)

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
	got := BuildClientRequest(req, nil)

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
	got := BuildClientRequest(req, nil)

	if got.Headers["Content-Type"] != "application/graphql" {
		t.Fatalf("content type: got %q", got.Headers["Content-Type"])
	}
}

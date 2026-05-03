package store

import (
	"strings"
	"testing"
)

func TestImportHTTPFile_Basic(t *testing.T) {
	input := `### Get user
GET https://api.example.com/users/1
Authorization: Bearer {{TOKEN}}
Accept: application/json
`
	col, err := ImportHTTPFile([]byte(input))
	if err != nil {
		t.Fatalf("ImportHTTPFile: %v", err)
	}
	if len(col.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(col.Requests))
	}
	var r *Request
	for _, v := range col.Requests {
		r = v
	}
	if r.Name != "Get user" {
		t.Errorf("name: got %q", r.Name)
	}
	if r.Method != "GET" {
		t.Errorf("method: got %q", r.Method)
	}
	if r.URL != "https://api.example.com/users/1" {
		t.Errorf("url: got %q", r.URL)
	}
	if len(r.Headers) != 2 {
		t.Errorf("headers: got %d", len(r.Headers))
	}
}

func TestImportHTTPFile_MultipleRequests(t *testing.T) {
	input := `### List items
GET https://api.example.com/items

### Create item
POST https://api.example.com/items
Content-Type: application/json

{"name": "widget"}

### Delete item
DELETE https://api.example.com/items/1
`
	col, err := ImportHTTPFile([]byte(input))
	if err != nil {
		t.Fatalf("ImportHTTPFile: %v", err)
	}
	if len(col.Requests) != 3 {
		t.Fatalf("expected 3 requests, got %d", len(col.Requests))
	}
	if len(col.Order) != 3 {
		t.Errorf("order length: got %d", len(col.Order))
	}
}

func TestImportHTTPFile_Body(t *testing.T) {
	input := `### Create user
POST https://api.example.com/users
Content-Type: application/json

{"name": "Alice", "email": "alice@example.com"}
`
	col, err := ImportHTTPFile([]byte(input))
	if err != nil {
		t.Fatalf("ImportHTTPFile: %v", err)
	}
	var r *Request
	for _, v := range col.Requests {
		r = v
	}
	if r.Method != "POST" {
		t.Errorf("method: got %q", r.Method)
	}
	if !strings.Contains(r.Body.Raw, "Alice") {
		t.Errorf("body: got %q", r.Body.Raw)
	}
}

func TestImportHTTPFile_NoSeparator(t *testing.T) {
	// File without ### should still parse as one request.
	input := `GET https://api.example.com/health
Accept: application/json
`
	col, err := ImportHTTPFile([]byte(input))
	if err != nil {
		t.Fatalf("ImportHTTPFile: %v", err)
	}
	if len(col.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(col.Requests))
	}
}

func TestImportHTTPFile_DefaultNameFromPath(t *testing.T) {
	// No name on ### line -> default name from method + path.
	input := `###
GET https://api.example.com/users/42
`
	col, err := ImportHTTPFile([]byte(input))
	if err != nil {
		t.Fatalf("ImportHTTPFile: %v", err)
	}
	var r *Request
	for _, v := range col.Requests {
		r = v
	}
	if r.Name != "GET /users/42" {
		t.Errorf("name: got %q", r.Name)
	}
}

func TestImportHTTPFile_BareURL(t *testing.T) {
	// URL without explicit method should default to GET.
	input := `### Health check
https://api.example.com/health
`
	col, err := ImportHTTPFile([]byte(input))
	if err != nil {
		t.Fatalf("ImportHTTPFile: %v", err)
	}
	var r *Request
	for _, v := range col.Requests {
		r = v
	}
	if r.Method != "GET" {
		t.Errorf("method: got %q, want GET", r.Method)
	}
}

func TestImportHTTPFile_Empty(t *testing.T) {
	_, err := ImportHTTPFile([]byte("# just a comment\n"))
	if err == nil {
		t.Error("expected error for file with no valid requests")
	}
}

func TestExportHTTPFile_RoundTrip(t *testing.T) {
	input := `### Get user
GET https://api.example.com/users/1
Authorization: Bearer {{TOKEN}}

### Create user
POST https://api.example.com/users
Content-Type: application/json

{"name": "Alice"}
`
	col, err := ImportHTTPFile([]byte(input))
	if err != nil {
		t.Fatalf("ImportHTTPFile: %v", err)
	}

	exported, err := ExportHTTPFile(col)
	if err != nil {
		t.Fatalf("ExportHTTPFile: %v", err)
	}

	// Re-import the exported bytes and verify we still get 2 requests.
	col2, err := ImportHTTPFile(exported)
	if err != nil {
		t.Fatalf("re-import: %v", err)
	}
	if len(col2.Requests) != 2 {
		t.Errorf("round-trip requests: got %d, want 2", len(col2.Requests))
	}
}

func TestExportHTTPFile_IncludesBody(t *testing.T) {
	col := &Collection{
		Name:     "Test",
		Requests: make(map[string]*Request),
	}
	r := &Request{
		ID:     "r1",
		Name:   "Create",
		Method: "POST",
		URL:    "https://example.com/items",
		Headers: []Header{
			{Key: "Content-Type", Value: "application/json", Enabled: true},
		},
		Body: Body{Mode: "raw", Raw: `{"name":"widget"}`},
	}
	col.Requests["r1"] = r
	col.Order = []string{"r1"}

	data, err := ExportHTTPFile(col)
	if err != nil {
		t.Fatalf("ExportHTTPFile: %v", err)
	}
	out := string(data)
	if !strings.Contains(out, "### Create") {
		t.Errorf("missing ### separator: %s", out)
	}
	if !strings.Contains(out, "POST https://example.com/items") {
		t.Errorf("missing method line: %s", out)
	}
	if !strings.Contains(out, "Content-Type: application/json") {
		t.Errorf("missing header: %s", out)
	}
	if !strings.Contains(out, `{"name":"widget"}`) {
		t.Errorf("missing body: %s", out)
	}
}

func TestLooksLikeHTTPFile(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		{"### Get\nGET https://x.com\n", true},
		{"GET https://x.com\n", true},
		{"POST https://x.com\n", true},
		{`{"info":{"name":"col"}}`, false},
		{"", false},
		{"# comment only", false},
	}
	for _, c := range cases {
		got := LooksLikeHTTPFile([]byte(c.input))
		if got != c.want {
			t.Errorf("LooksLikeHTTPFile(%q) = %v, want %v", c.input, got, c.want)
		}
	}
}

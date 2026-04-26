package store

import (
	"strings"
	"testing"
)

var v3Spec = []byte(`{
  "openapi": "3.0.0",
  "info": {"title": "Widget API", "version": "1.0.0"},
  "servers": [{"url": "https://api.example.com/v1"}],
  "paths": {
    "/widgets": {
      "get": {
        "summary": "List widgets",
        "tags": ["widgets"]
      },
      "post": {
        "summary": "Create widget",
        "requestBody": {
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "required": ["name"],
                "properties": {
                  "name": {"type": "string"},
                  "count": {"type": "integer"}
                }
              }
            }
          }
        }
      }
    },
    "/widgets/{id}": {
      "get": {"summary": "Get widget"},
      "delete": {"summary": "Delete widget"}
    }
  }
}`)

var v2Spec = []byte(`{
  "swagger": "2.0",
  "info": {"title": "Pet Store"},
  "host": "petstore.example.com",
  "basePath": "/api",
  "schemes": ["https"],
  "paths": {
    "/pets": {
      "get": {"summary": "List pets", "operationId": "listPets"},
      "post": {
        "summary": "Create pet",
        "consumes": ["application/json"],
        "parameters": [{"in": "body", "name": "body", "schema": {
          "type": "object",
          "properties": {
            "name": {"type": "string"}
          }
        }}]
      }
    }
  }
}`)

func TestImportOpenAPIv3(t *testing.T) {
	col, err := ImportOpenAPI(v3Spec)
	if err != nil {
		t.Fatalf("ImportOpenAPI v3: %v", err)
	}
	if col.Name != "Widget API" {
		t.Errorf("name: got %q", col.Name)
	}
	if len(col.Requests) != 4 {
		t.Errorf("requests: got %d, want 4", len(col.Requests))
	}
}

func TestImportOpenAPIv3_URLs(t *testing.T) {
	col, err := ImportOpenAPI(v3Spec)
	if err != nil {
		t.Fatalf("ImportOpenAPI: %v", err)
	}
	for _, r := range col.Requests {
		if !strings.HasPrefix(r.URL, "https://api.example.com/v1") {
			t.Errorf("URL %q does not start with base URL", r.URL)
		}
	}
}

func TestImportOpenAPIv3_PostBody(t *testing.T) {
	col, err := ImportOpenAPI(v3Spec)
	if err != nil {
		t.Fatalf("ImportOpenAPI: %v", err)
	}
	var post *Request
	for _, r := range col.Requests {
		if r.Method == "POST" && strings.HasSuffix(r.URL, "/widgets") {
			post = r
			break
		}
	}
	if post == nil {
		t.Fatal("POST /widgets not found")
	}
	if post.Body.Raw == "" {
		t.Error("expected body template for POST with JSON schema")
	}
	// Should have Content-Type header.
	var hasCT bool
	for _, h := range post.Headers {
		if h.Key == "Content-Type" && strings.Contains(h.Value, "json") {
			hasCT = true
		}
	}
	if !hasCT {
		t.Error("expected Content-Type: application/json header")
	}
}

func TestImportOpenAPIv2(t *testing.T) {
	col, err := ImportOpenAPI(v2Spec)
	if err != nil {
		t.Fatalf("ImportOpenAPI v2: %v", err)
	}
	if col.Name != "Pet Store" {
		t.Errorf("name: got %q", col.Name)
	}
	if len(col.Requests) != 2 {
		t.Errorf("requests: got %d, want 2", len(col.Requests))
	}
	for _, r := range col.Requests {
		if !strings.HasPrefix(r.URL, "https://petstore.example.com/api") {
			t.Errorf("URL %q missing base", r.URL)
		}
	}
}

func TestImportOpenAPIv2_Names(t *testing.T) {
	col, err := ImportOpenAPI(v2Spec)
	if err != nil {
		t.Fatalf("ImportOpenAPI: %v", err)
	}
	names := make(map[string]bool)
	for _, r := range col.Requests {
		names[r.Name] = true
	}
	if !names["listPets"] && !names["List pets"] {
		t.Errorf("expected operationId or summary as name, got %v", names)
	}
}

func TestImportOpenAPI_InvalidJSON(t *testing.T) {
	_, err := ImportOpenAPI([]byte(`not json`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestImportOpenAPI_NotOpenAPI(t *testing.T) {
	_, err := ImportOpenAPI([]byte(`{"info":{"name":"col"},"item":[]}`))
	if err == nil {
		t.Error("expected error for non-OpenAPI JSON")
	}
}

func TestLooksLikeOpenAPI(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		{`{"openapi":"3.0.0","info":{}}`, true},
		{`{"swagger":"2.0","info":{}}`, true},
		{`{"info":{"name":"postman"}}`, false},
		{`### GET https://x.com`, false},
		{``, false},
	}
	for _, c := range cases {
		got := LooksLikeOpenAPI([]byte(c.input))
		if got != c.want {
			t.Errorf("LooksLikeOpenAPI(%q) = %v, want %v", c.input, got, c.want)
		}
	}
}

func TestImportOpenAPI_PathParamsPreserved(t *testing.T) {
	col, err := ImportOpenAPI(v3Spec)
	if err != nil {
		t.Fatalf("ImportOpenAPI: %v", err)
	}
	var found bool
	for _, r := range col.Requests {
		if strings.Contains(r.URL, "{id}") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected {id} path param to be preserved in URL")
	}
}

func TestImportOpenAPI_DeterministicOrder(t *testing.T) {
	col1, err := ImportOpenAPI(v3Spec)
	if err != nil {
		t.Fatalf("first import: %v", err)
	}
	col2, err := ImportOpenAPI(v3Spec)
	if err != nil {
		t.Fatalf("second import: %v", err)
	}
	if len(col1.Order) != len(col2.Order) {
		t.Fatalf("order lengths differ: %d vs %d", len(col1.Order), len(col2.Order))
	}
	// Verify methods match in the same order.
	for i := range col1.Order {
		r1 := col1.Requests[col1.Order[i]]
		r2 := col2.Requests[col2.Order[i]]
		if r1.Method != r2.Method || r1.URL != r2.URL {
			t.Errorf("order[%d] differs: %s %s vs %s %s",
				i, r1.Method, r1.URL, r2.Method, r2.URL)
		}
	}
}

package store

import (
	"encoding/json"
	"strings"
	"testing"
)

var minimalPostman = []byte(`{
  "info": {"name": "Pet Store", "schema": "https://schema.getpostman.com/json/collection/v2.1.0/collection.json"},
  "item": [
    {
      "name": "List pets",
      "request": {
        "method": "GET",
        "url": {"raw": "https://api.example.com/pets"},
        "header": [
          {"key": "Accept", "value": "application/json", "disabled": false}
        ]
      }
    },
    {
      "name": "Create pet",
      "request": {
        "method": "POST",
        "url": {"raw": "https://api.example.com/pets"},
        "header": [{"key": "Content-Type", "value": "application/json", "disabled": false}],
        "body": {"mode": "raw", "raw": "{\"name\":\"Fido\"}"}
      }
    }
  ]
}`)

func TestImportPostmanCollectionName(t *testing.T) {
	c, err := ImportPostman(minimalPostman)
	if err != nil {
		t.Fatalf("ImportPostman: %v", err)
	}
	if c.Name != "Pet Store" {
		t.Errorf("name: got %q, want %q", c.Name, "Pet Store")
	}
}

func TestImportPostmanRequestCount(t *testing.T) {
	c, err := ImportPostman(minimalPostman)
	if err != nil {
		t.Fatalf("ImportPostman: %v", err)
	}
	if len(c.Requests) != 2 {
		t.Errorf("requests: got %d, want 2", len(c.Requests))
	}
	if len(c.Order) != 2 {
		t.Errorf("order: got %d, want 2", len(c.Order))
	}
}

func TestImportPostmanRequestFields(t *testing.T) {
	c, err := ImportPostman(minimalPostman)
	if err != nil {
		t.Fatalf("ImportPostman: %v", err)
	}

	// Find the "Create pet" request.
	var create *Request
	for _, r := range c.Requests {
		if r.Name == "Create pet" {
			create = r
			break
		}
	}
	if create == nil {
		t.Fatal("'Create pet' request not found")
	}
	if create.Method != "POST" {
		t.Errorf("method: got %q", create.Method)
	}
	if create.URL != "https://api.example.com/pets" {
		t.Errorf("url: got %q", create.URL)
	}
	if !strings.Contains(create.Body.Raw, "Fido") {
		t.Errorf("body: got %q", create.Body.Raw)
	}
}

func TestImportPostmanHeadersConvertEnabled(t *testing.T) {
	c, err := ImportPostman(minimalPostman)
	if err != nil {
		t.Fatalf("ImportPostman: %v", err)
	}
	var list *Request
	for _, r := range c.Requests {
		if r.Name == "List pets" {
			list = r
			break
		}
	}
	if list == nil {
		t.Fatal("'List pets' not found")
	}
	if len(list.Headers) != 1 {
		t.Fatalf("headers: got %d, want 1", len(list.Headers))
	}
	if list.Headers[0].Key != "Accept" {
		t.Errorf("header key: got %q", list.Headers[0].Key)
	}
	if !list.Headers[0].Enabled {
		t.Error("header should be enabled (disabled=false)")
	}
}

func TestImportPostmanDisabledHeaderNotEnabled(t *testing.T) {
	data := []byte(`{
		"info": {"name": "T"},
		"item": [{
			"name": "R",
			"request": {
				"method": "GET",
				"url": {"raw": "https://x.com"},
				"header": [{"key": "X-Old", "value": "v", "disabled": true}]
			}
		}]
	}`)
	c, err := ImportPostman(data)
	if err != nil {
		t.Fatalf("ImportPostman: %v", err)
	}
	for _, r := range c.Requests {
		if len(r.Headers) != 1 || r.Headers[0].Enabled {
			t.Error("disabled header should have Enabled=false")
		}
	}
}

func TestImportPostmanBearerAuth(t *testing.T) {
	data := []byte(`{
		"info": {"name": "Authed"},
		"item": [{
			"name": "Secure",
			"request": {
				"method": "GET",
				"url": {"raw": "https://api.example.com/secure"},
				"auth": {
					"type": "bearer",
					"bearer": [{"key": "token", "value": "my-jwt"}]
				}
			}
		}]
	}`)
	c, err := ImportPostman(data)
	if err != nil {
		t.Fatalf("ImportPostman: %v", err)
	}
	for _, r := range c.Requests {
		if r.Auth.Type != "bearer" {
			t.Errorf("auth type: got %q", r.Auth.Type)
		}
		if r.Auth.Token != "my-jwt" {
			t.Errorf("token: got %q", r.Auth.Token)
		}
	}
}

func TestImportPostmanBasicAuth(t *testing.T) {
	data := []byte(`{
		"info": {"name": "Authed"},
		"item": [{
			"name": "Basic",
			"request": {
				"method": "GET",
				"url": {"raw": "https://api.example.com/"},
				"auth": {
					"type": "basic",
					"basic": [
						{"key": "username", "value": "alice"},
						{"key": "password", "value": "s3cr3t"}
					]
				}
			}
		}]
	}`)
	c, err := ImportPostman(data)
	if err != nil {
		t.Fatalf("ImportPostman: %v", err)
	}
	for _, r := range c.Requests {
		if r.Auth.Type != "basic" {
			t.Errorf("auth type: got %q", r.Auth.Type)
		}
		if r.Auth.User != "alice" || r.Auth.Pass != "s3cr3t" {
			t.Errorf("credentials: user=%q pass=%q", r.Auth.User, r.Auth.Pass)
		}
	}
}

func TestImportPostmanFlattensFolders(t *testing.T) {
	data := []byte(`{
		"info": {"name": "Nested"},
		"item": [
			{
				"name": "Folder A",
				"item": [
					{"name": "req-1", "request": {"method": "GET", "url": {"raw": "https://a.com"}}},
					{"name": "req-2", "request": {"method": "POST", "url": {"raw": "https://b.com"}}}
				]
			},
			{"name": "req-3", "request": {"method": "DELETE", "url": {"raw": "https://c.com"}}}
		]
	}`)
	c, err := ImportPostman(data)
	if err != nil {
		t.Fatalf("ImportPostman: %v", err)
	}
	if len(c.Requests) != 3 {
		t.Errorf("expected 3 flattened requests, got %d", len(c.Requests))
	}
}

func TestImportPostmanInvalidJSON(t *testing.T) {
	_, err := ImportPostman([]byte(`not json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestImportPostmanAssignsIDs(t *testing.T) {
	c, err := ImportPostman(minimalPostman)
	if err != nil {
		t.Fatalf("ImportPostman: %v", err)
	}
	if c.ID == "" {
		t.Error("collection should have an ID")
	}
	for _, r := range c.Requests {
		if r.ID == "" {
			t.Errorf("request %q has no ID", r.Name)
		}
	}
}

func TestExportPostmanRoundTrip(t *testing.T) {
	c, err := ImportPostman(minimalPostman)
	if err != nil {
		t.Fatalf("ImportPostman: %v", err)
	}

	data, err := ExportPostman(c)
	if err != nil {
		t.Fatalf("ExportPostman: %v", err)
	}

	// Re-import and check names survive.
	c2, err := ImportPostman(data)
	if err != nil {
		t.Fatalf("re-import: %v", err)
	}
	if c2.Name != c.Name {
		t.Errorf("name: got %q, want %q", c2.Name, c.Name)
	}
	if len(c2.Requests) != len(c.Requests) {
		t.Errorf("requests: got %d, want %d", len(c2.Requests), len(c.Requests))
	}
}

func TestExportPostmanSetsSchema(t *testing.T) {
	c := &Collection{
		Name:     "Test",
		Requests: make(map[string]*Request),
		Order:    []string{},
	}
	data, err := ExportPostman(c)
	if err != nil {
		t.Fatalf("ExportPostman: %v", err)
	}
	var pm map[string]interface{}
	if err := json.Unmarshal(data, &pm); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	info, _ := pm["info"].(map[string]interface{})
	if info == nil {
		t.Fatal("missing 'info' field")
	}
	schema, _ := info["schema"].(string)
	if !strings.Contains(schema, "v2.1") {
		t.Errorf("schema should reference v2.1, got %q", schema)
	}
}

func TestExportPostmanPreservesHeaders(t *testing.T) {
	c := &Collection{
		Name: "H",
		Requests: map[string]*Request{
			"r1": {
				ID:     "r1",
				Name:   "R",
				Method: "GET",
				URL:    "https://example.com",
				Headers: []Header{
					{Key: "X-Custom", Value: "hello", Enabled: true},
					{Key: "X-Disabled", Value: "bye", Enabled: false},
				},
			},
		},
		Order: []string{"r1"},
	}
	data, err := ExportPostman(c)
	if err != nil {
		t.Fatalf("ExportPostman: %v", err)
	}
	if !strings.Contains(string(data), "X-Custom") {
		t.Error("X-Custom header not in export")
	}
}

func TestExportPostmanPreservesAuth(t *testing.T) {
	c := &Collection{
		Name: "A",
		Requests: map[string]*Request{
			"r1": {
				ID:     "r1",
				Name:   "Secure",
				Method: "GET",
				URL:    "https://example.com",
				Auth:   Auth{Type: "bearer", Token: "tok123"},
			},
		},
		Order: []string{"r1"},
	}
	data, err := ExportPostman(c)
	if err != nil {
		t.Fatalf("ExportPostman: %v", err)
	}
	if !strings.Contains(string(data), "tok123") {
		t.Error("bearer token not in export")
	}
	if !strings.Contains(string(data), "bearer") {
		t.Error("auth type not in export")
	}
}

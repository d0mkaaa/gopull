package store

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// ImportOpenAPI parses an OpenAPI v2 (Swagger) or v3 JSON spec into a Collection.
// YAML specs must be converted to JSON first.
func ImportOpenAPI(data []byte) (*Collection, error) {
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse OpenAPI JSON: %w", err)
	}
	if _, ok := raw["swagger"]; ok {
		return importOpenAPIv2(raw)
	}
	if _, ok := raw["openapi"]; ok {
		return importOpenAPIv3(raw)
	}
	return nil, fmt.Errorf("not a valid OpenAPI spec (missing swagger/openapi field)")
}

// FetchAndImportOpenAPI downloads a URL and imports it as an OpenAPI spec.
func FetchAndImportOpenAPI(rawURL string) (*Collection, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json, */*")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	return ImportOpenAPI(data)
}

// LooksLikeOpenAPI reports whether data appears to be an OpenAPI JSON spec.
func LooksLikeOpenAPI(data []byte) bool {
	text := strings.TrimSpace(string(data))
	if !strings.HasPrefix(text, "{") {
		return false
	}
	peek := text
	if len(peek) > 512 {
		peek = peek[:512]
	}
	return strings.Contains(peek, `"swagger"`) || strings.Contains(peek, `"openapi"`)
}

func importOpenAPIv2(raw map[string]interface{}) (*Collection, error) {
	title := nestedStr(raw, "info", "title")
	if title == "" {
		title = "API"
	}
	host := strVal(raw["host"])
	scheme := "https"
	if schemes, ok := raw["schemes"].([]interface{}); ok && len(schemes) > 0 {
		scheme = fmt.Sprintf("%v", schemes[0])
	}
	basePath := strings.TrimRight(strVal(raw["basePath"]), "/")
	baseURL := scheme + "://" + host + basePath
	return buildOpenAPICollection(title, baseURL, raw)
}

func importOpenAPIv3(raw map[string]interface{}) (*Collection, error) {
	title := nestedStr(raw, "info", "title")
	if title == "" {
		title = "API"
	}
	baseURL := "https://api.example.com"
	if servers, ok := raw["servers"].([]interface{}); ok && len(servers) > 0 {
		if srv, ok := servers[0].(map[string]interface{}); ok {
			if u := strVal(srv["url"]); u != "" {
				baseURL = u
			}
		}
	}
	baseURL = strings.TrimRight(baseURL, "/")
	return buildOpenAPICollection(title, baseURL, raw)
}

var openAPIMethods = []string{
	"get", "post", "put", "patch", "delete", "head", "options",
}

func buildOpenAPICollection(title, baseURL string, raw map[string]interface{}) (*Collection, error) {
	col := &Collection{
		Name:     title,
		Requests: make(map[string]*Request),
	}

	paths, _ := raw["paths"].(map[string]interface{})
	if len(paths) == 0 {
		return nil, fmt.Errorf("no paths found in spec")
	}

	// Sort paths for deterministic output.
	sortedPaths := make([]string, 0, len(paths))
	for p := range paths {
		sortedPaths = append(sortedPaths, p)
	}
	sort.Strings(sortedPaths)

	for _, path := range sortedPaths {
		pi, ok := paths[path].(map[string]interface{})
		if !ok {
			continue
		}
		for _, method := range openAPIMethods {
			op, ok := pi[method].(map[string]interface{})
			if !ok {
				continue
			}
			req := buildOpenAPIRequest(method, path, baseURL, op)
			col.Requests[req.ID] = req
			col.Order = append(col.Order, req.ID)
		}
	}

	if len(col.Requests) == 0 {
		return nil, fmt.Errorf("no operations found in spec")
	}
	return col, nil
}

func buildOpenAPIRequest(method, path, baseURL string, op map[string]interface{}) *Request {
	req := &Request{
		ID:     newID(),
		Method: strings.ToUpper(method),
		URL:    baseURL + path,
		Body:   Body{Mode: "raw"},
	}

	// Name: summary -> operationId -> "METHOD /path"
	switch {
	case strVal(op["summary"]) != "":
		req.Name = strVal(op["summary"])
	case strVal(op["operationId"]) != "":
		req.Name = strVal(op["operationId"])
	default:
		req.Name = strings.ToUpper(method) + " " + path
	}

	// Content-Type + body template for request-body methods.
	if method == "post" || method == "put" || method == "patch" {
		ct := openAPIContentType(op)
		req.Headers = append(req.Headers, Header{
			Key: "Content-Type", Value: ct, Enabled: true,
		})
		if body := openAPIBodyTemplate(op, ct); body != "" {
			req.Body.Raw = body
		}
	}

	return req
}

func openAPIContentType(op map[string]interface{}) string {
	// v3: requestBody.content keys
	if rb, ok := op["requestBody"].(map[string]interface{}); ok {
		if content, ok := rb["content"].(map[string]interface{}); ok {
			// Prefer JSON.
			for _, prefer := range []string{"application/json", "application/x-www-form-urlencoded"} {
				if _, ok := content[prefer]; ok {
					return prefer
				}
			}
			for ct := range content {
				return ct
			}
		}
	}
	// v2: consumes array
	if consumes, ok := op["consumes"].([]interface{}); ok && len(consumes) > 0 {
		return fmt.Sprintf("%v", consumes[0])
	}
	return "application/json"
}

func openAPIBodyTemplate(op map[string]interface{}, ct string) string {
	if !strings.Contains(ct, "json") {
		return ""
	}
	// v3: requestBody.content.<ct>.schema
	if rb, ok := op["requestBody"].(map[string]interface{}); ok {
		if content, ok := rb["content"].(map[string]interface{}); ok {
			if ctMap, ok := content[ct].(map[string]interface{}); ok {
				if schema, ok := ctMap["schema"].(map[string]interface{}); ok {
					return schemaToTemplate(schema)
				}
			}
		}
	}
	// v2: parameters with in=body
	if params, ok := op["parameters"].([]interface{}); ok {
		for _, p := range params {
			param, ok := p.(map[string]interface{})
			if !ok {
				continue
			}
			if strVal(param["in"]) == "body" {
				if schema, ok := param["schema"].(map[string]interface{}); ok {
					return schemaToTemplate(schema)
				}
			}
		}
	}
	return ""
}

func schemaToTemplate(schema map[string]interface{}) string {
	props, ok := schema["properties"].(map[string]interface{})
	if !ok || len(props) == 0 {
		return "{}"
	}

	keys := make([]string, 0, len(props))
	for k := range props {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	pairs := make([]string, 0, len(keys))
	for _, k := range keys {
		propSchema, _ := props[k].(map[string]interface{})
		var val string
		switch strVal(propSchema["type"]) {
		case "integer", "number":
			val = "0"
		case "boolean":
			val = "false"
		case "array":
			val = "[]"
		case "object":
			val = "{}"
		default:
			if ex := strVal(propSchema["example"]); ex != "" {
				val = fmt.Sprintf("%q", ex)
			} else {
				val = `""`
			}
		}
		pairs = append(pairs, fmt.Sprintf("  %q: %s", k, val))
	}
	return "{\n" + strings.Join(pairs, ",\n") + "\n}"
}

// nestedStr extracts a string value through nested map keys.
func nestedStr(m map[string]interface{}, keys ...string) string {
	var cur interface{} = m
	for _, k := range keys {
		mp, ok := cur.(map[string]interface{})
		if !ok {
			return ""
		}
		cur = mp[k]
	}
	return strVal(cur)
}

func strVal(v interface{}) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}

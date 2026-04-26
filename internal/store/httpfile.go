package store

import (
	"fmt"
	"net/url"
	"strings"
)

// ImportHTTPFile parses a .http / .rest file (VS Code REST Client / IntelliJ
// format) into a Collection. Each ### block becomes one Request.
func ImportHTTPFile(data []byte) (*Collection, error) {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	lines := strings.Split(text, "\n")

	col := &Collection{
		Name:     "Imported",
		Requests: make(map[string]*Request),
	}

	// Split into blocks at ### boundaries; each block starts with its ### line.
	var blocks [][]string
	var cur []string
	for _, line := range lines {
		if strings.HasPrefix(line, "###") {
			if len(cur) > 0 {
				blocks = append(blocks, cur)
			}
			cur = []string{line}
		} else {
			cur = append(cur, line)
		}
	}
	if len(cur) > 0 {
		blocks = append(blocks, cur)
	}

	// File with no ### at all: treat the whole file as one unnamed block.
	if len(blocks) == 0 || !strings.HasPrefix(blocks[0][0], "###") {
		blocks = [][]string{lines}
	}

	for _, block := range blocks {
		req, ok := parseHTTPBlock(block)
		if !ok {
			continue
		}
		col.Requests[req.ID] = req
		col.Order = append(col.Order, req.ID)
	}

	if len(col.Requests) == 0 {
		return nil, fmt.Errorf("no valid requests found in .http file")
	}
	return col, nil
}

func parseHTTPBlock(lines []string) (*Request, bool) {
	req := &Request{
		ID:     newID(),
		Method: "GET",
		Body:   Body{Mode: "raw"},
	}

	pos := 0

	// First line may be the ### separator, optionally followed by a name.
	if pos < len(lines) && strings.HasPrefix(lines[pos], "###") {
		name := strings.TrimSpace(strings.TrimPrefix(lines[pos], "###"))
		if name != "" {
			req.Name = name
		}
		pos++
	}

	// Skip blank lines, comments, and @variable declarations before the method line.
	for pos < len(lines) {
		l := strings.TrimSpace(lines[pos])
		if l == "" || strings.HasPrefix(l, "#") || strings.HasPrefix(l, "@") {
			pos++
			continue
		}
		break
	}
	if pos >= len(lines) {
		return nil, false
	}

	method, rawURL, ok := parseHTTPMethodLine(strings.TrimSpace(lines[pos]))
	if !ok {
		return nil, false
	}
	req.Method = method
	req.URL = rawURL
	pos++

	// Default name from method + path when not set by ### line.
	if req.Name == "" {
		if u, err := url.Parse(rawURL); err == nil && u.Path != "" {
			req.Name = method + " " + u.Path
		} else {
			req.Name = method + " " + rawURL
		}
	}

	// Headers: lines until the first blank line (or a body-file redirect < or a comment).
	headersMap := make(map[string]string)
	for pos < len(lines) {
		l := lines[pos]
		trimmed := strings.TrimSpace(l)
		if trimmed == "" {
			pos++
			break
		}
		// Skip comment lines inside the header block.
		if strings.HasPrefix(trimmed, "#") {
			pos++
			continue
		}
		// < filename: body from file - not supported, skip.
		if strings.HasPrefix(trimmed, "<") {
			pos++
			continue
		}
		if idx := strings.Index(l, ":"); idx > 0 {
			k := strings.TrimSpace(l[:idx])
			v := strings.TrimSpace(l[idx+1:])
			headersMap[k] = v
		}
		pos++
	}
	for k, v := range headersMap {
		req.Headers = append(req.Headers, Header{Key: k, Value: v, Enabled: true})
	}

	// Body: everything remaining, trimmed.
	var bodyLines []string
	for pos < len(lines) {
		bodyLines = append(bodyLines, lines[pos])
		pos++
	}
	if body := strings.TrimSpace(strings.Join(bodyLines, "\n")); body != "" {
		req.Body.Raw = body
	}

	return req, true
}

// parseHTTPMethodLine parses "METHOD URL" or "URL" (defaulting to GET).
// Also handles an optional trailing HTTP version: GET https://... HTTP/1.1
func parseHTTPMethodLine(s string) (method, rawURL string, ok bool) {
	// Strip trailing HTTP version.
	if idx := strings.LastIndex(s, " HTTP/"); idx > 0 {
		s = strings.TrimSpace(s[:idx])
	}

	for _, m := range []string{
		"GET", "POST", "PUT", "PATCH", "DELETE",
		"HEAD", "OPTIONS", "TRACE", "CONNECT",
	} {
		if strings.HasPrefix(strings.ToUpper(s), m+" ") || strings.HasPrefix(strings.ToUpper(s), m+"\t") {
			return m, strings.TrimSpace(s[len(m):]), true
		}
	}
	// Bare URL without a method.
	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "{{") {
		return "GET", s, true
	}
	return "", "", false
}

// ExportHTTPFile serialises a Collection as a .http file compatible with
// VS Code REST Client and IntelliJ HTTP Client.
func ExportHTTPFile(col *Collection) ([]byte, error) {
	var sb strings.Builder

	// Ordered request list: honour col.Order, then any unordered stragglers.
	seen := make(map[string]bool)
	var ordered []*Request
	for _, id := range col.Order {
		if r, ok := col.Requests[id]; ok {
			ordered = append(ordered, r)
			seen[id] = true
		}
	}
	for id, r := range col.Requests {
		if !seen[id] {
			ordered = append(ordered, r)
		}
	}

	for i, r := range ordered {
		if i > 0 {
			sb.WriteString("\n\n")
		}

		name := r.Name
		if name == "" {
			name = r.Method + " " + r.URL
		}
		sb.WriteString("### " + name + "\n")

		method := r.Method
		if method == "" {
			method = "GET"
		}
		sb.WriteString(method + " " + r.URL + "\n")

		for _, h := range r.Headers {
			if h.Enabled && h.Key != "" {
				sb.WriteString(h.Key + ": " + h.Value + "\n")
			}
		}

		if body := strings.TrimSpace(r.Body.Raw); body != "" {
			sb.WriteString("\n")
			sb.WriteString(body + "\n")
		}
	}

	return []byte(sb.String()), nil
}

// LooksLikeHTTPFile reports whether data appears to be a .http / .rest file
// rather than JSON. Used for format auto-detection on import.
func LooksLikeHTTPFile(data []byte) bool {
	text := strings.TrimSpace(string(data))
	if len(text) == 0 {
		return false
	}
	if strings.HasPrefix(text, "###") {
		return true
	}
	first := strings.ToUpper(text)
	for _, m := range []string{"GET ", "POST ", "PUT ", "PATCH ", "DELETE ", "HEAD ", "OPTIONS "} {
		if strings.HasPrefix(first, m) {
			return true
		}
	}
	return false
}

package lint

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/d0mkaaa/gopull/internal/store"
)

type Finding struct {
	Severity string `json:"severity"`
	Request  string `json:"request,omitempty"`
	Field    string `json:"field,omitempty"`
	Message  string `json:"message"`
}

func CheckPath(path string) ([]Finding, error) {
	c, err := store.LoadCollectionPath(path)
	if err != nil {
		return nil, err
	}
	root := ""
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		root = path
	}
	return Check(c, root), nil
}

func Check(c *store.Collection, root string) []Finding {
	var out []Finding
	if c.Name == "" {
		out = append(out, Finding{Severity: "warning", Message: "collection has no name"})
	}
	for _, id := range c.Order {
		r := c.Requests[id]
		if r == nil {
			out = append(out, Finding{Severity: "error", Request: id, Message: "order references a missing request"})
			continue
		}
		name := r.Name
		if name == "" {
			name = id
		}
		out = append(out, checkRequest(name, r, root)...)
	}
	return out
}

func FormatText(findings []Finding) string {
	if len(findings) == 0 {
		return "lint ok\n"
	}
	var b strings.Builder
	for _, f := range findings {
		if f.Request != "" {
			fmt.Fprintf(&b, "%s %s %s: %s\n", strings.ToUpper(f.Severity), f.Request, f.Field, f.Message)
		} else {
			fmt.Fprintf(&b, "%s: %s\n", strings.ToUpper(f.Severity), f.Message)
		}
	}
	return b.String()
}

func FormatJSON(findings []Finding) string {
	data, _ := json.MarshalIndent(findings, "", "  ")
	return string(data) + "\n"
}

func HasErrors(findings []Finding) bool {
	for _, f := range findings {
		if f.Severity == "error" {
			return true
		}
	}
	return false
}

func checkRequest(name string, r *store.Request, root string) []Finding {
	var out []Finding
	add := func(sev, field, msg string) {
		out = append(out, Finding{Severity: sev, Request: name, Field: field, Message: msg})
	}
	if strings.TrimSpace(r.URL) == "" {
		add("error", "url", "missing URL")
	}
	if strings.TrimSpace(r.Method) == "" {
		add("error", "method", "missing method")
	}
	for _, h := range r.Headers {
		if !h.Enabled {
			continue
		}
		key := strings.TrimSpace(h.Key)
		if key == "" {
			add("error", "headers", "header has no name")
			continue
		}
		if strings.ContainsAny(key, " \t\r\n:") {
			add("error", "headers", fmt.Sprintf("invalid header name %q", h.Key))
		}
	}
	mode := r.Body.Mode
	if mode == "" {
		mode = "raw"
	}
	switch mode {
	case "raw", "form", "graphql", "multipart", "file":
	default:
		add("error", "body", fmt.Sprintf("unknown body mode %q", mode))
	}
	if mode == "file" {
		checkFilePath(root, strings.TrimSpace(r.Body.Raw), func(msg string) { add("error", "body", msg) })
	}
	if mode == "multipart" {
		for _, line := range strings.Split(r.Body.Raw, "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "file ") {
				continue
			}
			_, value, ok := strings.Cut(strings.TrimSpace(strings.TrimPrefix(line, "file ")), "=")
			if !ok {
				add("error", "body", fmt.Sprintf("invalid multipart file line %q", line))
				continue
			}
			checkFilePath(root, strings.TrimSpace(value), func(msg string) { add("error", "body", msg) })
		}
	}
	out = append(out, checkPathParams(name, r)...)
	out = append(out, checkTests(name, r.Tests)...)
	return out
}

func checkFilePath(root, path string, add func(string)) {
	if path == "" {
		add("missing file path")
		return
	}
	if strings.Contains(path, "{{") {
		return
	}
	if root != "" && !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	if _, err := os.Stat(path); err != nil {
		add(fmt.Sprintf("file does not exist: %s", path))
	}
}

func checkPathParams(name string, r *store.Request) []Finding {
	var out []Finding
	used := map[string]bool{}
	re := regexp.MustCompile(`:([A-Za-z_][A-Za-z0-9_]*)`)
	for _, m := range re.FindAllStringSubmatch(r.URL, -1) {
		used[m[1]] = true
	}
	declared := map[string]bool{}
	for _, p := range r.Path {
		if p.Enabled {
			declared[p.Key] = true
		}
	}
	for key := range used {
		if !declared[key] {
			out = append(out, Finding{Severity: "error", Request: name, Field: "path", Message: fmt.Sprintf("missing value for :%s", key)})
		}
	}
	for key := range declared {
		if !used[key] {
			out = append(out, Finding{Severity: "warning", Request: name, Field: "path", Message: fmt.Sprintf("path param %q is not used by URL", key)})
		}
	}
	return out
}

func checkTests(name, script string) []Finding {
	var out []Finding
	for i, raw := range strings.Split(script, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "assert ") {
			if !validAssert(strings.TrimSpace(strings.TrimPrefix(line, "assert "))) {
				out = append(out, Finding{Severity: "error", Request: name, Field: "tests", Message: fmt.Sprintf("line %d has invalid assertion syntax", i+1)})
			}
			continue
		}
		if strings.HasPrefix(line, "set ") {
			if k, _, ok := strings.Cut(strings.TrimSpace(strings.TrimPrefix(line, "set ")), "="); !ok || strings.TrimSpace(k) == "" {
				out = append(out, Finding{Severity: "error", Request: name, Field: "tests", Message: fmt.Sprintf("line %d has invalid set syntax", i+1)})
			}
			continue
		}
		out = append(out, Finding{Severity: "error", Request: name, Field: "tests", Message: fmt.Sprintf("line %d must start with assert or set", i+1)})
	}
	return out
}

func validAssert(expr string) bool {
	switch {
	case strings.HasPrefix(expr, "header "):
		return len(strings.SplitN(strings.TrimPrefix(expr, "header "), " ", 3)) == 3
	case strings.HasPrefix(expr, "jsonpath "):
		return len(strings.SplitN(strings.TrimPrefix(expr, "jsonpath "), " ", 3)) == 3
	case strings.HasPrefix(expr, "response_time "):
		return len(strings.SplitN(strings.TrimPrefix(expr, "response_time "), " ", 2)) == 2
	default:
		return len(strings.SplitN(expr, " ", 3)) == 3
	}
}

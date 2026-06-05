package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/d0mkaaa/gopull/internal/store"
)

func TestCheckReportsBrokenRequest(t *testing.T) {
	c := &store.Collection{
		Name: "API",
		Requests: map[string]*store.Request{
			"bad": {
				Name:   "Bad",
				Method: "GET",
				URL:    "https://example.com/users/:id",
				Headers: []store.Header{
					{Key: "Bad Header", Value: "x", Enabled: true},
				},
				Body:  store.Body{Mode: "unknown"},
				Tests: "assert status\nwat",
			},
		},
		Order: []string{"bad"},
	}
	findings := Check(c, "")
	if !HasErrors(findings) {
		t.Fatalf("expected errors, got %#v", findings)
	}
	assertFinding(t, findings, "missing value for :id")
	assertFinding(t, findings, "unknown body mode")
	assertFinding(t, findings, "invalid header name")
	assertFinding(t, findings, "must start with assert or set")
}

func TestCheckPathValidPlainWorkspace(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, store.PlainCollectionFile), []byte("name: API\norder: ping\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ping.gopull"), []byte("name: Ping\nmethod: GET\nurl: https://example.com/ping\n[tests]\nassert status == 200\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	findings, err := CheckPath(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %#v", findings)
	}
}

func TestCheckSecretInAuthorizationHeader(t *testing.T) {
	r := &store.Request{
		Name:   "API",
		Method: "GET",
		URL:    "https://example.com",
		Headers: []store.Header{
			{Key: "Authorization", Value: "Bearer sk-abc123def456ghi789jkl", Enabled: true},
		},
	}
	findings := checkRequest("API", r, "")
	assertFinding(t, findings, "hardcoded secret")
}

func TestCheckSecretInAuthorizationHeaderSkipsTemplate(t *testing.T) {
	r := &store.Request{
		Name:   "API",
		Method: "GET",
		URL:    "https://example.com",
		Headers: []store.Header{
			{Key: "Authorization", Value: "Bearer {{TOKEN}}", Enabled: true},
		},
	}
	for _, f := range checkRequest("API", r, "") {
		if strings.Contains(f.Message, "hardcoded secret") {
			t.Fatalf("false positive on template value: %s", f.Message)
		}
	}
}

func TestCheckSecretInURLQueryParam(t *testing.T) {
	r := &store.Request{
		Name:   "API",
		Method: "GET",
		URL:    "https://example.com/data?api_key=supersecretvalue123",
	}
	findings := checkRequest("API", r, "")
	assertFinding(t, findings, "hardcoded secret")
}

func TestCheckSecretSkipsShortValues(t *testing.T) {
	r := &store.Request{
		Name:   "API",
		Method: "GET",
		URL:    "https://example.com/data?token=short",
	}
	for _, f := range checkRequest("API", r, "") {
		if strings.Contains(f.Message, "hardcoded secret") {
			t.Fatalf("false positive on short value: %s", f.Message)
		}
	}
}

func TestCheckSecretBearerAuthField(t *testing.T) {
	r := &store.Request{
		Name:   "API",
		Method: "GET",
		URL:    "https://example.com",
		Auth:   store.Auth{Type: "bearer", Token: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"},
	}
	findings := checkRequest("API", r, "")
	assertFinding(t, findings, "hardcoded")
}

func TestCheckContentTypeMismatchFormMode(t *testing.T) {
	r := &store.Request{
		Name:   "API",
		Method: "POST",
		URL:    "https://example.com",
		Body:   store.Body{Mode: "form"},
		Headers: []store.Header{
			{Key: "Content-Type", Value: "application/json", Enabled: true},
		},
	}
	findings := checkRequest("API", r, "")
	assertFinding(t, findings, "body mode")
}

func TestCheckContentTypeMismatchCorrectHeader(t *testing.T) {
	r := &store.Request{
		Name:   "API",
		Method: "POST",
		URL:    "https://example.com",
		Body:   store.Body{Mode: "form"},
		Headers: []store.Header{
			{Key: "Content-Type", Value: "application/x-www-form-urlencoded", Enabled: true},
		},
	}
	for _, f := range checkRequest("API", r, "") {
		if strings.Contains(f.Message, "body mode") {
			t.Fatalf("false positive on correct content-type: %s", f.Message)
		}
	}
}

func TestCheckContentTypeMismatchTemplateSkipped(t *testing.T) {
	r := &store.Request{
		Name:   "API",
		Method: "POST",
		URL:    "https://example.com",
		Body:   store.Body{Mode: "multipart"},
		Headers: []store.Header{
			{Key: "Content-Type", Value: "{{CONTENT_TYPE}}", Enabled: true},
		},
	}
	for _, f := range checkRequest("API", r, "") {
		if strings.Contains(f.Message, "body mode") {
			t.Fatalf("false positive on template content-type: %s", f.Message)
		}
	}
}

func assertFinding(t *testing.T, findings []Finding, want string) {
	t.Helper()
	for _, f := range findings {
		if strings.Contains(f.Message, want) {
			return
		}
	}
	t.Fatalf("missing finding containing %q in %#v", want, findings)
}

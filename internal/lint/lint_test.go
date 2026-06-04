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

func assertFinding(t *testing.T, findings []Finding, want string) {
	t.Helper()
	for _, f := range findings {
		if strings.Contains(f.Message, want) {
			return
		}
	}
	t.Fatalf("missing finding containing %q in %#v", want, findings)
}

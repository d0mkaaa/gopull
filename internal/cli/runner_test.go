package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/d0mkaaa/gopull/internal/store"
)

func TestLoadEnvFileSupportsDotenvAndOverrides(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("TOKEN=abc\nexport BASE_URL=https://example.com\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := loadEnvFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got["TOKEN"] != "abc" || got["BASE_URL"] != "https://example.com" {
		t.Fatalf("env: %#v", got)
	}
}

func TestLoadEnvFileSupportsBrunoJSONWithMissingEnabled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "env.json")
	data := `{"variables":[{"name":"TOKEN","value":"abc"},{"name":"OFF","value":"no","enabled":false}]}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := loadEnvFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got["TOKEN"] != "abc" {
		t.Fatalf("missing-enabled variable should default on: %#v", got)
	}
	if _, ok := got["OFF"]; ok {
		t.Fatalf("disabled variable should be absent: %#v", got)
	}
}

func TestLoadEnvRejectsBadOverride(t *testing.T) {
	st, err := storeForCLITest(t)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := loadEnv(st, "", "", []string{"TOKEN"}); err == nil {
		t.Fatal("expected bad override error")
	}
}

func TestNormalizeReportFallsBackToText(t *testing.T) {
	if got := normalizeReport("html"); got != "text" {
		t.Fatalf("got %q", got)
	}
}

func storeForCLITest(t *testing.T) (*store.Store, error) {
	t.Helper()
	return store.NewAt(t.TempDir())
}

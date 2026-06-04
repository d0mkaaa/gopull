package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/d0mkaaa/gopull/internal/store"
)

func TestInitCommandCreatesWorkspace(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "api")
	if code := Init([]string{dir, "--env"}); code != 0 {
		t.Fatalf("Init exit = %d", code)
	}
	if !store.IsPlainCollectionDir(dir) {
		t.Fatal("workspace metadata missing")
	}
	if _, err := os.Stat(filepath.Join(dir, ".env.example")); err != nil {
		t.Fatal(err)
	}
}

func TestLintCommandReturnsOneForErrors(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, store.PlainCollectionFile), []byte("name: API\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bad.gopull"), []byte("name: Bad\nmethod: GET\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := Lint([]string{dir}); code != 1 {
		t.Fatalf("Lint exit = %d, want 1", code)
	}
}

func TestLintCommandAllowsFlagAfterPath(t *testing.T) {
	dir := t.TempDir()
	if err := store.InitPlainWorkspace(dir, "API", false, false); err != nil {
		t.Fatal(err)
	}
	if code := Lint([]string{dir, "--format", "json"}); code != 0 {
		t.Fatalf("Lint exit = %d", code)
	}
}

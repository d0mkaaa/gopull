package tui

import (
	"strings"
	"testing"
)

func TestPrettyJSONIndentsCompact(t *testing.T) {
	in := `{"a":1,"b":{"c":2}}`
	out := prettyJSON([]byte(in))
	if !strings.Contains(out, "\n") {
		t.Error("expected indented output with newlines")
	}
	if !strings.Contains(out, `"a"`) {
		t.Errorf("key 'a' missing from output: %q", out)
	}
}

func TestPrettyJSONPreservesAlreadyIndented(t *testing.T) {
	in := "{\n  \"x\": 1\n}"
	out := prettyJSON([]byte(in))
	if !strings.Contains(out, `"x"`) {
		t.Errorf("key 'x' missing: %q", out)
	}
}

func TestPrettyJSONInvalidReturnsOriginal(t *testing.T) {
	in := `not json at all`
	out := prettyJSON([]byte(in))
	if out != in {
		t.Errorf("invalid JSON should return original: got %q", out)
	}
}

func TestPrettyJSONEmptyInput(t *testing.T) {
	if out := prettyJSON(nil); out != "" {
		t.Errorf("nil input: got %q", out)
	}
	if out := prettyJSON([]byte{}); out != "" {
		t.Errorf("empty input: got %q", out)
	}
}

func TestPrettyJSONArray(t *testing.T) {
	in := `[1,2,3]`
	out := prettyJSON([]byte(in))
	if !strings.Contains(out, "\n") {
		t.Error("array should be indented")
	}
}

func TestPrettyJSONNullAndBooleans(t *testing.T) {
	in := `{"ok":true,"val":null,"n":42}`
	out := prettyJSON([]byte(in))
	if !strings.Contains(out, "true") || !strings.Contains(out, "null") {
		t.Errorf("booleans/null not preserved: %q", out)
	}
}

func TestFormatSizeBytes(t *testing.T) {
	cases := []struct {
		n    int
		want string
	}{
		{0, "0B"},
		{1, "1B"},
		{512, "512B"},
		{1023, "1023B"},
	}
	for _, tc := range cases {
		got := formatSize(tc.n)
		if got != tc.want {
			t.Errorf("formatSize(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}

func TestFormatSizeKilobytes(t *testing.T) {
	cases := []struct {
		n    int
		want string
	}{
		{1024, "1.0kB"},
		{1536, "1.5kB"},
		{10240, "10.0kB"},
		{1024*1024 - 1, "1024.0kB"},
	}
	for _, tc := range cases {
		got := formatSize(tc.n)
		if got != tc.want {
			t.Errorf("formatSize(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}

func TestFormatSizeMegabytes(t *testing.T) {
	cases := []struct {
		n    int
		want string
	}{
		{1024 * 1024, "1.0MB"},
		{1024 * 1024 * 2, "2.0MB"},
		{1024 * 1024 * 10, "10.0MB"},
	}
	for _, tc := range cases {
		got := formatSize(tc.n)
		if got != tc.want {
			t.Errorf("formatSize(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}

func TestWrapTextZeroWidthReturnsOriginal(t *testing.T) {
	s := "hello world this is a long line"
	if got := wrapText(s, 0); got != s {
		t.Errorf("zero width: got %q", got)
	}
	if got := wrapText(s, -1); got != s {
		t.Errorf("negative width: got %q", got)
	}
}

func TestWrapTextShortLineUnchanged(t *testing.T) {
	s := "hello"
	if got := wrapText(s, 80); got != s {
		t.Errorf("short line should not be wrapped: %q", got)
	}
}

func TestWrapTextExactWidthUnchanged(t *testing.T) {
	s := "12345"
	if got := wrapText(s, 5); got != s {
		t.Errorf("exact-width line should not be wrapped: %q", got)
	}
}

func TestWrapTextLongLineWraps(t *testing.T) {
	s := "one two three four five"
	out := wrapText(s, 10)
	lines := strings.Split(out, "\n")
	if len(lines) < 2 {
		t.Errorf("expected wrapping, got single line: %q", out)
	}
	for _, l := range lines {
		if len([]rune(l)) > 10 {
			t.Errorf("line exceeds width: %q", l)
		}
	}
}

func TestWrapTextPreservesNewlines(t *testing.T) {
	s := "line one\nline two\nline three"
	out := wrapText(s, 80)
	if !strings.Contains(out, "line one") || !strings.Contains(out, "line two") {
		t.Errorf("newlines not preserved: %q", out)
	}
	if lines := strings.Split(out, "\n"); len(lines) != 3 {
		t.Errorf("expected 3 lines, got %d: %q", len(lines), out)
	}
}

func TestWrapTextLongWordNotBroken(t *testing.T) {
	// A single word longer than the width should appear as its own line.
	s := "superlongwordthatexceedswidth rest"
	out := wrapText(s, 10)
	if !strings.Contains(out, "superlongwordthatexceedswidth") {
		t.Errorf("long word should not be truncated: %q", out)
	}
}

func TestWrapTextMultilineWithLongLines(t *testing.T) {
	s := "short\na very long line that needs wrapping here definitely\nshort again"
	out := wrapText(s, 20)
	lines := strings.Split(out, "\n")
	if len(lines) < 4 {
		t.Errorf("expected at least 4 lines after wrapping, got %d:\n%s", len(lines), out)
	}
	for _, l := range lines {
		if len([]rune(l)) > 20 {
			t.Errorf("line %q exceeds width 20", l)
		}
	}
}

func TestLexerForContentType(t *testing.T) {
	cases := []struct {
		ct      string
		wantNil bool
	}{
		{"application/json", false},
		{"application/vnd.api+json", false},
		{"text/html", false},
		{"text/xml", false},
		{"application/xml", false},
		{"application/javascript", false},
		{"text/css", false},
		{"application/yaml", false},
		{"text/yaml", false},
		{"application/toml", false},
		{"text/plain", true}, // plain text: let auto-detect decide
		{"", true},           // empty: let auto-detect decide
		{"image/png", true},  // unknown binary: no lexer
		{"audio/mpeg", true},
	}
	for _, tc := range cases {
		t.Run(tc.ct, func(t *testing.T) {
			got := lexerForContentType(tc.ct)
			if tc.wantNil && got != nil {
				t.Errorf("expected nil lexer for %q, got %v", tc.ct, got)
			}
			if !tc.wantNil && got == nil {
				t.Errorf("expected non-nil lexer for %q", tc.ct)
			}
		})
	}
}

func TestHighlightEmptyBodyReturnsEmpty(t *testing.T) {
	if got := highlight(nil, "application/json"); got != "" {
		t.Errorf("nil body: got %q", got)
	}
	if got := highlight([]byte{}, "application/json"); got != "" {
		t.Errorf("empty body: got %q", got)
	}
}

func TestHighlightReturnsNonEmptyForValidJSON(t *testing.T) {
	body := []byte(`{"key":"value"}`)
	out := highlight(body, "application/json")
	if out == "" {
		t.Error("expected non-empty output for valid JSON")
	}
	// Should contain the key text somewhere (possibly with ANSI codes around it).
	if !strings.Contains(out, "key") {
		t.Errorf("output missing 'key': %q", out)
	}
}

func TestHighlightUnknownContentTypeFallsBack(t *testing.T) {
	body := []byte("hello world")
	out := highlight(body, "application/octet-stream")
	// Must not panic and must return something containing the original text.
	if !strings.Contains(out, "hello") {
		t.Errorf("fallback should preserve original text, got: %q", out)
	}
}

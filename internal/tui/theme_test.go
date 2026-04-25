package tui

import (
	"strings"
	"testing"
)

func TestThemeFromJSONValid(t *testing.T) {
	data := []byte(`{
		"name": "ocean",
		"chroma": "monokai",
		"bg": "#001122",
		"accent": "#00aaff",
		"muted": "#334455",
		"border": "#223344",
		"text": "#eeeeff",
		"subtle": "#778899",
		"success": "#00cc88",
		"warn": "#ffaa00",
		"error": "#ff3355",
		"search_bg": "#ffaa00",
		"search_fg": "#000000",
		"badge_fg": "#001122",
		"methods": {
			"GET": "#00cc88",
			"POST": "#ffaa00",
			"DELETE": "#ff3355"
		}
	}`)

	th, err := ThemeFromJSON(data)
	if err != nil {
		t.Fatalf("ThemeFromJSON: %v", err)
	}
	if th.name != "ocean" {
		t.Errorf("name: got %q", th.name)
	}
	if th.ChromaStyle != "monokai" {
		t.Errorf("chroma: got %q", th.ChromaStyle)
	}
	if string(th.Background) != "#001122" {
		t.Errorf("background: got %q", th.Background)
	}
	if string(th.Accent) != "#00aaff" {
		t.Errorf("accent: got %q", th.Accent)
	}
}

func TestThemeFromJSONMissingNameErrors(t *testing.T) {
	data := []byte(`{"chroma":"dracula","accent":"#cba6f7"}`)
	_, err := ThemeFromJSON(data)
	if err == nil {
		t.Fatal("expected error for missing name field")
	}
	if !strings.Contains(err.Error(), "name") {
		t.Errorf("error should mention 'name': %v", err)
	}
}

func TestThemeFromJSONInvalidJSONErrors(t *testing.T) {
	_, err := ThemeFromJSON([]byte(`not json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestThemeFromJSONDefaultsChromaStyle(t *testing.T) {
	data := []byte(`{"name":"minimal","accent":"#aabbcc"}`)
	th, err := ThemeFromJSON(data)
	if err != nil {
		t.Fatalf("ThemeFromJSON: %v", err)
	}
	if th.ChromaStyle != "dracula" {
		t.Errorf("chroma should default to dracula, got %q", th.ChromaStyle)
	}
}

func TestThemeFromJSONFillsMissingMethods(t *testing.T) {
	// When methods map is absent, all methods should default to accent color.
	data := []byte(`{"name":"minimal","accent":"#ff0000"}`)
	th, err := ThemeFromJSON(data)
	if err != nil {
		t.Fatalf("ThemeFromJSON: %v", err)
	}
	for _, method := range []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"} {
		c, ok := th.MethodColors[method]
		if !ok {
			t.Errorf("method %q missing from MethodColors", method)
		}
		if string(c) != "#ff0000" {
			t.Errorf("method %q: got %q, want accent #ff0000", method, c)
		}
	}
}

func TestThemeFromJSONMethodsAreUppercased(t *testing.T) {
	data := []byte(`{
		"name":"t",
		"accent":"#aabbcc",
		"methods":{"get":"#112233","post":"#445566"}
	}`)
	th, err := ThemeFromJSON(data)
	if err != nil {
		t.Fatalf("ThemeFromJSON: %v", err)
	}
	if _, ok := th.MethodColors["GET"]; !ok {
		t.Error("GET (from lowercase 'get') not found in MethodColors")
	}
	if _, ok := th.MethodColors["POST"]; !ok {
		t.Error("POST (from lowercase 'post') not found in MethodColors")
	}
}

func TestThemeFromJSONEmptyBgIsAllowed(t *testing.T) {
	data := []byte(`{"name":"transparent","accent":"#aabbcc"}`)
	th, err := ThemeFromJSON(data)
	if err != nil {
		t.Fatalf("ThemeFromJSON: %v", err)
	}
	if th.Background != "" {
		t.Errorf("empty bg should stay empty, got %q", th.Background)
	}
}

func TestThemeByIDKnownThemes(t *testing.T) {
	for _, id := range []string{"dark", "light", "nord", "gruvbox"} {
		th := themeByID(id)
		if th.name != id {
			t.Errorf("themeByID(%q).name = %q", id, th.name)
		}
	}
}

func TestThemeByIDUnknownFallsToDark(t *testing.T) {
	th := themeByID("does-not-exist")
	if th.name != "dark" {
		t.Errorf("unknown ID should fall back to dark, got %q", th.name)
	}
}

func TestAllThemeOptionsContainsBuiltins(t *testing.T) {
	opts := AllThemeOptions()
	ids := make(map[string]bool, len(opts))
	for _, o := range opts {
		ids[o.id] = true
	}
	for _, want := range []string{"dark", "light", "nord", "gruvbox"} {
		if !ids[want] {
			t.Errorf("built-in theme %q missing from AllThemeOptions", want)
		}
	}
}

func TestAllThemeOptionsBuiltinsFirst(t *testing.T) {
	opts := AllThemeOptions()
	if len(opts) < 4 {
		t.Fatalf("expected at least 4 options, got %d", len(opts))
	}
	builtins := []string{"dark", "light", "nord", "gruvbox"}
	for i, want := range builtins {
		if opts[i].id != want {
			t.Errorf("opts[%d].id = %q, want %q", i, opts[i].id, want)
		}
	}
}

func TestAllThemeOptionsHaveLabelsAndDescs(t *testing.T) {
	for _, o := range AllThemeOptions() {
		if o.id == "" {
			t.Error("option has empty id")
		}
		if o.label == "" {
			t.Errorf("option %q has empty label", o.id)
		}
		if o.desc == "" {
			t.Errorf("option %q has empty desc", o.id)
		}
	}
}

func TestBuiltinThemesHaveRequiredColors(t *testing.T) {
	for _, id := range []string{"dark", "light", "nord", "gruvbox"} {
		th := themeByID(id)
		if th.Accent == "" {
			t.Errorf("%s: Accent is empty", id)
		}
		if th.Background == "" {
			t.Errorf("%s: Background is empty", id)
		}
		if th.Text == "" {
			t.Errorf("%s: Text is empty", id)
		}
		if th.ChromaStyle == "" {
			t.Errorf("%s: ChromaStyle is empty", id)
		}
		for _, method := range []string{"GET", "POST", "PUT", "PATCH", "DELETE"} {
			if _, ok := th.MethodColors[method]; !ok {
				t.Errorf("%s: missing method color for %s", id, method)
			}
		}
	}
}

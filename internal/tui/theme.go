package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type Theme struct {
	name        string
	ChromaStyle string

	Background lipgloss.Color // empty = use terminal default

	Accent  lipgloss.Color
	Muted   lipgloss.Color
	Border  lipgloss.Color
	Success lipgloss.Color
	Warn    lipgloss.Color
	Error   lipgloss.Color
	Subtle  lipgloss.Color
	Text    lipgloss.Color

	SearchMatchBg lipgloss.Color
	SearchMatchFg lipgloss.Color
	BadgeFg       lipgloss.Color

	MethodColors map[string]lipgloss.Color
}

var darkTheme = Theme{
	name:        "dark",
	ChromaStyle: "dracula",

	Background: lipgloss.Color("#1e1e2e"), // catppuccin mocha base

	Accent:  lipgloss.Color("#cba6f7"),
	Muted:   lipgloss.Color("#585b70"),
	Border:  lipgloss.Color("#313244"),
	Success: lipgloss.Color("#a6e3a1"),
	Warn:    lipgloss.Color("#fab387"),
	Error:   lipgloss.Color("#f38ba8"),
	Subtle:  lipgloss.Color("#7f849c"),
	Text:    lipgloss.Color("#cdd6f4"),

	SearchMatchBg: lipgloss.Color("#f9e2af"),
	SearchMatchFg: lipgloss.Color("#1e1e2e"),
	BadgeFg:       lipgloss.Color("#1e1e2e"),

	MethodColors: map[string]lipgloss.Color{
		"GET":     lipgloss.Color("#a6e3a1"),
		"POST":    lipgloss.Color("#fab387"),
		"PUT":     lipgloss.Color("#89b4fa"),
		"DELETE":  lipgloss.Color("#f38ba8"),
		"PATCH":   lipgloss.Color("#cba6f7"),
		"HEAD":    lipgloss.Color("#585b70"),
		"OPTIONS": lipgloss.Color("#585b70"),
	},
}

var lightTheme = Theme{
	name:        "light",
	ChromaStyle: "github",

	Background: lipgloss.Color("#ffffff"),

	Accent:  lipgloss.Color("#8839ef"),
	Muted:   lipgloss.Color("#acb0be"),
	Border:  lipgloss.Color("#ccd0da"),
	Success: lipgloss.Color("#40a02b"),
	Warn:    lipgloss.Color("#fe640b"),
	Error:   lipgloss.Color("#d20f39"),
	Subtle:  lipgloss.Color("#5c5f77"),
	Text:    lipgloss.Color("#4c4f69"),

	SearchMatchBg: lipgloss.Color("#df8e1d"),
	SearchMatchFg: lipgloss.Color("#eff1f5"),
	BadgeFg:       lipgloss.Color("#eff1f5"),

	MethodColors: map[string]lipgloss.Color{
		"GET":     lipgloss.Color("#40a02b"),
		"POST":    lipgloss.Color("#fe640b"),
		"PUT":     lipgloss.Color("#1e66f5"),
		"DELETE":  lipgloss.Color("#d20f39"),
		"PATCH":   lipgloss.Color("#8839ef"),
		"HEAD":    lipgloss.Color("#9ca0b0"),
		"OPTIONS": lipgloss.Color("#9ca0b0"),
	},
}

var nordTheme = Theme{
	name:        "nord",
	ChromaStyle: "nord",

	Background: lipgloss.Color("#2E3440"), // nord polar night

	Accent:  lipgloss.Color("#88C0D0"),
	Muted:   lipgloss.Color("#4C566A"),
	Border:  lipgloss.Color("#3B4252"),
	Success: lipgloss.Color("#A3BE8C"),
	Warn:    lipgloss.Color("#EBCB8B"),
	Error:   lipgloss.Color("#BF616A"),
	Subtle:  lipgloss.Color("#81A1C1"),
	Text:    lipgloss.Color("#ECEFF4"),

	SearchMatchBg: lipgloss.Color("#EBCB8B"),
	SearchMatchFg: lipgloss.Color("#2E3440"),
	BadgeFg:       lipgloss.Color("#2E3440"),

	MethodColors: map[string]lipgloss.Color{
		"GET":     lipgloss.Color("#A3BE8C"),
		"POST":    lipgloss.Color("#EBCB8B"),
		"PUT":     lipgloss.Color("#88C0D0"),
		"DELETE":  lipgloss.Color("#BF616A"),
		"PATCH":   lipgloss.Color("#B48EAD"),
		"HEAD":    lipgloss.Color("#4C566A"),
		"OPTIONS": lipgloss.Color("#4C566A"),
	},
}

var gruvboxTheme = Theme{
	name:        "gruvbox",
	ChromaStyle: "gruvbox",

	Background: lipgloss.Color("#282828"), // gruvbox dark bg

	Accent:  lipgloss.Color("#d3869b"),
	Muted:   lipgloss.Color("#665c54"),
	Border:  lipgloss.Color("#504945"),
	Success: lipgloss.Color("#b8bb26"),
	Warn:    lipgloss.Color("#fabd2f"),
	Error:   lipgloss.Color("#fb4934"),
	Subtle:  lipgloss.Color("#928374"),
	Text:    lipgloss.Color("#ebdbb2"),

	SearchMatchBg: lipgloss.Color("#fabd2f"),
	SearchMatchFg: lipgloss.Color("#282828"),
	BadgeFg:       lipgloss.Color("#282828"),

	MethodColors: map[string]lipgloss.Color{
		"GET":     lipgloss.Color("#b8bb26"),
		"POST":    lipgloss.Color("#fabd2f"),
		"PUT":     lipgloss.Color("#83a598"),
		"DELETE":  lipgloss.Color("#fb4934"),
		"PATCH":   lipgloss.Color("#d3869b"),
		"HEAD":    lipgloss.Color("#665c54"),
		"OPTIONS": lipgloss.Color("#665c54"),
	},
}

var themeRegistry = map[string]Theme{
	"dark":    darkTheme,
	"light":   lightTheme,
	"nord":    nordTheme,
	"gruvbox": gruvboxTheme,
}

var builtinThemeIDs = []string{"dark", "light", "nord", "gruvbox"}

func themeByID(id string) Theme {
	if t, ok := themeRegistry[id]; ok {
		return t
	}
	return darkTheme
}

// AllThemeOptions returns a sorted list for the settings picker:
// built-ins in their canonical order, then user themes alphabetically.
func AllThemeOptions() []themeOption {
	opts := []themeOption{
		{"dark", "dark", "catppuccin mocha"},
		{"light", "light", "catppuccin latte"},
		{"nord", "nord", "arctic blue"},
		{"gruvbox", "gruvbox", "warm retro"},
	}
	var custom []themeOption
	for id, t := range themeRegistry {
		isBuiltin := false
		for _, bid := range builtinThemeIDs {
			if bid == id {
				isBuiltin = true
				break
			}
		}
		if !isBuiltin {
			custom = append(custom, themeOption{id, t.name, "custom"})
		}
	}
	sort.Slice(custom, func(i, j int) bool { return custom[i].id < custom[j].id })
	return append(opts, custom...)
}

// All color fields are CSS hex strings, e.g. "#ff6b6b".
// See ~/.config/gopull/themes/example.json for a starter template.
type themeJSON struct {
	Name     string            `json:"name"`
	Chroma   string            `json:"chroma"`
	Bg       string            `json:"bg,omitempty"`
	Accent   string            `json:"accent"`
	Muted    string            `json:"muted"`
	Border   string            `json:"border"`
	Success  string            `json:"success"`
	Warn     string            `json:"warn"`
	Error    string            `json:"error"`
	Subtle   string            `json:"subtle"`
	Text     string            `json:"text"`
	SearchBg string            `json:"search_bg"`
	SearchFg string            `json:"search_fg"`
	BadgeFg  string            `json:"badge_fg"`
	Methods  map[string]string `json:"methods"`
}

// ThemeFromJSON parses a user theme JSON file.
func ThemeFromJSON(data []byte) (Theme, error) {
	var tj themeJSON
	if err := json.Unmarshal(data, &tj); err != nil {
		return Theme{}, err
	}
	if tj.Name == "" {
		return Theme{}, fmt.Errorf("theme file missing \"name\" field")
	}
	if tj.Chroma == "" {
		tj.Chroma = "dracula"
	}

	def := func(s, fallback string) lipgloss.Color {
		if strings.HasPrefix(s, "#") || strings.HasPrefix(s, "rgb") {
			return lipgloss.Color(s)
		}
		if s != "" {
			return lipgloss.Color(s)
		}
		return lipgloss.Color(fallback)
	}

	accent := def(tj.Accent, "#cba6f7")
	mc := make(map[string]lipgloss.Color)
	for k, v := range tj.Methods {
		mc[strings.ToUpper(k)] = lipgloss.Color(v)
	}
	for _, method := range []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"} {
		if _, ok := mc[method]; !ok {
			mc[method] = accent
		}
	}

	return Theme{
		name:          tj.Name,
		ChromaStyle:   tj.Chroma,
		Background:    lipgloss.Color(tj.Bg), // empty string = no custom background
		Accent:        accent,
		Muted:         def(tj.Muted, "#585b70"),
		Border:        def(tj.Border, "#313244"),
		Success:       def(tj.Success, "#a6e3a1"),
		Warn:          def(tj.Warn, "#fab387"),
		Error:         def(tj.Error, "#f38ba8"),
		Subtle:        def(tj.Subtle, "#7f849c"),
		Text:          def(tj.Text, "#cdd6f4"),
		SearchMatchBg: def(tj.SearchBg, "#f9e2af"),
		SearchMatchFg: def(tj.SearchFg, "#1e1e2e"),
		BadgeFg:       def(tj.BadgeFg, "#1e1e2e"),
		MethodColors:  mc,
	}, nil
}

// LoadUserThemes scans dir for *.json files, parses them as themes,
// and registers any valid ones into the global theme registry.
func LoadUserThemes(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		t, err := ThemeFromJSON(data)
		if err != nil {
			continue
		}
		themeRegistry[t.name] = t
	}
}

func applyTheme(t Theme) {
	colorBg = t.Background
	colorAccent = t.Accent
	colorMuted = t.Muted
	colorBorder = t.Border
	colorSuccess = t.Success
	colorWarn = t.Warn
	colorError = t.Error
	colorSubtle = t.Subtle
	colorText = t.Text
	colorBadgeFg = t.BadgeFg

	chromaStyle = t.ChromaStyle
	methodColors = t.MethodColors

	pane = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorBorder).
		Padding(0, 1)
	if colorBg != "" {
		pane = pane.Background(colorBg).BorderBackground(colorBg)
	}

	paneActive = pane.Copy().
		BorderForeground(colorAccent)

	statusOK = lipgloss.NewStyle().Foreground(colorSuccess).Bold(true)
	statusWarn = lipgloss.NewStyle().Foreground(colorWarn).Bold(true)
	statusErr = lipgloss.NewStyle().Foreground(colorError).Bold(true)

	hint = lipgloss.NewStyle().Foreground(colorSubtle)
	if colorBg != "" {
		hint = hint.Background(colorBg)
	}

	tabActive = lipgloss.NewStyle().
		Foreground(colorAccent).
		Bold(true)

	tabFocused = lipgloss.NewStyle().
		Foreground(colorSubtle)

	tabInactive = lipgloss.NewStyle().
		Foreground(colorMuted)

	if colorBg != "" {
		tabActive = tabActive.Background(colorBg)
		tabFocused = tabFocused.Background(colorBg)
		tabInactive = tabInactive.Background(colorBg)
	}

	sidebarTitle = lipgloss.NewStyle().
		Foreground(colorAccent).
		Bold(true)
	if colorBg != "" {
		sidebarTitle = sidebarTitle.Background(colorBg)
	}

	sidebarBack = lipgloss.NewStyle().
		Foreground(colorMuted).
		Italic(true)

	envBadge = lipgloss.NewStyle().
		Foreground(t.BadgeFg).
		Background(colorAccent).
		Padding(0, 1)

	statusBar = lipgloss.NewStyle().
		Foreground(colorSubtle)
	if colorBg != "" {
		statusBar = statusBar.Background(colorBg)
	}

	testPass = lipgloss.NewStyle().Foreground(colorSuccess).Bold(true)
	testFail = lipgloss.NewStyle().Foreground(colorError).Bold(true)
	testSet = lipgloss.NewStyle().Foreground(colorAccent)
	if colorBg != "" {
		testPass = testPass.Background(colorBg)
		testFail = testFail.Background(colorBg)
		testSet = testSet.Background(colorBg)
	}

	searchMatch = lipgloss.NewStyle().
		Background(t.SearchMatchBg).
		Foreground(t.SearchMatchFg)

	formMode = lipgloss.NewStyle().
		Foreground(colorAccent).
		Bold(true)
}

// WriteExampleTheme writes a starter theme JSON to dir/example.json if it
// doesn't already exist. This helps users discover the format.
func WriteExampleTheme(dir string) {
	path := filepath.Join(dir, "example.json")
	if _, err := os.Stat(path); err == nil {
		return // already exists
	}
	example := `{
  "_comment": "Copy this file, rename it, and edit the colors. Reload gopull to pick it up.",
  "name": "my-theme",
  "chroma": "dracula",
  "bg":       "#1e1e2e",
  "accent":   "#cba6f7",
  "muted":    "#585b70",
  "border":   "#313244",
  "success":  "#a6e3a1",
  "warn":     "#fab387",
  "error":    "#f38ba8",
  "subtle":   "#7f849c",
  "text":     "#cdd6f4",
  "search_bg": "#f9e2af",
  "search_fg": "#1e1e2e",
  "badge_fg":  "#1e1e2e",
  "methods": {
    "GET":     "#a6e3a1",
    "POST":    "#fab387",
    "PUT":     "#89b4fa",
    "PATCH":   "#cba6f7",
    "DELETE":  "#f38ba8",
    "HEAD":    "#585b70",
    "OPTIONS": "#585b70"
  }
}
`
	_ = os.WriteFile(path, []byte(example), 0o644)
}

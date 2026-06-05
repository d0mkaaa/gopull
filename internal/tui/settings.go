package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type settingsFocus int

const (
	sfThemes settingsFocus = iota
	sfTimeout
	sfMaxDisplay
	sfThemeEditor
)

type themeOption struct {
	id    string
	label string
	desc  string
}

const numEditorFields = 11

var editorFieldLabels = [numEditorFields]string{
	"name", "accent", "background", "border",
	"text", "subtle", "muted",
	"success", "warn", "error",
	"chroma",
}

var editorFieldPlaceholders = [numEditorFields]string{
	"my-theme", "#cba6f7", "(empty = terminal)", "#313244",
	"#cdd6f4", "#7f849c", "#585b70",
	"#a6e3a1", "#fab387", "#f38ba8",
	"dracula",
}

type themeEditorModel struct {
	inputs       [numEditorFields]textinput.Model
	focus        int
	searchMatchBg lipgloss.Color
	searchMatchFg lipgloss.Color
	badgeFg       lipgloss.Color
	methodColors  map[string]lipgloss.Color
}

func newThemeEditorModel(base Theme) themeEditorModel {
	var e themeEditorModel
	for i := range e.inputs {
		inp := textinput.New()
		inp.CharLimit = 30
		inp.Width = 18
		inp.Placeholder = editorFieldPlaceholders[i]
		e.inputs[i] = inp
	}
	// Pre-fill from the base theme.
	e.inputs[0].SetValue(base.name)
	e.inputs[1].SetValue(string(base.Accent))
	e.inputs[2].SetValue(string(base.Background))
	e.inputs[3].SetValue(string(base.Border))
	e.inputs[4].SetValue(string(base.Text))
	e.inputs[5].SetValue(string(base.Subtle))
	e.inputs[6].SetValue(string(base.Muted))
	e.inputs[7].SetValue(string(base.Success))
	e.inputs[8].SetValue(string(base.Warn))
	e.inputs[9].SetValue(string(base.Error))
	e.inputs[10].SetValue(base.ChromaStyle)
	e.inputs[0].Focus()
	e.searchMatchBg = base.SearchMatchBg
	e.searchMatchFg = base.SearchMatchFg
	e.badgeFg = base.BadgeFg
	e.methodColors = base.MethodColors
	return e
}

func (e themeEditorModel) Update(msg tea.Msg) (themeEditorModel, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		var cmd tea.Cmd
		e.inputs[e.focus], cmd = e.inputs[e.focus].Update(msg)
		return e, cmd
	}

	switch km.Type {
	case tea.KeyTab:
		e.inputs[e.focus].Blur()
		e.focus = (e.focus + 1) % numEditorFields
		e.inputs[e.focus].Focus()
		return e, textinput.Blink

	case tea.KeyShiftTab:
		e.inputs[e.focus].Blur()
		e.focus = (e.focus + numEditorFields - 1) % numEditorFields
		e.inputs[e.focus].Focus()
		return e, textinput.Blink
	}

	var cmd tea.Cmd
	e.inputs[e.focus], cmd = e.inputs[e.focus].Update(msg)
	return e, cmd
}

func (e themeEditorModel) View(width int) string {
	accent := themed(lipgloss.NewStyle().Foreground(colorAccent).Bold(true))
	sub := themed(lipgloss.NewStyle().Foreground(colorSubtle))
	muted := themed(lipgloss.NewStyle().Foreground(colorMuted))

	var sb strings.Builder
	sb.WriteString(sidebarTitle.Render("new theme"))
	sb.WriteString("\n\n")

	for i, inp := range e.inputs {
		label := sub.Width(12).Render(editorFieldLabels[i])

		// Color swatch: show a filled block for hex colors.
		swatch := "  "
		if i > 0 && i < numEditorFields-1 {
			v := strings.TrimSpace(inp.Value())
			if strings.HasPrefix(v, "#") && (len(v) == 7 || len(v) == 4) {
				swatch = lipgloss.NewStyle().Background(lipgloss.Color(v)).Render("  ")
			}
		}

		fieldView := inp.View()
		if i == e.focus {
			fieldView = accent.Render(">") + " " + fieldView
		} else {
			fieldView = "  " + fieldView
		}

		sb.WriteString(label + fieldView + " " + swatch + "\n")
	}

	sb.WriteString("\n")
	sb.WriteString(muted.Render("tab/shift-tab navigate"))
	sb.WriteString("\n")
	sb.WriteString(hint.Render("ctrl+s save   esc back"))

	_ = width
	return sb.String()
}

func (e themeEditorModel) toThemeJSON() themeJSON {
	get := func(i int) string { return strings.TrimSpace(e.inputs[i].Value()) }
	chroma := get(10)
	if chroma == "" {
		chroma = "dracula"
	}
	tj := themeJSON{
		Name:     get(0),
		Chroma:   chroma,
		Bg:       get(2),
		Accent:   get(1),
		Border:   get(3),
		Text:     get(4),
		Subtle:   get(5),
		Muted:    get(6),
		Success:  get(7),
		Warn:     get(8),
		Error:    get(9),
		SearchBg: string(e.searchMatchBg),
		SearchFg: string(e.searchMatchFg),
		BadgeFg:  string(e.badgeFg),
	}
	if len(e.methodColors) > 0 {
		tj.Methods = make(map[string]string, len(e.methodColors))
		for k, v := range e.methodColors {
			tj.Methods[k] = string(v)
		}
	}
	return tj
}

type SettingsModel struct {
	themes          []themeOption
	themeIdx        int
	timeoutInput    textinput.Model
	maxDisplayInput textinput.Model
	focus           settingsFocus
	timeoutSecs     int
	maxDisplayBytes int
	configDir       string
	workspaceDir    string

	themeEditor themeEditorModel
}

func newSettings(currentTheme string, timeoutSecs, maxDisplayBytes int, configDir, workspaceDir string) SettingsModel {
	ti := textinput.New()
	ti.Placeholder = "30"
	ti.CharLimit = 5
	ti.Width = 6
	ti.SetValue(strconv.Itoa(timeoutSecs))

	mb := bytesToMB(maxDisplayBytes)
	mi := textinput.New()
	mi.Placeholder = "5"
	mi.CharLimit = 5
	mi.Width = 6
	mi.SetValue(strconv.Itoa(mb))

	themes := AllThemeOptions()

	idx := 0
	for i, t := range themes {
		if t.id == currentTheme {
			idx = i
			break
		}
	}

	return SettingsModel{
		themes:          themes,
		themeIdx:        idx,
		timeoutInput:    ti,
		maxDisplayInput: mi,
		focus:           sfThemes,
		timeoutSecs:     timeoutSecs,
		maxDisplayBytes: maxDisplayBytes,
		configDir:       configDir,
		workspaceDir:    workspaceDir,
	}
}

func (m SettingsModel) InThemeEditor() bool { return m.focus == sfThemeEditor }

func (m SettingsModel) ExitThemeEditor() SettingsModel {
	m.focus = sfThemes
	return m
}

func (m SettingsModel) overlayWidth() int {
	if m.focus == sfThemeEditor {
		return 72
	}
	return 64
}

func (m SettingsModel) nextFocus() SettingsModel {
	switch m.focus {
	case sfThemes:
		m.focus = sfTimeout
		m.timeoutInput.Focus()
	case sfTimeout:
		m.timeoutInput.Blur()
		m.focus = sfMaxDisplay
		m.maxDisplayInput.Focus()
	case sfMaxDisplay:
		m.maxDisplayInput.Blur()
		m.focus = sfThemes
	}
	return m
}

func (m SettingsModel) prevFocus() SettingsModel {
	switch m.focus {
	case sfThemes:
		m.focus = sfMaxDisplay
		m.maxDisplayInput.Focus()
	case sfTimeout:
		m.timeoutInput.Blur()
		m.focus = sfThemes
	case sfMaxDisplay:
		m.maxDisplayInput.Blur()
		m.focus = sfTimeout
		m.timeoutInput.Focus()
	}
	return m
}

func (m SettingsModel) Update(msg tea.Msg) (SettingsModel, tea.Cmd) {
	// Theme editor handles its own inputs.
	if m.focus == sfThemeEditor {
		km, ok := msg.(tea.KeyMsg)
		if ok {
			if km.Type == tea.KeyCtrlS {
				tj := m.themeEditor.toThemeJSON()
				if tj.Name == "" {
					return m, nil // require a name before saving
				}
				return m, saveCustomThemeFileCmd(m.configDir, tj)
			}
		}
		var cmd tea.Cmd
		m.themeEditor, cmd = m.themeEditor.Update(msg)
		return m, cmd
	}

	km, ok := msg.(tea.KeyMsg)
	if !ok {
		if m.focus == sfTimeout {
			var cmd tea.Cmd
			m.timeoutInput, cmd = m.timeoutInput.Update(msg)
			return m, cmd
		}
		if m.focus == sfMaxDisplay {
			var cmd tea.Cmd
			m.maxDisplayInput, cmd = m.maxDisplayInput.Update(msg)
			return m, cmd
		}
		return m, nil
	}

	switch km.Type {
	case tea.KeyTab:
		m = m.nextFocus()
		if m.focus == sfTimeout || m.focus == sfMaxDisplay {
			return m, textinput.Blink
		}
		return m, nil

	case tea.KeyShiftTab:
		m = m.prevFocus()
		if m.focus == sfTimeout || m.focus == sfMaxDisplay {
			return m, textinput.Blink
		}
		return m, nil
	}

	if m.focus == sfThemes {
		switch km.String() {
		case "j", "down":
			if m.themeIdx < len(m.themes)-1 {
				m.themeIdx++
				return m.applySelected()
			}
		case "k", "up":
			if m.themeIdx > 0 {
				m.themeIdx--
				return m.applySelected()
			}
		case "n":
			// Open the theme editor seeded with the current theme as a starting point.
			base := themeByID(m.themes[m.themeIdx].id)
			base.name = "" // clear name so user must choose one
			m.themeEditor = newThemeEditorModel(base)
			m.focus = sfThemeEditor
			return m, textinput.Blink
		}
		return m, nil
	}

	var cmd tea.Cmd
	if m.focus == sfMaxDisplay {
		m.maxDisplayInput, cmd = m.maxDisplayInput.Update(msg)
		return m, cmd
	}
	m.timeoutInput, cmd = m.timeoutInput.Update(msg)
	return m, cmd
}

func (m SettingsModel) applySelected() (SettingsModel, tea.Cmd) {
	id := m.themes[m.themeIdx].id
	applyTheme(themeByID(id))
	return m, func() tea.Msg { return themeAppliedMsg{theme: id} }
}

// TimeoutValue returns the parsed timeout in seconds, falling back to the
// previous value if the field is empty or non-numeric.
func (m SettingsModel) TimeoutValue() int {
	v, err := strconv.Atoi(strings.TrimSpace(m.timeoutInput.Value()))
	if err != nil || v <= 0 {
		return m.timeoutSecs
	}
	return v
}

func (m SettingsModel) MaxDisplayBytesValue() int {
	v, err := strconv.Atoi(strings.TrimSpace(m.maxDisplayInput.Value()))
	if err != nil || v <= 0 {
		return m.maxDisplayBytes
	}
	return v * 1024 * 1024
}

func (m SettingsModel) SelectedTheme() string {
	if m.themeIdx < len(m.themes) {
		return m.themes[m.themeIdx].id
	}
	return "dark"
}

func (m SettingsModel) View() string {
	if m.focus == sfThemeEditor {
		return m.themeEditor.View(m.overlayWidth())
	}

	accent := themed(lipgloss.NewStyle().Foreground(colorAccent).Bold(true))
	muted := themed(lipgloss.NewStyle().Foreground(colorMuted))
	sub := themed(lipgloss.NewStyle().Foreground(colorSubtle))

	sectionLabel := func(label string, active bool) string {
		if active {
			return accent.Render(label)
		}
		return sub.Render(label)
	}

	var sb strings.Builder

	sb.WriteString(sidebarTitle.Render("settings"))
	sb.WriteString("\n\n")

	focused := m.focus == sfThemes
	sb.WriteString(sectionLabel("theme", focused))
	sb.WriteString("\n")

	for i, t := range m.themes {
		if i == m.themeIdx {
			bullet := accent.Render("  * " + t.label)
			desc := muted.Render("   " + t.desc)
			sb.WriteString(bullet + desc + "\n")
		} else {
			sb.WriteString(muted.Render("    "+t.label) + "\n")
		}
	}

	sb.WriteString("\n")

	focused = m.focus == sfTimeout
	sb.WriteString(sectionLabel("timeout", focused))
	sb.WriteString("\n")
	sb.WriteString("  " + m.timeoutInput.View() + sub.Render(" seconds"))
	sb.WriteString("\n\n")

	focused = m.focus == sfMaxDisplay
	sb.WriteString(sectionLabel("max response display", focused))
	sb.WriteString("\n")
	sb.WriteString("  " + m.maxDisplayInput.View() + sub.Render(" MB before body is hidden"))
	sb.WriteString("\n\n")

	sb.WriteString(sectionLabel("workspace", false))
	sb.WriteString("\n")
	if m.workspaceDir != "" {
		sb.WriteString("  " + muted.Render(clipText(m.workspaceDir, 54)))
	} else {
		sb.WriteString("  " + muted.Render("config store"))
	}
	sb.WriteString("\n\n")

	var hintStr string
	if m.focus == sfThemes {
		hintStr = "up/down pick   n new theme   tab next   esc save"
	} else {
		hintStr = "tab next   shift+tab previous   esc save"
	}
	sb.WriteString(hint.Render(hintStr))

	return sb.String()
}

func bytesToMB(bytes int) int {
	if bytes <= 0 {
		return 5
	}
	mb := bytes / (1024 * 1024)
	if mb <= 0 {
		return 1
	}
	return mb
}

func saveCustomThemeFileCmd(configDir string, tj themeJSON) tea.Cmd {
	return func() tea.Msg {
		if tj.Name == "" {
			return customThemeSavedMsg{err: fmt.Errorf("theme name is required")}
		}

		data, err := json.MarshalIndent(tj, "", "  ")
		if err != nil {
			return customThemeSavedMsg{err: err}
		}

		t, err := ThemeFromJSON(data)
		if err != nil {
			return customThemeSavedMsg{err: fmt.Errorf("invalid theme: %w", err)}
		}

		dir := filepath.Join(configDir, "themes")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return customThemeSavedMsg{err: err}
		}

		fname := sanitizeFilename(tj.Name) + ".json"
		if err := os.WriteFile(filepath.Join(dir, fname), data, 0o644); err != nil {
			return customThemeSavedMsg{err: err}
		}

		return customThemeSavedMsg{themeID: t.name, theme: t}
	}
}

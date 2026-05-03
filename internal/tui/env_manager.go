package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/d0mkaaa/gopull/internal/store"
)

type EnvEditorModel struct {
	id          string
	nameInput   textinput.Model
	dotenvInput textinput.Model
	varsInput   textarea.Model
	field       int
}

func newEnvEditor() EnvEditorModel {
	name := textinput.New()
	name.Placeholder = "Environment name"
	name.CharLimit = 120

	dotenv := textinput.New()
	dotenv.Placeholder = "optional .env path"
	dotenv.CharLimit = 1024

	vars := textarea.New()
	vars.Placeholder = "BASE_URL=https://api.example.com\nsecret TOKEN=value\n# DISABLED=value"
	vars.ShowLineNumbers = false
	vars.Prompt = ""

	m := EnvEditorModel{
		nameInput:   name,
		dotenvInput: dotenv,
		varsInput:   vars,
	}
	return m.refreshTheme()
}

func (m EnvEditorModel) refreshTheme() EnvEditorModel {
	applyTextinputTheme(&m.nameInput)
	applyTextinputTheme(&m.dotenvInput)
	applyTextareaTheme(&m.varsInput)
	return m
}

func (m EnvEditorModel) Load(e *store.Environment) EnvEditorModel {
	m = newEnvEditor()
	if e != nil {
		m.id = e.ID
		m.nameInput.SetValue(e.Name)
		m.dotenvInput.SetValue(e.DotenvPath)
		m.varsInput.SetValue(renderEnvVars(e.Variables))
	}
	m.field = 0
	return m.focusField()
}

func (m EnvEditorModel) SetSize(w, h int) EnvEditorModel {
	inner := max(20, w-6)
	m.nameInput.Width = inner
	m.dotenvInput.Width = inner
	m.varsInput.SetWidth(inner)
	m.varsInput.SetHeight(max(4, h-12))
	return m
}

func (m EnvEditorModel) Update(msg tea.Msg) (EnvEditorModel, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.Type {
		case tea.KeyTab:
			m.field = (m.field + 1) % 3
			return m.focusField(), nil
		case tea.KeyShiftTab:
			m.field = (m.field + 2) % 3
			return m.focusField(), nil
		case tea.KeyUp:
			if m.field > 0 {
				m.field--
				return m.focusField(), nil
			}
		case tea.KeyDown:
			if m.field < 2 {
				m.field++
				return m.focusField(), nil
			}
		}
	}

	var cmd tea.Cmd
	switch m.field {
	case 0:
		m.nameInput, cmd = m.nameInput.Update(msg)
	case 1:
		m.dotenvInput, cmd = m.dotenvInput.Update(msg)
	default:
		m.varsInput, cmd = m.varsInput.Update(msg)
	}
	return m, cmd
}

func (m EnvEditorModel) focusField() EnvEditorModel {
	m.nameInput.Blur()
	m.dotenvInput.Blur()
	m.varsInput.Blur()
	switch m.field {
	case 0:
		m.nameInput.Focus()
	case 1:
		m.dotenvInput.Focus()
	default:
		m.varsInput.Focus()
	}
	return m
}

func (m EnvEditorModel) Build() *store.Environment {
	name := strings.TrimSpace(m.nameInput.Value())
	if name == "" {
		name = "New environment"
	}
	return &store.Environment{
		ID:         m.id,
		Name:       name,
		DotenvPath: strings.TrimSpace(m.dotenvInput.Value()),
		Variables:  parseEnvVarLines(m.varsInput.Value()),
	}
}

func (m EnvEditorModel) View() string {
	return lipgloss.JoinVertical(lipgloss.Left,
		sidebarTitle.Render("edit environment"),
		"",
		hint.Render("name"),
		m.nameInput.View(),
		"",
		hint.Render("dotenv path"),
		m.dotenvInput.View(),
		"",
		hint.Render("variables"),
		m.varsInput.View(),
		"",
		hint.Render("  tab field   ctrl+s save   esc cancel"),
	)
}

func parseEnvVarLines(text string) []store.EnvVar {
	var vars []store.EnvVar
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		enabled := true
		if strings.HasPrefix(line, "#") {
			enabled = false
			line = strings.TrimSpace(strings.TrimPrefix(line, "#"))
		}
		secret := false
		if strings.HasPrefix(strings.ToLower(line), "secret ") {
			secret = true
			line = strings.TrimSpace(line[len("secret "):])
		}
		idx := strings.IndexByte(line, '=')
		if idx <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(line[idx+1:])
		if key == "" {
			continue
		}
		vars = append(vars, store.EnvVar{
			Key:     key,
			Value:   value,
			Enabled: enabled,
			Secret:  secret,
		})
	}
	return vars
}

func renderEnvVars(vars []store.EnvVar) string {
	lines := make([]string, 0, len(vars))
	for _, v := range vars {
		if v.Key == "" {
			continue
		}
		prefix := ""
		if !v.Enabled {
			prefix = "# "
		}
		if v.Secret {
			prefix += "secret "
		}
		lines = append(lines, prefix+v.Key+"="+v.Value)
	}
	return strings.Join(lines, "\n")
}

func envSummary(e *store.Environment) string {
	var parts []string
	if e.DotenvPath != "" {
		parts = append(parts, "dotenv "+e.DotenvPath)
	}
	for _, v := range e.Variables {
		if !v.Enabled || v.Key == "" {
			continue
		}
		val := v.Value
		if v.Secret {
			val = "****"
		}
		parts = append(parts, v.Key+"="+val)
		if len(parts) >= 3 {
			break
		}
	}
	if len(parts) == 0 {
		return fmt.Sprintf("%d variables", len(e.Variables))
	}
	return strings.Join(parts, "  ")
}

func (m Model) startEnvEdit(e *store.Environment) (tea.Model, tea.Cmd) {
	m.pendingEnvDeleteID = ""
	m.envEditing = true
	m.envEditor = m.envEditor.Load(e).SetSize(72, m.height-4)
	return m, textinput.Blink
}

func (m Model) updateEnvEditor(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.Type == tea.KeyEsc:
		m.envEditing = false
		return m, nil
	case keyMatchesSave(msg):
		env := m.envEditor.Build()
		m.envEditing = false
		return m, saveEnvironmentCmd(m.store, env)
	}
	var cmd tea.Cmd
	m.envEditor, cmd = m.envEditor.Update(msg)
	return m, cmd
}

func keyMatchesSave(msg tea.KeyMsg) bool {
	return msg.String() == "ctrl+s"
}

func (m Model) viewEnvManager() string {
	if m.envEditing {
		return m.envEditor.SetSize(72, m.height-4).View()
	}
	body := m.envPicker.View()
	return lipgloss.JoinVertical(lipgloss.Left,
		sidebarTitle.Render("environments"),
		body,
		hint.Render("  enter select   n new   e edit   d delete   esc close"),
	)
}

func selectedEnv(m Model) *store.Environment {
	if item := m.envPicker.SelectedItem(); item != nil {
		if ei, ok := item.(envItem); ok {
			return ei.e
		}
	}
	return nil
}

func saveEnvironmentCmd(st *store.Store, env *store.Environment) tea.Cmd {
	return func() tea.Msg {
		if err := st.SaveEnvironment(env); err != nil {
			return environmentsUpdatedMsg{err: err}
		}
		envs, err := st.LoadEnvironments()
		if err != nil {
			return environmentsUpdatedMsg{err: err}
		}
		return environmentsUpdatedMsg{envs: envs, activeEnvID: env.ID, status: "environment saved"}
	}
}

func deleteEnvironmentCmd(st *store.Store, envID, activeEnvID string) tea.Cmd {
	return func() tea.Msg {
		if err := st.DeleteEnvironment(envID); err != nil {
			return environmentsUpdatedMsg{err: err}
		}
		envs, err := st.LoadEnvironments()
		if err != nil {
			return environmentsUpdatedMsg{err: err}
		}
		nextActive := activeEnvID
		if envID == activeEnvID {
			nextActive = ""
		}
		return environmentsUpdatedMsg{envs: envs, activeEnvID: nextActive, status: "environment deleted"}
	}
}

func envListFiltering(l list.Model) bool {
	return l.FilterState() == list.Filtering
}

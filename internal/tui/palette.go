package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/d0mkaaa/gopull/internal/store"
)

type paletteItem struct {
	label  string
	detail string
	action string
	req    *store.Request
	collID string
}

type PaletteModel struct {
	input    textinput.Model
	all      []paletteItem
	filtered []paletteItem
	idx      int
}

var staticActions = []paletteItem{
	{label: "Send Request", detail: "ctrl+r", action: "send"},
	{label: "Save Request", detail: "ctrl+s", action: "save"},
	{label: "New Request", detail: "alt+n", action: "new"},
	{label: "Toggle Sidebar", detail: "alt+b", action: "sidebar"},
	{label: "Settings", detail: "alt+o", action: "settings"},
	{label: "Environment Picker", detail: "ctrl+e", action: "env"},
	{label: "Import Collection", detail: "ctrl+i  (.json or .http)", action: "import"},
	{label: "Export Collection", detail: "ctrl+x  Postman JSON", action: "export"},
	{label: "Export as .http", detail: "VS Code REST Client format", action: "export_http"},
	{label: "Copy Request as curl", detail: "alt+c", action: "curl_export"},
	{label: "Open Body in Editor", detail: "alt+e", action: "external_editor"},
	{label: "Quit", detail: "alt+q", action: "quit"},
}

func newPalette(cols []*store.Collection) PaletteModel {
	ti := textinput.New()
	ti.Placeholder = "type to filter..."
	ti.CharLimit = 100
	ti.Focus()

	all := make([]paletteItem, len(staticActions))
	copy(all, staticActions)

	for _, c := range cols {
		for _, id := range c.Order {
			r, ok := c.Requests[id]
			if !ok {
				continue
			}
			all = append(all, paletteItem{
				label:  c.Name + " / " + r.Name,
				detail: r.Method + " " + r.URL,
				action: "load",
				req:    r,
				collID: c.ID,
			})
		}
		for id, r := range c.Requests {
			inOrder := false
			for _, oid := range c.Order {
				if oid == id {
					inOrder = true
					break
				}
			}
			if !inOrder {
				all = append(all, paletteItem{
					label:  c.Name + " / " + r.Name,
					detail: r.Method + " " + r.URL,
					action: "load",
					req:    r,
					collID: c.ID,
				})
			}
		}
	}

	m := PaletteModel{input: ti, all: all}
	m.filter("")
	return m
}

func (m *PaletteModel) filter(q string) {
	q = strings.ToLower(q)
	m.filtered = nil
	for _, item := range m.all {
		if q == "" || strings.Contains(strings.ToLower(item.label), q) || strings.Contains(strings.ToLower(item.detail), q) {
			m.filtered = append(m.filtered, item)
			if len(m.filtered) >= 15 {
				break
			}
		}
	}
	if m.idx >= len(m.filtered) {
		m.idx = 0
	}
}

func (m PaletteModel) Update(msg tea.KeyMsg) (PaletteModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		return m, nil
	case "enter":
		if len(m.filtered) > 0 {
			item := m.filtered[m.idx]
			return m, func() tea.Msg {
				return paletteExecMsg{action: item.action, req: item.req, collID: item.collID}
			}
		}
		return m, nil
	case "ctrl+n", "down":
		if m.idx < len(m.filtered)-1 {
			m.idx++
		}
		return m, nil
	case "ctrl+p", "up":
		if m.idx > 0 {
			m.idx--
		}
		return m, nil
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	q := m.input.Value()
	m.filter(q)
	return m, cmd
}

func (m PaletteModel) View() string {
	title := sidebarTitle.Render("command palette")

	rows := make([]string, len(m.filtered))
	for i, item := range m.filtered {
		sel := "  "
		label := item.label
		detail := item.detail
		if i == m.idx {
			sel = tabActive.Render(">") + " "
			label = lipgloss.NewStyle().Foreground(colorAccent).Render(label)
		} else {
			sel += " "
		}
		rows[i] = sel + label + "  " + hint.Render(detail)
	}

	list := strings.Join(rows, "\n")
	if list == "" {
		list = hint.Render("no matches")
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		m.input.View(),
		"",
		list,
		"",
		hint.Render("  ↑↓ navigate   enter run   esc close"),
	)
}

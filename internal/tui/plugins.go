package tui

import (
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/d0mkaaa/gopull/internal/plugins"
)

type PluginManagerModel struct {
	infos  []plugins.Info
	idx    int
	width  int
	height int
}

func newPluginManager(infos []plugins.Info) PluginManagerModel {
	return PluginManagerModel{infos: infos}
}

func (m PluginManagerModel) SetSize(w, h int) PluginManagerModel {
	m.width = w
	m.height = h
	return m
}

func (m PluginManagerModel) Selected() (plugins.Info, bool) {
	if len(m.infos) == 0 || m.idx < 0 || m.idx >= len(m.infos) {
		return plugins.Info{}, false
	}
	return m.infos[m.idx], true
}

func (m PluginManagerModel) Update(msg tea.KeyMsg) (PluginManagerModel, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.idx > 0 {
			m.idx--
		}
	case "down", "j":
		if m.idx < len(m.infos)-1 {
			m.idx++
		}
	}
	return m, nil
}

func (m PluginManagerModel) View() string {
	title := sidebarTitle.Render("plugins")
	if len(m.infos) == 0 {
		return lipgloss.JoinVertical(lipgloss.Left,
			title,
			"",
			hint.Render("no local plugins found"),
			"",
			hint.Render("  r reload   esc close"),
		)
	}
	rows := make([]string, 0, len(m.infos)+6)
	nameWidth := max(18, min(34, m.width/2))
	for i, info := range m.infos {
		state := statusErr.Render(" off ")
		if info.Enabled {
			state = statusOK.Render(" on  ")
		}
		name := info.Name
		if name == "" {
			name = filepath.Base(info.Path)
		}
		line := state + "  " + fillLine(clipText(name, nameWidth), nameWidth) + "  " + hint.Render(clipText(strings.Join(info.Hooks, ", "), max(10, m.width-nameWidth-14)))
		if i == m.idx {
			line = tabActive.Render(">") + " " + line
		} else {
			line = "  " + line
		}
		rows = append(rows, line)
	}
	if info, ok := m.Selected(); ok {
		rows = append(rows,
			"",
			hint.Render("path ")+clipText(info.Path, max(20, m.width-8)),
			hint.Render("api  ")+valueOr(info.APIVersion, "legacy"),
			hint.Render("perms ")+clipText(valueOr(strings.Join(info.Permissions, ", "), "none"), max(20, m.width-8)),
		)
	}
	rows = append(rows, "", hint.Render("  up/down select   space enable/disable   r reload   esc close"))
	return lipgloss.JoinVertical(lipgloss.Left, append([]string{title, ""}, rows...)...)
}

func valueOr(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

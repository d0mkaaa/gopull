package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func viewCheatsheet(w, h int) string {
	type entry struct {
		key    string
		action string
	}
	type section struct {
		title   string
		entries []entry
	}

	sections := []section{
		{
			title: "global",
			entries: []entry{
				{"ctrl+r", "send request"},
				{"ctrl+s", "save request"},
				{"alt+n", "new request"},
				{"alt+p", "command palette"},
				{"alt+b", "toggle sidebar"},
				{"alt+o", "settings"},
				{"ctrl+e", "environment manager"},
				{"alt+h", "history browser"},
				{"ctrl+i", "import  (.json / .http / OpenAPI)"},
				{"ctrl+x", "export collection  (Postman JSON)"},
				{"alt+q", "quit"},
				{"?", "this cheatsheet"},
			},
		},
		{
			title: "editor",
			entries: []entry{
				{"[  /  ]", "switch tabs  (body headers auth tests opts)"},
				{"tab / shift+tab", "move focus between panels"},
				{"up/down / space", "cycle standard methods"},
				{"letter key", "type custom method  (PROPFIND, REPORT...)"},
				{"esc", "cancel custom method"},
				{"alt+m", "toggle body mode  (raw / form / graphql)"},
				{"alt+j", "format JSON body"},
				{"alt+c", "copy request as curl"},
				{"alt+e", "open body in external editor"},
			},
		},
		{
			title: "response",
			entries: []entry{
				{"j / k", "scroll down / up"},
				{"g / G", "jump to top / bottom"},
				{"ctrl+d / ctrl+u", "half-page down / up"},
				{"t", "toggle JSON tree view"},
				{"space", "tree: expand / collapse node"},
				{"c / e", "tree: collapse all / expand all"},
				{"{ / }", "tree: prev / next sibling"},
				{"/", "search body"},
				{"n / N", "next / prev search match"},
				{"v", "visual line select"},
				{"y", "copy body to clipboard"},
				{"w", "save body to file"},
				{"D", "diff against history"},
				{"[  /  ]", "switch tabs  (body headers tests)"},
			},
		},
		{
			title: "sidebar",
			entries: []entry{
				{"enter", "open collection or request"},
				{"r", "run all requests in collection"},
				{"n", "rename selected item"},
				{"ctrl+d", "duplicate request"},
				{"ctrl+j / ctrl+k", "move request down / up"},
				{"d  d", "delete  (press twice to confirm)"},
				{"esc", "back to collections"},
				{"tab", "move focus to editor"},
			},
		},
		{
			title: "assertions  (tests tab)",
			entries: []entry{
				{"assert status == 200", ""},
				{"assert header Content-Type == application/json", ""},
				{"assert body contains \"token\"", ""},
				{"assert jsonpath $.data.id > 0", ""},
				{"assert response_time < 500", "ms"},
				{"set TOKEN = $.data.access_token", "extract to env var"},
			},
		},
	}

	keyW := 26
	accentSt := lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	mutedSt := lipgloss.NewStyle().Foreground(colorMuted)
	subtleSt := lipgloss.NewStyle().Foreground(colorSubtle)

	col1 := strings.Builder{}
	col2 := strings.Builder{}

	half := len(sections) / 2
	renderSection := func(sb *strings.Builder, sec section) {
		sb.WriteString(accentSt.Render(sec.title) + "\n")
		for _, e := range sec.entries {
			if e.action == "" {
				sb.WriteString(mutedSt.Render("  "+e.key) + "\n")
			} else {
				k := subtleSt.Width(keyW).Render(e.key)
				sb.WriteString("  " + k + "  " + hint.Render(e.action) + "\n")
			}
		}
		sb.WriteString("\n")
	}

	for i, sec := range sections {
		if i <= half {
			renderSection(&col1, sec)
		} else {
			renderSection(&col2, sec)
		}
	}

	colW := (w - 6) / 2
	left := lipgloss.NewStyle().Width(colW).Render(col1.String())
	right := lipgloss.NewStyle().Width(colW).Render(col2.String())
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, "   ", right)

	return lipgloss.JoinVertical(lipgloss.Left,
		sidebarTitle.Render("keybindings"),
		"",
		body,
		"",
		hint.Render("  any key to close"),
	)
}

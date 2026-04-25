package tui

import (
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

var (
	colorBg      lipgloss.Color
	colorAccent  lipgloss.Color
	colorMuted   lipgloss.Color
	colorBorder  lipgloss.Color
	colorSuccess lipgloss.Color
	colorWarn    lipgloss.Color
	colorError   lipgloss.Color
	colorSubtle  lipgloss.Color
	colorText    lipgloss.Color
	colorBadgeFg lipgloss.Color
)

var (
	pane         lipgloss.Style
	paneActive   lipgloss.Style
	statusOK     lipgloss.Style
	statusWarn   lipgloss.Style
	statusErr    lipgloss.Style
	hint         lipgloss.Style
	tabActive    lipgloss.Style
	tabFocused   lipgloss.Style
	tabInactive  lipgloss.Style
	sidebarTitle lipgloss.Style
	sidebarBack  lipgloss.Style
	envBadge     lipgloss.Style
	statusBar    lipgloss.Style
	testPass     lipgloss.Style
	testFail     lipgloss.Style
	testSet      lipgloss.Style
	searchMatch  lipgloss.Style
	formMode     lipgloss.Style
)

var methodColors map[string]lipgloss.Color

func init() {
	applyTheme(darkTheme)
}

func methodBadge(m string) string {
	c, ok := methodColors[strings.ToUpper(m)]
	if !ok {
		c = colorAccent
	}
	return lipgloss.NewStyle().
		Foreground(colorBadgeFg).
		Background(c).
		Padding(0, 1).
		Bold(true).
		Render(m)
}

func statusStyle(code int) lipgloss.Style {
	switch {
	case code >= 200 && code < 300:
		return statusOK
	case code >= 300 && code < 400:
		return statusWarn
	default:
		return statusErr
	}
}

// elapsedColor returns a style that colors elapsed time green/yellow/red
// based on how fast the response was.
func elapsedColor(d time.Duration) lipgloss.Style {
	switch {
	case d < 200*time.Millisecond:
		return lipgloss.NewStyle().Foreground(colorSuccess)
	case d < 1000*time.Millisecond:
		return lipgloss.NewStyle().Foreground(colorWarn)
	default:
		return lipgloss.NewStyle().Foreground(colorError)
	}
}

func renderTabs(names []string, active int, focused bool) string {
	sep := lipgloss.NewStyle().Foreground(colorMuted).Render(" · ")
	parts := make([]string, len(names))
	for i, name := range names {
		switch {
		case i == active && focused:
			parts[i] = tabActive.Render(name)
		case i == active:
			parts[i] = tabFocused.Render(name)
		default:
			parts[i] = tabInactive.Render(name)
		}
	}
	return strings.Join(parts, sep)
}

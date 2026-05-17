package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/d0mkaaa/gopull/internal/store"
)

func TestViewFitsTerminalBounds(t *testing.T) {
	st, err := store.NewAt(t.TempDir())
	if err != nil {
		t.Fatalf("NewAt: %v", err)
	}

	for _, tc := range []struct {
		name string
		w    int
		h    int
	}{
		{"narrow", 72, 20},
		{"normal", 120, 28},
		{"wide", 180, 40},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := New(st, "test")
			m.width = tc.w
			m.height = tc.h
			m.welcomeVisible = false
			m.theme = "paper"
			applyTheme(themeRegistry[m.theme])
			m.relayout()
			m.response.result = &result{
				status:      "200 OK",
				code:        200,
				elapsed:     930 * time.Millisecond,
				body:        strings.Repeat("response ", 60),
				plainBody:   strings.Repeat("response ", 60),
				rawHeaders:  "Set-Cookie: " + strings.Repeat("abcdef", 80),
				contentType: "text/html",
				size:        78_300,
			}
			m.response.tab = rtHeaders
			m.response = m.response.refreshViewport()

			assertFrameFits(t, m.View(), tc.w, tc.h)
			assertHintPinned(t, m.View())
			assertMainPanesHaveEqualHeight(t, m)
			assertNoWrappedRules(t, m.View())
			assertBodyUsesFullWidth(t, m.View(), tc.w)
		})
	}
}

func assertFrameFits(t *testing.T, out string, w, h int) {
	t.Helper()
	out = stripOSC11(out)
	lines := strings.Split(out, "\n")
	if len(lines) > h {
		t.Fatalf("frame height = %d, want <= %d", len(lines), h)
	}
	for i, line := range lines {
		if got := lipgloss.Width(line); got > w {
			t.Fatalf("line %d width = %d, want <= %d: %q", i+1, got, w, line)
		}
	}
}

func assertMainPanesHaveEqualHeight(t *testing.T, m Model) {
	t.Helper()
	want := max(1, m.height-2)
	panes := []struct {
		name string
		view string
	}{
		{"sidebar", m.paneFrameStyle(pane, m.sidebarPaneWidth()).Render(m.sidebar.View())},
		{"editor", m.paneFrameStyle(pane, m.editorPaneWidth()).Render(m.editor.View())},
		{"response", m.paneFrameStyle(pane, m.responsePaneWidth()).Render(m.response.View())},
	}
	for _, p := range panes {
		if got := lipgloss.Height(p.view); got != want {
			t.Fatalf("%s pane height = %d, want %d", p.name, got, want)
		}
	}
}

func assertNoWrappedRules(t *testing.T, out string) {
	t.Helper()
	for i, line := range strings.Split(stripOSC11(out), "\n") {
		if strings.TrimSpace(line) == "--" {
			t.Fatalf("wrapped rule leaked onto line %d", i+1)
		}
	}
}

func assertBodyUsesFullWidth(t *testing.T, out string, w int) {
	t.Helper()
	lines := strings.Split(stripOSC11(out), "\n")
	if len(lines) == 0 {
		t.Fatal("empty frame")
	}
	if got := lipgloss.Width(lines[0]); got != w {
		t.Fatalf("body width = %d, want %d", got, w)
	}
}

func assertHintPinned(t *testing.T, out string) {
	t.Helper()
	out = stripOSC11(out)
	lines := strings.Split(out, "\n")
	if len(lines) < 2 {
		t.Fatalf("frame has no room for hint: %q", out)
	}
	bottom := strings.Join(lines[len(lines)-2:], "\n")
	if !strings.Contains(bottom, "no env") || !strings.Contains(bottom, "alt+q quit") {
		t.Fatalf("hint bar was not pinned at bottom:\n%s", bottom)
	}
}

func stripOSC11(s string) string {
	if !strings.HasPrefix(s, "\x1b]11;") {
		return s
	}
	if i := strings.IndexByte(s, '\a'); i >= 0 {
		return s[i+1:]
	}
	return s
}

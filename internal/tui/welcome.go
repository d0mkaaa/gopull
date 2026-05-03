package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const welcomeSteps = 3

func viewWelcome(step, w, h int) string {
	var content string
	switch step {
	case 0:
		content = welcomeStep0()
	case 1:
		content = welcomeStep1()
	case 2:
		content = welcomeStep2()
	}

	accent := lipgloss.NewStyle().Foreground(colorAccent).Faint(true)
	progress := accent.Render(fmt.Sprintf("step %d of %d", step+1, welcomeSteps))

	var hint string
	if step < welcomeSteps-1 {
		hint = lipgloss.NewStyle().Foreground(colorAccent).Faint(true).Render("press any key to continue  ->")
	} else {
		hint = lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render("press any key to start")
	}

	body := content + "\n\n" + hint

	box := paneActive.
		Width(w-20).
		Padding(2, 4).
		Render(body)

	boxWithProgress := lipgloss.JoinVertical(lipgloss.Right,
		progress,
		box,
	)

	return lipgloss.Place(w, h,
		lipgloss.Center, lipgloss.Center,
		boxWithProgress,
	)
}

func welcomeStep0() string {
	accent := lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	muted := lipgloss.NewStyle().Foreground(colorMuted)

	lines := []string{
		accent.Render("gopull"),
		muted.Render("HTTP requests, without leaving the terminal."),
		"",
		muted.Render("Three panels work together:"),
		"",
		"  " + accent.Render("sidebar") + muted.Render("   - your saved requests and collections"),
		"  " + accent.Render("editor") + muted.Render("    - build the request: method, URL, headers, body"),
		"  " + accent.Render("response") + muted.Render("  - status, timing, pretty-printed body"),
		"",
		muted.Render("Tab moves focus between them."),
	}
	return strings.Join(lines, "\n")
}

func welcomeStep1() string {
	accent := lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	muted := lipgloss.NewStyle().Foreground(colorMuted)
	key := lipgloss.NewStyle().Foreground(colorText).Bold(true)

	lines := []string{
		accent.Render("sending requests"),
		"",
		muted.Render("Type a URL in the editor and press ") + key.Render("ctrl+r") + muted.Render(" to send."),
		"",
		muted.Render("Paste any ") + key.Render("curl") + muted.Render(" command into the URL field"),
		muted.Render("and press ") + key.Render("tab") + muted.Render(" - it imports automatically."),
		muted.Render("Chrome DevTools, Postman, OpenAI docs: all work."),
		"",
		muted.Render("Use ") + key.Render("{{VARIABLE}}") + muted.Render(" anywhere in a request."),
		muted.Render("Switch environments with ") + key.Render("ctrl+e") + muted.Render("."),
	}
	return strings.Join(lines, "\n")
}

func welcomeStep2() string {
	accent := lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	muted := lipgloss.NewStyle().Foreground(colorMuted)
	key := lipgloss.NewStyle().Foreground(colorText).Bold(true)
	desc := lipgloss.NewStyle().Foreground(colorMuted)

	row := func(k, d string) string {
		return "  " + key.Render(pad(k, 12)) + desc.Render(d)
	}

	lines := []string{
		accent.Render("keys"),
		"",
		row("ctrl+r", "send request"),
		row("ctrl+s", "save to collection"),
		row("alt+n", "new request"),
		row("ctrl+e", "switch environment"),
		row("alt+p", "command palette"),
		row("[ / ]", "switch tabs"),
		row("j / k", "scroll response"),
		row("/", "search response"),
		row("y", "copy body"),
		row("D", "diff against history"),
		row("alt+q", "quit"),
		"",
		muted.Render("If you get lost, alt+p always has you covered."),
	}
	return strings.Join(lines, "\n")
}

func pad(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
}

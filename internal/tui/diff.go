package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/d0mkaaa/gopull/internal/store"
)

type DiffModel struct {
	history     []store.HistoryEntry
	idx         int
	currentBody string
	viewport    viewport.Model
	picking     bool
}

func newDiff(currentBody string, entries []store.HistoryEntry) DiffModel {
	vp := viewport.New(80, 20)
	return DiffModel{
		history:     entries,
		currentBody: currentBody,
		viewport:    vp,
		picking:     true,
	}
}

func (m DiffModel) SetSize(w, h int) DiffModel {
	m.viewport.Width = w - 4
	m.viewport.Height = max(3, h-6)
	return m
}

func (m DiffModel) Update(msg tea.KeyMsg) (DiffModel, tea.Cmd) {
	if m.picking {
		switch msg.String() {
		case "j", "down":
			if m.idx < len(m.history)-1 {
				m.idx++
			}
		case "k", "up":
			if m.idx > 0 {
				m.idx--
			}
		case "enter":
			if len(m.history) > 0 {
				other := m.history[m.idx].Response.Body
				diff := unifiedDiff(other, m.currentBody)
				m.viewport.SetContent(diff)
				m.viewport.GotoTop()
				m.picking = false
			}
		case "esc":
			return m, func() tea.Msg { return focusResponseMsg{} }
		}
		return m, nil
	}

	switch msg.String() {
	case "j", "down":
		m.viewport.LineDown(1)
	case "k", "up":
		m.viewport.LineUp(1)
	case "esc":
		m.picking = true
	}
	return m, nil
}

func (m DiffModel) View() string {
	title := sidebarTitle.Render("response diff")

	if len(m.history) == 0 {
		return lipglossJoin(title, "", hint.Render("no history entries"))
	}

	if m.picking {
		lines := make([]string, len(m.history))
		for i, e := range m.history {
			prefix := "  "
			if i == m.idx {
				prefix = tabActive.Render(">")
				prefix += " "
			} else {
				prefix += " "
			}
			ts := e.Timestamp.Format("2006-01-02 15:04:05")
			lines[i] = prefix + hint.Render(fmt.Sprintf("%-6s %-40s  %s  %d",
				e.Request.Method, truncate(e.Request.URL, 40), ts, e.Response.StatusCode))
		}
		return lipglossJoin(title,
			hint.Render("select history entry to diff against"),
			"",
			strings.Join(lines, "\n"),
			"",
			hint.Render("  j/k navigate   enter diff   esc close"),
		)
	}

	return lipglossJoin(title,
		m.viewport.View(),
		"",
		hint.Render("  j/k scroll   esc back to list"),
	)
}

func lipglossJoin(parts ...string) string {
	return strings.Join(parts, "\n")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}

func unifiedDiff(oldText, newText string) string {
	oldLines := strings.Split(oldText, "\n")
	newLines := strings.Split(newText, "\n")

	lcs := computeLCS(oldLines, newLines)
	var sb strings.Builder

	type hunk struct {
		oldStart, oldLen, newStart, newLen int
		lines                              []string
	}

	oi, ni, li := 0, 0, 0
	for oi < len(oldLines) || ni < len(newLines) {
		if li < len(lcs) {
			oldLCS := lcs[li][0]
			newLCS := lcs[li][1]
			for oi < oldLCS {
				sb.WriteString(testFail.Render("-") + " " + oldLines[oi] + "\n")
				oi++
			}
			for ni < newLCS {
				sb.WriteString(testPass.Render("+") + " " + newLines[ni] + "\n")
				ni++
			}
			sb.WriteString(hint.Render(" ") + " " + oldLines[oi] + "\n")
			oi++
			ni++
			li++
		} else {
			for oi < len(oldLines) {
				sb.WriteString(testFail.Render("-") + " " + oldLines[oi] + "\n")
				oi++
			}
			for ni < len(newLines) {
				sb.WriteString(testPass.Render("+") + " " + newLines[ni] + "\n")
				ni++
			}
		}
	}

	result := strings.TrimRight(sb.String(), "\n")
	if result == "" {
		return hint.Render("no differences")
	}
	return result
}

// computeLCS returns the common subsequence as pairs of [oldIdx, newIdx].
func computeLCS(a, b []string) [][2]int {
	m, n := len(a), len(b)
	if m == 0 || n == 0 {
		return nil
	}

	// limit size to avoid O(mn) blowup on huge responses
	if m > 500 {
		m = 500
		a = a[:m]
	}
	if n > 500 {
		n = 500
		b = b[:n]
	}

	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}
	for i := m - 1; i >= 0; i-- {
		for j := n - 1; j >= 0; j-- {
			if a[i] == b[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] > dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}

	var result [][2]int
	i, j := 0, 0
	for i < m && j < n {
		if a[i] == b[j] {
			result = append(result, [2]int{i, j})
			i++
			j++
		} else if dp[i+1][j] >= dp[i][j+1] {
			i++
		} else {
			j++
		}
	}
	return result
}

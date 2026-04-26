package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"

	"github.com/d0mkaaa/gopull/internal/store"
)

type runnerResult struct {
	name    string
	done    bool
	code    int
	elapsed time.Duration
	pass    int
	fail    int
	err     string
}

type RunnerModel struct {
	collection *store.Collection
	order      []string
	results    []runnerResult
	running    bool
	done       bool
	viewport   viewport.Model
}

func newRunner(c *store.Collection) RunnerModel {
	order := make([]string, 0, len(c.Order))
	for _, id := range c.Order {
		if _, ok := c.Requests[id]; ok {
			order = append(order, id)
		}
	}
	results := make([]runnerResult, len(order))
	for i, id := range order {
		r := c.Requests[id]
		name := id
		if r != nil {
			name = r.Name
		}
		results[i] = runnerResult{name: name}
	}
	vp := viewport.New(60, 20)
	return RunnerModel{
		collection: c,
		order:      order,
		results:    results,
		viewport:   vp,
	}
}

func (m RunnerModel) SetSize(w, h int) RunnerModel {
	m.viewport.Width = w - 4
	m.viewport.Height = max(3, h-4)
	return m
}

func (m RunnerModel) SetResult(idx int, r runnerResult) RunnerModel {
	if idx < 0 || idx >= len(m.results) {
		return m
	}
	m.results[idx] = r
	m.viewport.SetContent(m.renderResults())
	m.viewport.GotoBottom()
	return m
}

func (m RunnerModel) renderResults() string {
	var sb strings.Builder
	for i, r := range m.results {
		var icon string
		switch {
		case !r.done && i == m.currentRunningIdx():
			icon = hint.Render("↻")
		case !r.done:
			icon = hint.Render("·")
		case r.err != "":
			icon = testFail.Render("✗")
		case r.fail > 0:
			icon = testFail.Render("✗")
		default:
			icon = testPass.Render("✓")
		}

		line := icon + " " + r.name
		if r.done {
			if r.err != "" {
				line += hint.Render("  error: " + r.err)
			} else {
				line += hint.Render(fmt.Sprintf("  %d  %s", r.code, r.elapsed.Round(time.Millisecond)))
				if r.pass+r.fail > 0 {
					line += "  " + testPass.Render(fmt.Sprintf("%d pass", r.pass))
					if r.fail > 0 {
						line += "  " + testFail.Render(fmt.Sprintf("%d fail", r.fail))
					}
				}
			}
		}
		sb.WriteString(line + "\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

func (m RunnerModel) currentRunningIdx() int {
	for i, r := range m.results {
		if !r.done {
			return i
		}
	}
	return -1
}

func (m RunnerModel) View() string {
	name := ""
	if m.collection != nil {
		name = m.collection.Name
	}
	title := sidebarTitle.Render("runner: " + name)

	pass, fail, total := 0, 0, 0
	for _, r := range m.results {
		if r.done {
			total++
			pass += r.pass
			fail += r.fail
		}
	}

	var footer string
	if m.done {
		summary := testPass.Render(fmt.Sprintf("%d pass", pass))
		if fail > 0 {
			summary += "  " + testFail.Render(fmt.Sprintf("%d fail", fail))
		}
		footer = hint.Render(fmt.Sprintf("%d/%d done  ", total, len(m.results))) +
			summary + hint.Render("   esc close")
	} else {
		idx := m.currentRunningIdx()
		if idx >= 0 && idx < len(m.results) {
			footer = hint.Render("running  ") +
				lipgloss.NewStyle().Foreground(colorAccent).Render(m.results[idx].name) +
				hint.Render("...")
		} else {
			footer = hint.Render("running...")
		}
	}

	content := m.renderResults()
	if content == "" {
		content = hint.Render("no requests")
	}

	return strings.Join([]string{title, "", content, "", footer}, "\n")
}

package tui

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/d0mkaaa/gopull/internal/store"
)

type HistoryModel struct {
	input    textinput.Model
	entries  []store.HistoryEntry
	filtered []int
	idx      int
	width    int
	height   int
}

func newHistory(entries []store.HistoryEntry) HistoryModel {
	ti := textinput.New()
	ti.Placeholder = "filter history..."
	ti.CharLimit = 200
	ti.Focus()
	applyTextinputTheme(&ti)

	m := HistoryModel{input: ti, entries: entries, width: 96, height: 24}
	m.filter("")
	return m
}

func (m HistoryModel) SetSize(w, h int) HistoryModel {
	m.width = max(60, w)
	m.height = max(12, h)
	m.input.Width = max(20, m.width-8)
	return m
}

func (m *HistoryModel) filter(q string) {
	q = strings.ToLower(strings.TrimSpace(q))
	m.filtered = nil
	for i, e := range m.entries {
		if q == "" || historyMatches(e, q) {
			m.filtered = append(m.filtered, i)
		}
	}
	if m.idx >= len(m.filtered) {
		m.idx = 0
	}
}

func historyMatches(e store.HistoryEntry, q string) bool {
	fields := []string{
		e.Request.Method,
		e.Request.URL,
		strconv.Itoa(e.Response.StatusCode),
		e.Response.ContentType,
	}
	return strings.Contains(strings.ToLower(strings.Join(fields, " ")), q)
}

func (m HistoryModel) Update(msg tea.KeyMsg) (HistoryModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		return m, func() tea.Msg { return historyActionMsg{action: "close"} }
	case "enter":
		return m.historyAction("load")
	case "ctrl+r":
		return m.historyAction("replay")
	case "s":
		return m.historyAction("save")
	case "D":
		return m.historyAction("diff")
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
	m.filter(m.input.Value())
	return m, cmd
}

func (m HistoryModel) historyAction(action string) (HistoryModel, tea.Cmd) {
	if len(m.filtered) == 0 {
		return m, nil
	}
	entry := m.entries[m.filtered[m.idx]]
	return m, func() tea.Msg { return historyActionMsg{action: action, entry: entry} }
}

func (m HistoryModel) View() string {
	title := sidebarTitle.Render("history")
	if len(m.entries) == 0 {
		return lipgloss.JoinVertical(lipgloss.Left,
			title,
			"",
			hint.Render("no history yet"),
			"",
			hint.Render("  esc close"),
		)
	}

	maxRows := max(4, m.height-8)
	if maxRows > len(m.filtered) {
		maxRows = len(m.filtered)
	}
	start := 0
	if m.idx >= maxRows {
		start = m.idx - maxRows + 1
	}

	rows := make([]string, 0, maxRows)
	for pos := start; pos < start+maxRows && pos < len(m.filtered); pos++ {
		e := m.entries[m.filtered[pos]]
		prefix := "  "
		method := hint.Render(fmt.Sprintf("%-7s", e.Request.Method))
		if pos == m.idx {
			prefix = tabActive.Render(">") + " "
			method = lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render(fmt.Sprintf("%-7s", e.Request.Method))
		}
		ts := e.Timestamp.Format("01-02 15:04:05")
		status := statusStyle(e.Response.StatusCode).Render(strconv.Itoa(e.Response.StatusCode))
		elapsed := fmt.Sprintf("%dms", e.Response.ElapsedMs)
		urlText := truncate(e.Request.URL, max(20, m.width-48))
		rows = append(rows, prefix+method+" "+hint.Render(ts)+"  "+status+"  "+
			hint.Render(fmt.Sprintf("%7s", elapsed))+"  "+hint.Render(fmt.Sprintf("%7s", formatSize(e.Response.SizeBytes)))+"  "+urlText)
	}

	count := hint.Render(fmt.Sprintf("%d/%d", len(m.filtered), len(m.entries)))
	return lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		m.input.View()+"  "+count,
		"",
		strings.Join(rows, "\n"),
		"",
		hint.Render("  up/down navigate   enter load   ctrl+r replay   s save   D diff   esc close"),
	)
}

func requestFromHistory(e store.HistoryEntry) store.Request {
	bodyMode := e.Request.BodyMode
	if bodyMode == "" {
		bodyMode = "raw"
	}
	req := store.Request{
		Name:    historyRequestName(e),
		Method:  e.Request.Method,
		URL:     e.Request.URL,
		Body:    store.Body{Mode: bodyMode, Raw: e.Request.Body},
		Auth:    e.Request.Auth,
		Options: e.Request.Options,
		Tests:   e.Request.Tests,
	}
	if req.Method == "" {
		req.Method = "GET"
	}
	for k, v := range e.Request.Headers {
		req.Headers = append(req.Headers, store.Header{Key: k, Value: v, Enabled: true})
	}
	return req
}

func historyRequestName(e store.HistoryEntry) string {
	method := e.Request.Method
	if method == "" {
		method = "GET"
	}
	if u, err := url.Parse(e.Request.URL); err == nil && u.Path != "" && u.Path != "/" {
		return method + " " + u.Path
	}
	if e.Request.URL != "" {
		return method + " " + truncate(e.Request.URL, 50)
	}
	return method + " request"
}

func loadHistoryBrowserCmd(st *store.Store) tea.Cmd {
	return func() tea.Msg {
		h, err := st.LoadHistory()
		if err != nil {
			return historyBrowserLoadedMsg{err: err}
		}
		return historyBrowserLoadedMsg{entries: h.Entries}
	}
}

func saveHistoryRequestCmd(st *store.Store, collID string, entry store.HistoryEntry) tea.Cmd {
	return func() tea.Msg {
		req := requestFromHistory(entry)
		if collID == "" {
			c, err := st.EnsureDefaultCollection()
			if err != nil {
				return errMsg{err}
			}
			collID = c.ID
		}
		if err := st.SaveRequest(collID, &req); err != nil {
			return errMsg{err}
		}
		cols, err := st.LoadCollections()
		if err != nil {
			return errMsg{err}
		}
		return collectionsUpdatedMsg{cols: cols, status: "saved from history"}
	}
}

func (m Model) updateHistory(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.history, cmd = m.history.Update(msg)
	return m, cmd
}

func (m Model) handleHistoryAction(msg historyActionMsg) (tea.Model, tea.Cmd) {
	switch msg.action {
	case "close":
		m.historyVisible = false
		return m, nil
	case "load":
		req := requestFromHistory(msg.entry)
		m.historyVisible = false
		m.editor = m.editor.Load(&req, m.sidebar.activeCollID)
		m = m.focusPanel(rfEditor)
		return m, nil
	case "replay":
		req := requestFromHistory(msg.entry)
		m.historyVisible = false
		m.editor = m.editor.Load(&req, m.sidebar.activeCollID)
		m = m.focusPanel(rfEditor)
		return m.doSend()
	case "save":
		m.historyVisible = false
		return m, saveHistoryRequestCmd(m.store, m.sidebar.activeCollID, msg.entry)
	case "diff":
		m.historyVisible = false
		others := make([]store.HistoryEntry, 0, len(m.history.entries))
		for _, e := range m.history.entries {
			if e.ID != msg.entry.ID {
				others = append(others, e)
			}
		}
		m.diff = newDiff(msg.entry.Response.Body, others)
		m.diff = m.diff.SetSize(m.responsePaneWidth(), m.height-4)
		m.diffVisible = true
		return m, nil
	}
	return m, nil
}

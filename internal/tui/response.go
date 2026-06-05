package tui

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/d0mkaaa/gopull/internal/tests"
)

type respTab int

const (
	rtPreview respTab = iota
	rtBody
	rtHeaders
	rtTests
)

type TestRow struct {
	label  string
	pass   bool
	actual string
	isSet  bool
}

type result struct {
	status      string
	code        int
	elapsed     time.Duration
	body        string
	plainBody   string
	rawHeaders  string
	contentType string
	size        int
	binary      bool
}

type ResponseModel struct {
	viewport    viewport.Model
	searchInput textinput.Model
	searching   bool
	query       string

	matchPositions []int
	matchIndex     int

	// visual line selection (vim-style v + j/k + y)
	visualMode   bool
	visualAnchor int // viewport.YOffset when v was pressed

	tab      respTab
	result   *result
	err      error
	loading  bool
	tooLarge bool
	focused  bool

	jsonTree *jsonTreeState
	treeView bool

	testRows []TestRow

	streaming    bool
	streamLines  []string
	streamStart  time.Time
	streamCode   int
	streamStatus string
	streamHdrs   http.Header

	width  int
	height int
}

func newResponse(w, h int) ResponseModel {
	si := textinput.New()
	si.Placeholder = "search..."
	si.CharLimit = 100

	vp := viewport.New(w-4, max(3, h-5))
	return ResponseModel{
		viewport:    vp,
		searchInput: si,
		width:       w,
		height:      h,
	}
}

func (m ResponseModel) InVisualMode() bool { return m.visualMode }
func (m ResponseModel) InTreeMode() bool   { return m.treeView && m.jsonTree != nil }
func (m ResponseModel) HasJSONTree() bool  { return m.jsonTree != nil }

func (m ResponseModel) Update(msg tea.Msg) (ResponseModel, tea.Cmd) {
	// Exit visual mode on esc - highest priority, even before focused check.
	if km, ok := msg.(tea.KeyMsg); ok && m.visualMode && km.Type == tea.KeyEsc {
		m.visualMode = false
		m = m.refreshViewport()
		return m, nil
	}

	if m.searching {
		if km, ok := msg.(tea.KeyMsg); ok {
			switch km.Type {
			case tea.KeyEsc:
				m.searching = false
				m.query = ""
				m.searchInput.SetValue("")
				m.matchPositions = nil
				m.matchIndex = 0
				m = m.refreshViewport()
				return m, nil
			case tea.KeyEnter:
				m.searching = false
				m.searchInput.Blur()
				return m, nil
			}
		}
		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		q := m.searchInput.Value()
		if q != m.query {
			m.query = q
			m = m.applySearch(q)
		}
		return m, cmd
	}

	if !m.focused {
		return m, nil
	}

	if km, ok := msg.(tea.KeyMsg); ok {
		// tooLarge mode: intercept all keys, offer limited actions.
		if m.tooLarge && m.tab == rtBody {
			switch {
			case km.Type == tea.KeyEnter:
				m.tooLarge = false
				m = m.refreshViewport()
			case km.String() == "t" && m.jsonTree != nil:
				m.tooLarge = false
				m.treeView = true
				m = m.refreshViewportTree()
			case km.String() == "s" && m.result != nil:
				ct := m.result.contentType
				body := m.result.plainBody
				return m, func() tea.Msg { return saveResponseMsg{body: body, contentType: ct} }
			case km.String() == "y" && m.result != nil:
				copyToClipboard(m.result.plainBody)
			}
			return m, nil
		}

		switch {
		case key.Matches(km, keys.Search):
			if !m.visualMode && !m.treeView && m.tab != rtTests {
				m.searching = true
				m.searchInput.Focus()
				return m, textinput.Blink
			}
			return m, nil

		case km.Type == tea.KeyEsc && m.query != "":
			m.query = ""
			m.searchInput.SetValue("")
			m.matchPositions = nil
			m.matchIndex = 0
			m = m.refreshViewport()
			return m, nil

		case km.String() == "t" && m.jsonTree != nil && m.tab == rtBody:
			m.treeView = !m.treeView
			if m.treeView {
				m.visualMode = false
			}
			m = m.refreshViewport()
			return m, nil

		case km.Type == tea.KeySpace && m.treeView && m.jsonTree != nil:
			m.jsonTree.toggle()
			m = m.refreshViewportTree()
			return m, nil

		case km.String() == "c" && m.treeView && m.jsonTree != nil:
			m.jsonTree.collapseAll()
			m = m.refreshViewportTree()
			return m, nil

		case km.String() == "e" && m.treeView && m.jsonTree != nil:
			m.jsonTree.expandAll()
			m = m.refreshViewportTree()
			return m, nil

		case km.String() == "{" && m.treeView && m.jsonTree != nil:
			m.jsonTree.jumpPrevSibling()
			m = m.syncViewportToCursor()
			return m, nil

		case km.String() == "}" && m.treeView && m.jsonTree != nil:
			m.jsonTree.jumpNextSibling()
			m = m.syncViewportToCursor()
			return m, nil

		case km.String() == "v" && !m.treeView && (m.tab == rtBody || m.tab == rtHeaders):
			m.visualMode = !m.visualMode
			if m.visualMode {
				m.visualAnchor = m.viewport.YOffset
			}
			m = m.refreshViewport()
			return m, nil

		case km.String() == "n" && len(m.matchPositions) > 0:
			m.matchIndex = (m.matchIndex + 1) % len(m.matchPositions)
			m.viewport.YOffset = m.matchPositions[m.matchIndex]
			return m, nil

		case km.String() == "N" && len(m.matchPositions) > 0:
			m.matchIndex = (m.matchIndex + len(m.matchPositions) - 1) % len(m.matchPositions)
			m.viewport.YOffset = m.matchPositions[m.matchIndex]
			return m, nil

		case km.String() == "j":
			if m.treeView && m.jsonTree != nil {
				m.jsonTree.moveCursor(1)
				m = m.syncViewportToCursor()
			} else {
				m.viewport.ScrollDown(1)
				if m.visualMode {
					m = m.refreshViewportBody()
				}
			}
			return m, nil

		case km.String() == "k":
			if m.treeView && m.jsonTree != nil {
				m.jsonTree.moveCursor(-1)
				m = m.syncViewportToCursor()
			} else {
				m.viewport.ScrollUp(1)
				if m.visualMode {
					m = m.refreshViewportBody()
				}
			}
			return m, nil

		case km.String() == "g":
			if m.treeView && m.jsonTree != nil {
				m.jsonTree.cursor = 0
				m = m.syncViewportToCursor()
			} else {
				m.viewport.GotoTop()
				if m.visualMode {
					m = m.refreshViewportBody()
				}
			}
			return m, nil

		case km.String() == "G":
			if m.treeView && m.jsonTree != nil {
				m.jsonTree.cursor = len(m.jsonTree.flat) - 1
				m = m.syncViewportToCursor()
			} else {
				m.viewport.GotoBottom()
				if m.visualMode {
					m = m.refreshViewportBody()
				}
			}
			return m, nil

		case km.Type == tea.KeyCtrlD:
			if m.treeView && m.jsonTree != nil {
				m.jsonTree.moveCursor(m.viewport.Height / 2)
				m = m.syncViewportToCursor()
			} else {
				m.viewport.ScrollDown(m.viewport.Height / 2)
				if m.visualMode {
					m = m.refreshViewportBody()
				}
			}
			return m, nil

		case km.Type == tea.KeyCtrlU:
			if m.treeView && m.jsonTree != nil {
				m.jsonTree.moveCursor(-(m.viewport.Height / 2))
				m = m.syncViewportToCursor()
			} else {
				m.viewport.ScrollUp(m.viewport.Height / 2)
				if m.visualMode {
					m = m.refreshViewportBody()
				}
			}
			return m, nil

		case km.String() == "D" && m.result != nil && !m.result.binary:
			body := m.result.plainBody
			return m, func() tea.Msg { return openDiffMsg{currentBody: body} }

		case km.String() == "y" && m.result != nil:
			var content string
			if m.visualMode {
				content = m.visualSelection()
				m.visualMode = false
				m = m.refreshViewport()
			} else {
				switch m.tab {
				case rtPreview:
					content = m.result.plainBody
				case rtBody:
					content = m.result.plainBody
				case rtHeaders:
					content = m.result.rawHeaders
				}
			}
			if content != "" {
				copyToClipboard(content)
			}
			return m, nil

		case km.String() == "w" && m.result != nil && m.tab != rtTests:
			var content string
			switch m.tab {
			case rtPreview:
				content = m.result.plainBody
			case rtBody:
				content = m.result.plainBody
			case rtHeaders:
				content = m.result.rawHeaders
			}
			ct := m.result.contentType
			if m.tab == rtHeaders {
				ct = "text/plain"
			}
			return m, func() tea.Msg {
				return saveResponseMsg{body: content, contentType: ct}
			}

		case key.Matches(km, keys.TabLeft):
			if m.tab > rtPreview {
				m.visualMode = false
				m = m.setTab(m.tab - 1)
			}
			return m, nil

		case key.Matches(km, keys.TabRight):
			if m.tab < m.lastTab() {
				m.visualMode = false
				m = m.setTab(m.tab + 1)
			}
			return m, nil

		case km.Type == tea.KeyTab:
			return m, func() tea.Msg { return focusSidebarMsg{} }

		case km.Type == tea.KeyShiftTab:
			return m, func() tea.Msg { return focusEditorMsg{} }
		}
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	// Mouse scroll while in visual mode: re-render with updated selection.
	if m.visualMode && m.tab == rtBody && m.result != nil {
		m = m.refreshViewportBody()
	}
	return m, cmd
}

func (m ResponseModel) setTab(t respTab) ResponseModel {
	if t < rtPreview {
		t = rtPreview
	}
	if last := m.lastTab(); t > last {
		t = last
	}
	m.tab = t
	m.query = ""
	m.searching = false
	m.searchInput.SetValue("")
	m.matchPositions = nil
	m.matchIndex = 0
	return m.refreshViewport()
}

func (m ResponseModel) lastTab() respTab {
	if len(m.testRows) > 0 {
		return rtTests
	}
	return rtHeaders
}

func (m ResponseModel) View() string {
	var statusLine string
	switch {
	case m.streaming:
		statusLine = statusOK.Render(m.streamStatus) + hint.Render(fmt.Sprintf("  streaming  %d lines", len(m.streamLines)))
	case m.loading:
		statusLine = hint.Render("sending...")
	case m.err != nil:
		statusLine = statusErr.Render("error  ") + hint.Render(m.err.Error())
	case m.result != nil:
		pct := int(m.viewport.ScrollPercent() * 100)
		elapsedStr := m.result.elapsed.Round(time.Millisecond).String()
		pipe := themed(lipgloss.NewStyle().Foreground(colorBorder)).Render(" | ")
		ct := clipText(m.result.contentType, max(10, m.width-42))
		statusLine = statusStyle(m.result.code).Render(m.result.status) +
			pipe + elapsedColor(m.result.elapsed).Render(elapsedStr) +
			pipe + hint.Render(formatSize(m.result.size)) +
			pipe + hint.Render(ct) +
			pipe + hint.Render(fmt.Sprintf("%d%%", pct))
	default:
		statusLine = hint.Render("no response yet")
	}

	bodyLabel := "body"
	if m.result != nil {
		switch {
		case strings.Contains(m.result.contentType, "json"):
			bodyLabel = "body - json"
		case strings.Contains(m.result.contentType, "xml"):
			bodyLabel = "body - xml"
		case strings.Contains(m.result.contentType, "html"):
			bodyLabel = "body - html"
		}
	}
	tabNames := []string{"preview", bodyLabel, "headers"}
	if len(m.testRows) > 0 {
		pass, total := 0, 0
		for _, r := range m.testRows {
			if !r.isSet {
				total++
				if r.pass {
					pass++
				}
			}
		}
		tabNames = append(tabNames, fmt.Sprintf("tests %d/%d", pass, total))
	}

	tabs := renderTabs(tabNames, int(m.tab), m.focused)

	// Status bar below the tabs - search or visual mode indicator.
	var extra string
	if m.searching {
		matchInfo := m.matchCountStr()
		extra = "\n" + hint.Render("/ ") + m.searchInput.View() + matchInfo +
			hint.Render("  enter confirm  esc cancel")
	} else if m.query != "" {
		matchInfo := m.matchCountStr()
		extra = "\n" + hint.Render("/ ") + searchMatch.Render(" "+m.query+" ") +
			matchInfo + hint.Render("  n/N navigate  esc clear")
	} else if m.visualMode {
		lo, hi := m.visualAnchor, m.viewport.YOffset+m.viewport.Height-1
		if lo > hi {
			lo, hi = hi, lo
		}
		nLines := hi - lo + 1
		extra = "\n" + themed(lipgloss.NewStyle().
			Foreground(colorAccent).
			Bold(true)).
			Render(fmt.Sprintf(" VISUAL  %d lines   y copy   esc cancel", nLines))
	} else if m.treeView && m.jsonTree != nil {
		extra = "\n" + hint.Render("tree   space toggle   c collapse all   e expand all   {/} sibling   t exit")
	}

	statusLine = fillLine(statusLine, m.ruleWidth())
	parts := []string{statusLine, mutedRule(m.ruleWidth()), tabs}
	if extra != "" {
		parts = append(parts, extra)
	}
	parts = append(parts, m.viewport.View())
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m ResponseModel) ruleWidth() int {
	return max(0, m.width-2)
}

func (m ResponseModel) matchCountStr() string {
	if len(m.matchPositions) == 0 {
		if m.query != "" {
			return hint.Render("  no matches")
		}
		return ""
	}
	return hint.Render(fmt.Sprintf("  %d/%d", m.matchIndex+1, len(m.matchPositions)))
}

func (m ResponseModel) Focus() ResponseModel {
	m.focused = true
	return m
}

func (m ResponseModel) Blur() ResponseModel {
	m.focused = false
	m.searching = false
	m.searchInput.SetValue("")
	m.query = ""
	m.matchPositions = nil
	m.matchIndex = 0
	if m.visualMode {
		m.visualMode = false
		if m.result != nil {
			m = m.refreshViewport()
		}
	}
	return m
}

func (m ResponseModel) SetSize(w, h int) ResponseModel {
	m.width = w
	m.height = h
	m.viewport.Width = max(1, m.ruleWidth())
	m.viewport.Height = max(3, h-6)
	return m.refreshViewport()
}

func (m ResponseModel) RefreshTheme() ResponseModel {
	return m.refreshViewport()
}

func (m ResponseModel) SetLoading(v bool) ResponseModel {
	m.loading = v
	if v {
		m.result = nil
		m.err = nil
		m.testRows = nil
		m.tab = rtPreview
		m.streaming = false
		m.streamLines = nil
		m.visualMode = false
		m.tooLarge = false
		m.jsonTree = nil
		m.treeView = false
		m.viewport.SetContent("")
	}
	return m
}

func (m ResponseModel) AppendStreamLine(line string) ResponseModel {
	m.streamLines = append(m.streamLines, line)
	m.viewport.SetContent(wrapText(strings.Join(m.streamLines, "\n"), m.viewport.Width))
	m.viewport.GotoBottom()
	return m
}

func (m ResponseModel) FinalizeStream(elapsed time.Duration, body []byte, ct string) ResponseModel {
	r := buildResult(body, m.streamHdrs, m.streamStatus, m.streamCode, elapsed)
	m.streaming = false
	return m.SetResult(r)
}

func (m ResponseModel) SetResult(r *result) ResponseModel {
	m.loading = false
	m.tooLarge = false
	m.result = r
	m.err = nil
	m.tab = rtPreview
	m.query = ""
	m.matchPositions = nil
	m.matchIndex = 0
	m.visualMode = false
	m.jsonTree = nil
	m.treeView = false
	if strings.Contains(r.contentType, "json") {
		if tree, err := parseJSONTree([]byte(r.plainBody)); err == nil {
			m.jsonTree = tree
		}
	}
	m.viewport.GotoTop()
	return m.refreshViewport()
}

func (m ResponseModel) SetTooLarge(r *result) ResponseModel {
	m.loading = false
	m.tooLarge = true
	m.result = r
	m.err = nil
	m.tab = rtPreview
	m.query = ""
	m.matchPositions = nil
	m.matchIndex = 0
	m.visualMode = false
	m.treeView = false
	m.jsonTree = nil
	if strings.Contains(r.contentType, "json") {
		if tree, err := parseJSONTree([]byte(r.plainBody)); err == nil {
			m.jsonTree = tree
		}
	}
	m.viewport.GotoTop()
	return m.refreshViewport()
}

func (m ResponseModel) SetError(err error) ResponseModel {
	m.loading = false
	m.result = nil
	m.err = err
	m.visualMode = false
	m.viewport.SetContent(err.Error())
	return m
}

func (m ResponseModel) SetTestRows(rows []TestRow) ResponseModel {
	m.testRows = rows
	if m.tab > m.lastTab() {
		m.tab = m.lastTab()
	}
	return m.refreshViewport()
}

func (m ResponseModel) refreshViewport() ResponseModel {
	if colorBg != "" {
		m.viewport.Style = lipgloss.NewStyle().Background(colorBg)
	} else {
		m.viewport.Style = lipgloss.NewStyle()
	}
	if m.result == nil {
		return m
	}
	switch m.tab {
	case rtPreview:
		if m.query != "" {
			m = m.applySearch(m.query)
		} else {
			m.viewport.SetContent(m.previewView())
		}
	case rtBody:
		if m.result.binary {
			m.viewport.SetContent(m.binaryView())
		} else if m.tooLarge {
			m.viewport.SetContent(m.tooLargeView())
		} else if m.treeView && m.jsonTree != nil {
			m = m.syncViewportToCursor()
		} else if m.query != "" {
			m = m.applySearch(m.query)
		} else if m.visualMode {
			m = m.refreshViewportBody()
		} else {
			m.viewport.SetContent(wrapText(m.result.body, m.viewport.Width))
		}
	case rtHeaders:
		m.viewport.SetContent(wrapHeaderText(m.result.rawHeaders, m.viewport.Width))
	case rtTests:
		m.viewport.SetContent(m.renderTestRows())
	}
	return m
}

func (m ResponseModel) previewView() string {
	if m.result == nil {
		return ""
	}
	return strings.Join(m.previewLines(false), "\n")
}

func (m ResponseModel) previewPlain() string {
	if m.result == nil {
		return ""
	}
	return strings.Join(m.previewLines(true), "\n")
}

func (m ResponseModel) previewLines(plain bool) []string {
	r := m.result
	width := max(20, m.viewport.Width)
	line := func(label, value string) string {
		return previewKV(label, value, width, plain)
	}

	kind := responseKind(r)
	if r.binary {
		kind = "binary"
	}
	rows := []string{
		previewTitle("response preview", plain),
		line("status", r.status),
		line("elapsed", r.elapsed.Round(time.Millisecond).String()),
		line("size", formatSize(r.size)),
		line("type", defaultString(r.contentType, kind)),
		line("headers", fmt.Sprintf("%d", headerLineCount(r.rawHeaders))),
	}
	if n := setCookieCount(r.rawHeaders); n > 0 {
		rows = append(rows, line("set-cookie", fmt.Sprintf("%d", n)))
	}
	rows = append(rows, "")

	if r.binary {
		rows = append(rows,
			previewTitle("body", plain),
			line("kind", "binary "+formatSize(r.size)),
			line("actions", "w save to file, y copy raw bytes"),
		)
		return rows
	}

	rows = append(rows, bodyPreviewLines(r, width, plain)...)
	return rows
}

func bodyPreviewLines(r *result, width int, plain bool) []string {
	body := strings.TrimSpace(r.plainBody)
	if body == "" {
		return []string{previewTitle("body", plain), previewKV("kind", "empty", width, plain)}
	}
	if lines, ok := jsonPreviewLines(body, width, plain); ok {
		return lines
	}
	ct := strings.ToLower(r.contentType)
	switch {
	case strings.Contains(ct, "html") || looksLikeHTML(body):
		return htmlPreviewLines(body, width, plain)
	case isXMLContentType(ct) || looksLikeXML(body):
		return xmlPreviewLines(body, width, plain)
	default:
		return textPreviewLines(body, width, plain)
	}
}

func jsonPreviewLines(body string, width int, plain bool) ([]string, bool) {
	var v any
	dec := json.NewDecoder(strings.NewReader(body))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		return nil, false
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, false
	}

	rows := []string{previewTitle("json", plain)}
	switch x := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		rows = append(rows, previewKV("shape", fmt.Sprintf("object, %d keys", len(keys)), width, plain))
		for _, k := range firstStrings(keys, 10) {
			rows = append(rows, previewBullet(k, jsonValueSummary(x[k]), width, plain))
		}
		if len(keys) > 10 {
			rows = append(rows, previewBullet("...", fmt.Sprintf("%d more keys", len(keys)-10), width, plain))
		}
	case []any:
		rows = append(rows, previewKV("shape", fmt.Sprintf("array, %d items", len(x)), width, plain))
		for i, item := range firstAny(x, 8) {
			rows = append(rows, previewBullet(fmt.Sprintf("[%d]", i), jsonValueSummary(item), width, plain))
		}
		if len(x) > 8 {
			rows = append(rows, previewBullet("...", fmt.Sprintf("%d more items", len(x)-8), width, plain))
		}
	default:
		rows = append(rows, previewKV("value", jsonValueSummary(x), width, plain))
	}
	return rows, true
}

func htmlPreviewLines(body string, width int, plain bool) []string {
	title := firstHTMLMatch(body, `(?is)<title[^>]*>(.*?)</title>`)
	h1 := firstHTMLMatch(body, `(?is)<h1[^>]*>(.*?)</h1>`)
	h2 := firstHTMLMatch(body, `(?is)<h2[^>]*>(.*?)</h2>`)
	text := stripHTML(body)
	rows := []string{previewTitle("html", plain)}
	if title != "" {
		rows = append(rows, previewKV("title", title, width, plain))
	}
	if h1 != "" {
		rows = append(rows, previewKV("h1", h1, width, plain))
	}
	if h2 != "" {
		rows = append(rows, previewKV("h2", h2, width, plain))
	}
	if text != "" {
		rows = append(rows, "", previewTitle("text", plain))
		for _, line := range previewTextLines(text, 5) {
			rows = append(rows, previewText(line, width, plain))
		}
	}
	return rows
}

func xmlPreviewLines(body string, width int, plain bool) []string {
	dec := xml.NewDecoder(strings.NewReader(body))
	root := ""
	children := make(map[string]bool)
	var textParts []string
	depth := 0
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return textPreviewLines(body, width, plain)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			if root == "" {
				root = t.Name.Local
			} else if depth == 2 {
				children[t.Name.Local] = true
			}
		case xml.EndElement:
			if depth > 0 {
				depth--
			}
		case xml.CharData:
			if s := compactSpace(string(t)); s != "" {
				textParts = append(textParts, s)
			}
		}
	}
	rows := []string{previewTitle("xml", plain)}
	if root != "" {
		rows = append(rows, previewKV("root", root, width, plain))
	}
	if len(children) > 0 {
		names := make([]string, 0, len(children))
		for name := range children {
			names = append(names, name)
		}
		sort.Strings(names)
		rows = append(rows, previewKV("children", strings.Join(firstStrings(names, 8), ", "), width, plain))
	}
	if len(textParts) > 0 {
		rows = append(rows, "", previewTitle("text", plain))
		for _, line := range previewTextLines(strings.Join(textParts, " "), 4) {
			rows = append(rows, previewText(line, width, plain))
		}
	}
	return rows
}

func textPreviewLines(body string, width int, plain bool) []string {
	rows := []string{previewTitle("text", plain)}
	lines := strings.Split(body, "\n")
	rows = append(rows, previewKV("lines", fmt.Sprintf("%d", len(lines)), width, plain))
	rows = append(rows, "")
	for _, line := range previewTextLines(body, 8) {
		rows = append(rows, previewText(line, width, plain))
	}
	return rows
}

func previewTitle(s string, plain bool) string {
	if plain {
		return s
	}
	return themed(lipgloss.NewStyle().Foreground(colorAccent).Bold(true)).Render(s)
}

func previewKV(label, value string, width int, plain bool) string {
	label = clipText(label, 12)
	value = compactSpace(value)
	if value == "" {
		value = "-"
	}
	if plain {
		return fmt.Sprintf("%-12s %s", label, clipText(value, max(1, width-13)))
	}
	return hint.Render(fmt.Sprintf("%-12s", label)) + " " + clipText(value, max(1, width-13))
}

func previewBullet(label, value string, width int, plain bool) string {
	label = clipText(label, 20)
	value = compactSpace(value)
	if plain {
		return "  " + clipText(label, 20) + "  " + clipText(value, max(1, width-24))
	}
	return "  " + clipText(label, 20) + themedSpace(max(1, 22-lipgloss.Width(label))) + hint.Render(clipText(value, max(1, width-24)))
}

func previewText(line string, width int, plain bool) string {
	line = compactSpace(line)
	if plain {
		return clipText(line, width)
	}
	return hint.Render(clipText(line, width))
}

func responseKind(r *result) string {
	ct := strings.ToLower(r.contentType)
	switch {
	case strings.Contains(ct, "json"):
		return "json"
	case strings.Contains(ct, "html"):
		return "html"
	case strings.Contains(ct, "xml"):
		return "xml"
	case strings.HasPrefix(ct, "text/"):
		return "text"
	case ct != "":
		return ct
	default:
		return "text"
	}
}

func defaultString(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}

func headerLineCount(raw string) int {
	count := 0
	for _, line := range strings.Split(raw, "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count
}

func setCookieCount(raw string) int {
	count := 0
	for _, line := range strings.Split(raw, "\n") {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(line)), "set-cookie:") {
			count++
		}
	}
	return count
}

func looksLikeHTML(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return strings.HasPrefix(s, "<!doctype html") || strings.HasPrefix(s, "<html") || strings.Contains(s, "<body")
}

func looksLikeXML(s string) bool {
	s = strings.TrimSpace(s)
	return strings.HasPrefix(s, "<?xml") || (strings.HasPrefix(s, "<") && strings.Contains(s, ">"))
}

func firstHTMLMatch(body, pattern string) string {
	re := regexp.MustCompile(pattern)
	match := re.FindStringSubmatch(body)
	if len(match) < 2 {
		return ""
	}
	return compactSpace(stripHTML(match[1]))
}

func stripHTML(s string) string {
	s = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`).ReplaceAllString(s, " ")
	s = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`).ReplaceAllString(s, " ")
	s = regexp.MustCompile(`(?s)<[^>]+>`).ReplaceAllString(s, " ")
	return compactSpace(html.UnescapeString(s))
}

func compactSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func jsonValueSummary(v any) string {
	switch x := v.(type) {
	case map[string]any:
		return fmt.Sprintf("object, %d keys", len(x))
	case []any:
		if len(x) == 0 {
			return "array, 0 items"
		}
		return fmt.Sprintf("array, %d items, first %s", len(x), jsonValueSummary(x[0]))
	case string:
		return "string " + fmt.Sprintf("%q", clipText(x, 48))
	case json.Number:
		return "number " + x.String()
	case bool:
		return fmt.Sprintf("bool %v", x)
	case nil:
		return "null"
	default:
		return fmt.Sprintf("%T", x)
	}
}

func firstStrings(values []string, n int) []string {
	if len(values) <= n {
		return values
	}
	return values[:n]
}

func firstAny(values []any, n int) []any {
	if len(values) <= n {
		return values
	}
	return values[:n]
}

func previewTextLines(body string, maxLines int) []string {
	var out []string
	for _, line := range strings.Split(body, "\n") {
		line = compactSpace(line)
		if line == "" {
			continue
		}
		out = append(out, line)
		if len(out) == maxLines {
			break
		}
	}
	if len(out) == 0 {
		return []string{"empty body"}
	}
	return out
}

func (m ResponseModel) binaryView() string {
	if m.result == nil {
		return ""
	}
	warn := themed(lipgloss.NewStyle().Foreground(colorSubtle).Bold(true))
	ct := m.result.contentType
	if ct == "" {
		ct = "binary"
	}
	return warn.Render(ct+"  "+formatSize(m.result.size)) + "\n\n" +
		hint.Render("w  save to file") + "\n" +
		hint.Render("y  copy raw bytes to clipboard")
}

func (m ResponseModel) tooLargeView() string {
	if m.result == nil {
		return ""
	}
	size := formatSize(m.result.size)
	warn := themed(lipgloss.NewStyle().Foreground(colorWarn).Bold(true))
	treeHint := ""
	if m.jsonTree != nil {
		treeHint = "\n" + hint.Render("t      tree view")
	}
	return warn.Render("response body too large to display  ("+size+")") + "\n\n" +
		hint.Render("enter  show anyway") +
		treeHint + "\n" +
		hint.Render("s      save to file") + "\n" +
		hint.Render("y      copy to clipboard")
}

func (m ResponseModel) syncViewportToCursor() ResponseModel {
	if m.jsonTree == nil {
		return m
	}
	cursor := m.jsonTree.cursor
	vpH := m.viewport.Height
	off := m.viewport.YOffset
	if cursor < off {
		off = cursor
	} else if cursor >= off+vpH {
		off = cursor - vpH + 1
	}
	if off < 0 {
		off = 0
	}
	m.viewport.SetContent(m.jsonTree.render(m.viewport.Width))
	m.viewport.YOffset = off
	return m
}

func (m ResponseModel) refreshViewportTree() ResponseModel {
	if m.jsonTree == nil || !m.treeView {
		return m
	}
	return m.syncViewportToCursor()
}

// refreshViewportBody re-renders the body tab content, applying visual
// selection highlights when active.
func (m ResponseModel) refreshViewportBody() ResponseModel {
	if m.result == nil || m.tab != rtBody {
		return m
	}
	if m.visualMode {
		plain := wrapText(m.result.plainBody, m.viewport.Width)
		m.viewport.SetContent(m.renderWithVisualMode(plain))
	} else {
		m.viewport.SetContent(wrapText(m.result.body, m.viewport.Width))
	}
	return m
}

// renderWithVisualMode takes plain (unstyled) text and highlights the selected
// line range with a background, making the selection visible.
func (m ResponseModel) renderWithVisualMode(plain string) string {
	lo := m.visualAnchor
	hi := m.viewport.YOffset + m.viewport.Height - 1
	if lo > hi {
		lo, hi = hi, lo
	}

	lines := strings.Split(plain, "\n")
	selStyle := lipgloss.NewStyle().
		Background(colorBorder).
		Foreground(colorText)

	for i := lo; i <= hi && i < len(lines); i++ {
		l := lines[i]
		// Pad to viewport width so the background covers the full line.
		rw := lipgloss.Width(l)
		if rw < m.viewport.Width {
			l += strings.Repeat(" ", m.viewport.Width-rw)
		}
		lines[i] = selStyle.Render(l)
	}
	return strings.Join(lines, "\n")
}

// visualSelection extracts the plain text for the currently selected line range.
func (m ResponseModel) visualSelection() string {
	if m.result == nil {
		return ""
	}
	var source string
	switch m.tab {
	case rtBody:
		source = m.result.plainBody
	case rtHeaders:
		source = wrapHeaderText(m.result.rawHeaders, m.viewport.Width)
	default:
		return ""
	}

	lo := m.visualAnchor
	hi := m.viewport.YOffset + m.viewport.Height - 1
	if lo > hi {
		lo, hi = hi, lo
	}

	lines := strings.Split(source, "\n")
	if lo < 0 {
		lo = 0
	}
	if hi >= len(lines) {
		hi = len(lines) - 1
	}
	if lo > hi {
		return ""
	}
	return strings.Join(lines[lo:hi+1], "\n")
}

func (m ResponseModel) renderTestRows() string {
	if len(m.testRows) == 0 {
		return hint.Render("no test results yet - add assertions in the tests tab")
	}
	var sb strings.Builder
	for _, r := range m.testRows {
		if r.isSet {
			sb.WriteString(testSet.Render("SET  ") + r.label + "\n")
		} else if r.pass {
			sb.WriteString(testPass.Render("PASS") + "  " + r.label + "\n")
		} else {
			sb.WriteString(testFail.Render("FAIL") + "  " + r.label)
			if r.actual != "" {
				sb.WriteString(hint.Render("  ->  " + r.actual))
			}
			sb.WriteString("\n")
		}
	}
	return strings.TrimRight(sb.String(), "\n")
}

func (m ResponseModel) applySearch(query string) ResponseModel {
	if query == "" || m.result == nil {
		m.matchPositions = nil
		m.matchIndex = 0
		return m.refreshViewportNoSearch()
	}

	source := ""
	switch m.tab {
	case rtPreview:
		source = m.previewPlain()
	case rtBody:
		source = wrapText(m.result.plainBody, m.viewport.Width)
	case rtHeaders:
		source = wrapHeaderText(m.result.rawHeaders, m.viewport.Width)
	}

	re, err := regexp.Compile("(?i)" + regexp.QuoteMeta(query))
	if err != nil {
		m.viewport.SetContent(source)
		return m
	}

	lines := strings.Split(source, "\n")
	rendered := make([]string, len(lines))
	m.matchPositions = nil

	for i, line := range lines {
		if re.MatchString(line) {
			rendered[i] = re.ReplaceAllStringFunc(line, func(s string) string {
				return searchMatch.Render(s)
			})
			m.matchPositions = append(m.matchPositions, i)
		} else {
			rendered[i] = line
		}
	}

	m.viewport.SetContent(strings.Join(rendered, "\n"))

	if len(m.matchPositions) > 0 {
		if m.matchIndex >= len(m.matchPositions) {
			m.matchIndex = 0
		}
		m.viewport.YOffset = m.matchPositions[m.matchIndex]
	}
	return m
}

func (m ResponseModel) refreshViewportNoSearch() ResponseModel {
	if m.result == nil {
		return m
	}
	switch m.tab {
	case rtPreview:
		m.viewport.SetContent(m.previewView())
	case rtBody:
		m.viewport.SetContent(wrapText(m.result.body, m.viewport.Width))
	case rtHeaders:
		m.viewport.SetContent(wrapHeaderText(m.result.rawHeaders, m.viewport.Width))
	}
	return m
}

func buildResult(body []byte, hdrs http.Header, status string, code int, elapsed time.Duration) *result {
	ct := hdrs.Get("Content-Type")
	if idx := strings.Index(ct, ";"); idx != -1 {
		ct = strings.TrimSpace(ct[:idx])
	}

	if isBinaryContentType(ct) {
		return &result{
			status:      status,
			code:        code,
			elapsed:     elapsed,
			rawHeaders:  formatHeaders(hdrs),
			contentType: ct,
			size:        len(body),
			binary:      true,
		}
	}

	var plain string
	if isXMLContentType(ct) {
		plain = prettyXML(body)
	} else {
		plain = prettyJSON(body)
	}
	highlighted := highlight([]byte(plain), ct)
	if highlighted == "" {
		highlighted = plain
	}

	return &result{
		status:      status,
		code:        code,
		elapsed:     elapsed,
		body:        highlighted,
		plainBody:   plain,
		rawHeaders:  formatHeaders(hdrs),
		contentType: ct,
		size:        len(body),
	}
}

func prettyJSON(b []byte) string {
	var buf bytes.Buffer
	if err := json.Indent(&buf, b, "", "  "); err == nil {
		return buf.String()
	}
	return string(b)
}

func prettyXML(b []byte) string {
	dec := xml.NewDecoder(bytes.NewReader(b))
	dec.Strict = false

	var buf bytes.Buffer
	enc := xml.NewEncoder(&buf)
	enc.Indent("", "  ")

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return string(b) // fall back on bad XML
		}
		if err := enc.EncodeToken(tok); err != nil {
			return string(b)
		}
	}
	if err := enc.Flush(); err != nil {
		return string(b)
	}
	if out := strings.TrimSpace(buf.String()); out != "" {
		return out
	}
	return string(b)
}

func isXMLContentType(ct string) bool {
	ct = strings.ToLower(ct)
	return strings.Contains(ct, "xml")
}

func isBinaryContentType(ct string) bool {
	ct = strings.ToLower(ct)
	return strings.HasPrefix(ct, "image/") ||
		strings.HasPrefix(ct, "audio/") ||
		strings.HasPrefix(ct, "video/") ||
		ct == "application/octet-stream" ||
		ct == "application/pdf" ||
		ct == "application/zip" ||
		ct == "application/gzip"
}

func formatHeaders(h http.Header) string {
	var b strings.Builder
	for k, vs := range h {
		for _, v := range vs {
			fmt.Fprintf(&b, "%s: %s\n", k, v)
		}
	}
	return b.String()
}

func formatSize(n int) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%dB", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1fkB", float64(n)/1024)
	default:
		return fmt.Sprintf("%.1fMB", float64(n)/(1024*1024))
	}
}

func wrapText(s string, width int) string {
	if width <= 0 {
		return s
	}
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if utf8.RuneCountInString(line) <= width {
			out = append(out, line)
			continue
		}
		var cur strings.Builder
		for _, word := range strings.Fields(line) {
			wl := utf8.RuneCountInString(word)
			cl := utf8.RuneCountInString(cur.String())
			switch {
			case cl == 0:
				cur.WriteString(word)
			case cl+1+wl <= width:
				cur.WriteByte(' ')
				cur.WriteString(word)
			default:
				out = append(out, cur.String())
				cur.Reset()
				cur.WriteString(word)
			}
		}
		if cur.Len() > 0 {
			out = append(out, cur.String())
		}
	}
	return strings.Join(out, "\n")
}

func wrapHeaderText(s string, width int) string {
	if width <= 0 {
		return s
	}
	soft := wrapText(s, width)
	var out []string
	for _, line := range strings.Split(soft, "\n") {
		for lipgloss.Width(line) > width {
			var b strings.Builder
			for _, r := range line {
				next := b.String() + string(r)
				if lipgloss.Width(next) > width {
					break
				}
				b.WriteRune(r)
			}
			part := b.String()
			if part == "" {
				break
			}
			out = append(out, part)
			line = strings.TrimPrefix(line, part)
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func buildTestRows(r tests.RunResult) []TestRow {
	var rows []TestRow
	for _, a := range r.Assertions {
		rows = append(rows, TestRow{
			label:  a.Label,
			pass:   a.Pass,
			actual: a.Actual,
		})
	}
	for k, v := range r.EnvUpdates {
		display := v
		if len(display) > 40 {
			display = display[:40] + "..."
		}
		rows = append(rows, TestRow{
			label: fmt.Sprintf("%s = %s", k, display),
			pass:  true,
			isSet: true,
		})
	}
	return rows
}

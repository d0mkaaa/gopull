package tui

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/d0mkaaa/gopull/internal/client"
	"github.com/d0mkaaa/gopull/internal/curlparse"
	"github.com/d0mkaaa/gopull/internal/plugins"
	"github.com/d0mkaaa/gopull/internal/store"
	"github.com/d0mkaaa/gopull/internal/tests"
)

var methods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}

type rootFocus int

const (
	rfSidebar rootFocus = iota
	rfEditor
	rfResponse
)

type envItem struct{ e *store.Environment }

func (i envItem) Title() string       { return i.e.Name }
func (i envItem) Description() string { return fmt.Sprintf("%d variables", len(i.e.Variables)) }
func (i envItem) FilterValue() string { return i.e.Name }

type Model struct {
	sidebar  SidebarModel
	editor   EditorModel
	response ResponseModel
	spin     spinner.Model

	envPicker        list.Model
	envPickerVisible bool
	environments     []*store.Environment
	activeEnvID      string
	activeEnvName    string
	envVars          map[string]string

	importInput  textinput.Model
	importActive bool

	settings        SettingsModel
	settingsVisible bool

	focus          rootFocus
	sidebarVisible bool

	timeout      time.Duration
	theme        string
	activeStream io.Closer

	diff        DiffModel
	diffVisible bool

	palette        PaletteModel
	paletteVisible bool

	welcomeVisible bool
	welcomeStep    int

	runner        RunnerModel
	runnerVisible bool

	store      *store.Store
	configDir  string // ~/.config/gopull
	plugins    *plugins.Runner
	pluginInfo string // e.g. "2 plugins"
	status     string

	width  int
	height int
}

func New(st *store.Store) Model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(colorAccent)

	ep := list.New(nil, list.NewDefaultDelegate(), 40, 10)
	ep.SetShowTitle(false)
	ep.SetShowHelp(false)
	ep.KeyMap.Quit.SetEnabled(false)

	ii := textinput.New()
	ii.Placeholder = "path/to/collection.json"
	ii.CharLimit = 512

	m := Model{
		sidebar:        newSidebar(30, 20),
		editor:         newEditor(60, 20),
		response:       newResponse(60, 20),
		spin:           sp,
		envPicker:      ep,
		importInput:    ii,
		focus:          rfEditor,
		sidebarVisible: true,
		timeout:        30 * time.Second,
		theme:          "dark",
		store:     st,
		configDir: st.Dir(),
	}
	m.editor = m.editor.Focus()
	return m
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		loadDataCmd(m.store),
		loadPluginsCmd(filepath.Join(m.configDir, "plugins")),
		loadUserThemesCmd(m.configDir),
		spinner.Tick,
	)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {

	case dataLoadedMsg:
		m.theme = msg.theme
		m.timeout = msg.timeout
		applyTheme(themeByID(msg.theme))
		m.spin.Style = lipgloss.NewStyle().Foreground(colorAccent)
		m.sidebar = m.sidebar.Refresh(msg.cols)
		m.sidebar = m.sidebar.RefreshTheme()
		m.environments = msg.envs
		m.refreshEnvPicker()
		if msg.state.ActiveEnvID != "" {
			for _, e := range msg.envs {
				if e.ID == msg.state.ActiveEnvID {
					m = m.applyEnv(e)
					break
				}
			}
		}
		if len(msg.keybindings) > 0 {
			applyKeyOverrides(msg.keybindings)
		}
		if msg.loadErr != "" {
			m.status = "load error: " + msg.loadErr
			cmds = append(cmds, clearStatusCmd())
		}
		if !msg.state.SeenWelcome {
			m.welcomeVisible = true
		}
		return m, tea.Batch(cmds...)

	case focusEditorMsg:
		m = m.focusPanel(rfEditor)
		return m, nil

	case focusResponseMsg:
		m.diffVisible = false
		m = m.focusPanel(rfResponse)
		return m, nil

	case focusSidebarMsg:
		if m.sidebarVisible {
			m = m.focusPanel(rfSidebar)
		} else {
			m = m.focusPanel(rfEditor)
		}
		return m, nil

	case loadRequestMsg:
		m.editor = m.editor.Load(msg.req, msg.collID)
		m = m.focusPanel(rfEditor)
		return m, nil

	case collectionsUpdatedMsg:
		m.sidebar = m.sidebar.Refresh(msg.cols)
		if msg.status != "" {
			m.status = msg.status
			cmds = append(cmds, clearStatusCmd())
		}
		return m, tea.Batch(cmds...)

	case deleteRequestMsg:
		return m, deleteRequestCmd(m.store, msg.collID, msg.reqID)

	case deleteCollectionMsg:
		return m, deleteCollectionCmd(m.store, msg.collID)

	case saveResponseMsg:
		return m, saveBodyCmd(msg.body, msg.contentType)

	case fileWrittenMsg:
		if msg.err != nil {
			m.status = "write failed: " + msg.err.Error()
		} else {
			m.status = "saved → " + msg.path
		}
		return m, clearStatusCmd()

	case responseMsg:
		if msg.err != nil {
			m.response = m.response.SetError(msg.err)
		} else {
			m.response = m.response.SetResult(msg.r)
			cmds = append(cmds, appendHistoryCmd(m.store, msg.r, m.editor))
			if script := m.editor.TestsScript(); script != "" {
				res := tests.Run(script, msg.r.code, msg.rawBody)
				m.response = m.response.SetTestRows(buildTestRows(res))
				for k, v := range res.EnvUpdates {
					if m.envVars == nil {
						m.envVars = make(map[string]string)
					}
					m.envVars[k] = v
				}
			}
			// apply env updates from post_response plugins
			for k, v := range msg.envUpdates {
				if m.envVars == nil {
					m.envVars = make(map[string]string)
				}
				m.envVars[k] = v
			}
			if len(msg.pluginLogs) > 0 {
				m.status = msg.pluginLogs[len(msg.pluginLogs)-1]
				cmds = append(cmds, clearStatusCmd())
			}
		}
		return m, tea.Batch(cmds...)

	case streamReadyMsg:
		m.activeStream = msg.stream
		m.response = m.response.SetLoading(false)
		m.response.streaming = true
		m.response.streamCode = msg.statusCode
		m.response.streamStatus = msg.status
		m.response.streamHdrs = msg.headers
		m.response.streamStart = msg.start
		return m, streamFirstLineCmd(msg.stream, msg.start, msg.statusCode, msg.status, msg.headers)

	case streamLineMsg:
		m.response = m.response.AppendStreamLine(msg.line)
		return m, msg.next

	case streamDoneMsg:
		m.activeStream = nil
		ct := msg.headers.Get("Content-Type")
		m.response = m.response.FinalizeStream(msg.elapsed, msg.body, ct)
		if m.response.result != nil {
			cmds = append(cmds, appendHistoryCmd(m.store, m.response.result, m.editor))
		}
		if script := m.editor.TestsScript(); script != "" {
			res := tests.Run(script, msg.code, msg.body)
			m.response = m.response.SetTestRows(buildTestRows(res))
			for k, v := range res.EnvUpdates {
				if m.envVars == nil {
					m.envVars = make(map[string]string)
				}
				m.envVars[k] = v
			}
		}
		return m, tea.Batch(cmds...)

	case runCollectionMsg:
		for _, c := range m.sidebar.Collections() {
			if c.ID == msg.collID {
				m.runner = newRunner(c)
				m.runner = m.runner.SetSize(m.responsePaneWidth(), m.height-4)
				m.runnerVisible = true
				if len(m.runner.order) > 0 {
					m.runner.running = true
					return m, runNextRequestCmd(m.store, &m.runner, 0, m.envVars, m.timeout)
				}
				return m, nil
			}
		}
		return m, nil

	case runnerResultMsg:
		m.runner = m.runner.SetResult(msg.idx, msg.result)
		next := msg.idx + 1
		if next < len(m.runner.order) {
			return m, runNextRequestCmd(m.store, &m.runner, next, m.envVars, m.timeout)
		}
		m.runner.running = false
		m.runner.done = true
		return m, nil

	case paletteExecMsg:
		m.paletteVisible = false
		return m.execPaletteAction(msg)

	case openDiffMsg:
		return m, loadHistoryForDiffCmd(m.store, msg.currentBody)

	case historyLoadedMsg:
		m.diff = newDiff(msg.currentBody, msg.entries)
		m.diff = m.diff.SetSize(m.responsePaneWidth(), m.height-4)
		m.diffVisible = true
		return m, nil

	case importDoneMsg:
		if msg.err != nil {
			m.status = "import failed: " + msg.err.Error()
		} else {
			m.status = "imported: " + msg.col.Name
			if cols, err := m.store.LoadCollections(); err == nil {
				m.sidebar = m.sidebar.Refresh(cols)
			}
		}
		return m, clearStatusCmd()

	case exportDoneMsg:
		if msg.err != nil {
			m.status = "export failed: " + msg.err.Error()
		} else {
			m.status = "exported → " + msg.path
		}
		return m, clearStatusCmd()

	case themeAppliedMsg:
		m.theme = msg.theme
		m.sidebar = m.sidebar.RefreshTheme()
		m.spin.Style = lipgloss.NewStyle().Foreground(colorAccent)
		return m, saveThemeCmd(m.store, m.theme)

	case pluginsLoadedMsg:
		m.plugins = msg.runner
		if n := msg.runner.Count(); n > 0 {
			m.pluginInfo = fmt.Sprintf("%d plugin(s) loaded", n)
		}
		return m, nil

	case userThemesLoadedMsg:
		// Re-apply the active theme: if it's a user theme that just loaded,
		// this ensures the correct colors appear instead of the fallback.
		applyTheme(themeByID(m.theme))
		m.sidebar = m.sidebar.RefreshTheme()
		m.spin.Style = lipgloss.NewStyle().Foreground(colorAccent)
		return m, nil

	case customThemeSavedMsg:
		if msg.err != nil {
			m.status = "theme save failed: " + msg.err.Error()
			return m, clearStatusCmd()
		}
		themeRegistry[msg.themeID] = msg.theme
		applyTheme(msg.theme)
		m.theme = msg.themeID
		m.sidebar = m.sidebar.RefreshTheme()
		m.spin.Style = lipgloss.NewStyle().Foreground(colorAccent)
		m.settings = m.settings.ExitThemeEditor()
		m.settings.themes = AllThemeOptions()
		// Select the new theme in the picker.
		for i, t := range m.settings.themes {
			if t.id == msg.themeID {
				m.settings.themeIdx = i
				break
			}
		}
		m.status = "theme saved: " + msg.themeID
		return m, tea.Batch(saveThemeCmd(m.store, m.theme), clearStatusCmd())

	case tea.MouseMsg:
		// Left click: switch focus to the pane that was clicked.
		if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress {
			x := msg.X
			sideEnd := 0
			if m.sidebarVisible {
				sideEnd = m.sidebarPaneWidth() + 4 + 1 // border(2) + padding(2) + separator(1)
			}
			editorEnd := sideEnd + m.editorPaneWidth() + 4 + 1
			switch {
			case m.sidebarVisible && x < sideEnd:
				if m.focus != rfSidebar {
					m = m.focusPanel(rfSidebar)
				}
			case x < editorEnd:
				if m.focus != rfEditor {
					m = m.focusPanel(rfEditor)
				}
			default:
				if m.focus != rfResponse {
					m = m.focusPanel(rfResponse)
				}
			}
		}
		// Fall through to panel update so the viewport handles scroll wheel.

	case externalEditorDoneMsg:
		if msg.err == nil {
			data, err := os.ReadFile(msg.tmpFile)
			if err == nil {
				m.editor = m.editor.SetBody(string(data))
			}
		}
		_ = os.Remove(msg.tmpFile)
		return m, nil

	case curlCopiedMsg:
		m.status = "curl command copied to clipboard"
		return m, clearStatusCmd()

	case errMsg:
		m.status = msg.err.Error()
		return m, clearStatusCmd()

	case clearStatusMsg:
		m.status = ""
		return m, nil

	case spinner.TickMsg:
		if m.response.loading {
			var cmd tea.Cmd
			m.spin, cmd = m.spin.Update(msg)
			cmds = append(cmds, cmd)
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.relayout()
		return m, nil

	case tea.KeyMsg:
		if m.welcomeVisible {
			if m.welcomeStep < welcomeSteps-1 {
				m.welcomeStep++
				return m, nil
			}
			m.welcomeVisible = false
			m.welcomeStep = 0
			return m, markWelcomeSeenCmd(m.store)
		}
		if m.paletteVisible {
			return m.updatePalette(msg)
		}
		if m.diffVisible {
			var cmd tea.Cmd
			m.diff, cmd = m.diff.Update(msg)
			return m, cmd
		}
		if m.runnerVisible {
			if msg.Type == tea.KeyEsc && m.runner.done {
				m.runnerVisible = false
				return m, nil
			}
			return m, nil
		}
		if m.settingsVisible {
			return m.updateSettings(msg)
		}
		if m.envPickerVisible {
			return m.updateEnvPicker(msg)
		}
		if m.importActive {
			return m.updateImportOverlay(msg)
		}

		switch {
		case key.Matches(msg, keys.Palette):
			m.palette = newPalette(m.sidebar.Collections())
			m.paletteVisible = true
			return m, textinput.Blink

		case key.Matches(msg, keys.Quit):
			m.store.SaveState(&store.AppState{
				ActiveEnvID:        m.activeEnvID,
				ActiveCollectionID: m.sidebar.activeCollID,
			})
			return m, tea.Quit

		case key.Matches(msg, keys.Send):
			return m.doSend()

		case key.Matches(msg, keys.Save):
			return m.doSave()

		case key.Matches(msg, keys.NewRequest):
			m.editor = newEditor(m.editor.width, m.editor.height)
			m.editor = m.editor.Focus()
			m.focus = rfEditor
			m.status = ""
			return m, nil

		case key.Matches(msg, keys.ToggleSidebar):
			m.sidebarVisible = !m.sidebarVisible
			if !m.sidebarVisible && m.focus == rfSidebar {
				m = m.focusPanel(rfEditor)
			}
			m.relayout()
			return m, nil

		case key.Matches(msg, keys.Settings):
			m.settings = newSettings(m.theme, int(m.timeout.Seconds()), m.configDir)
			m.settingsVisible = true
			return m, nil

		case key.Matches(msg, keys.EnvPicker):
			m.envPickerVisible = true
			return m, nil

		case key.Matches(msg, keys.Import):
			m.importActive = true
			m.importInput.Focus()
			m.status = ""
			return m, textinput.Blink

		case key.Matches(msg, keys.Export):
			m.status = ""
			return m.doExport()

		case key.Matches(msg, keys.CurlExport):
			return m.doCurlExport()

		case key.Matches(msg, keys.ExternalEditor):
			return m.doExternalEditor()
		}
	}

	if m.importActive {
		var cmd tea.Cmd
		m.importInput, cmd = m.importInput.Update(msg)
		cmds = append(cmds, cmd)
		return m, tea.Batch(cmds...)
	}

	var cmd tea.Cmd
	switch m.focus {
	case rfSidebar:
		m.sidebar, cmd = m.sidebar.Update(msg)
	case rfEditor:
		m.editor, cmd = m.editor.Update(msg)
	case rfResponse:
		m.response, cmd = m.response.Update(msg)
	}
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m Model) View() string {
	if m.width == 0 {
		return "loading..."
	}

	var panes []string

	if m.sidebarVisible {
		s := pane
		if m.focus == rfSidebar {
			s = paneActive
		}
		panes = append(panes, s.Width(m.sidebarPaneWidth()).Render(m.sidebar.View()))
		panes = append(panes, " ")
	}

	edStyle := pane
	if m.focus == rfEditor {
		edStyle = paneActive
	}
	panes = append(panes, edStyle.Width(m.editorPaneWidth()).Render(m.editor.View()))
	panes = append(panes, " ")

	respStyle := pane
	if m.focus == rfResponse {
		respStyle = paneActive
	}
	panes = append(panes, respStyle.Width(m.responsePaneWidth()).Render(m.response.View()))

	body := lipgloss.JoinHorizontal(lipgloss.Top, panes...)

	if m.welcomeVisible {
		out := viewWelcome(m.welcomeStep, m.width, m.height)
		if colorBg != "" {
			return "\033]11;" + string(colorBg) + "\007" + out
		}
		return out
	}

	var out string

	switch {
	case m.diffVisible:
		overlay := paneActive.Width(m.width - 10).Render(m.diff.View())
		out = m.placeOverlay(overlay) + "\n" + m.viewHint()

	case m.paletteVisible:
		overlay := paneActive.Width(60).Render(m.palette.View())
		out = m.placeOverlay(overlay) + "\n" + m.viewHint()

	case m.runnerVisible:
		overlay := paneActive.Width(m.responsePaneWidth()).Render(m.runner.View())
		out = m.placeOverlay(overlay) + "\n" + m.viewHint()

	case m.settingsVisible:
		overlay := paneActive.Width(m.settings.overlayWidth()).Render(m.settings.View())
		out = m.placeOverlay(overlay) + "\n" + m.viewHint()

	case m.importActive:
		overlay := paneActive.Width(64).Render(
			lipgloss.JoinVertical(lipgloss.Left,
				sidebarTitle.Render("import postman collection"),
				"",
				hint.Render("file path:"),
				m.importInput.View(),
				"",
				hint.Render("  enter import   esc cancel"),
			),
		)
		out = m.placeOverlay(overlay) + "\n" + m.viewHint()

	case m.envPickerVisible:
		overlay := paneActive.Width(44).Render(
			lipgloss.JoinVertical(lipgloss.Left,
				sidebarTitle.Render("environments"),
				m.envPicker.View(),
				hint.Render("  enter select   esc cancel"),
			),
		)
		out = m.placeOverlay(overlay) + "\n" + m.viewHint()

	default:
		out = lipgloss.JoinVertical(lipgloss.Left, body, m.viewHint())
	}

	// OSC 11 sets the terminal's actual default background color so that
	// ANSI resets (ESC[0m) inside syntax-highlighted content reset to our
	// theme color instead of the terminal's native one.  Every frame we
	// re-assert it so theme switches take effect immediately.
	if colorBg != "" {
		out = "\033]11;" + string(colorBg) + "\007" + out
	}
	return out
}

// placeOverlay centers an overlay panel within the content area (height-1)
// so there is always room for the hint bar below.
//
// When a background color is set we manually build each line of the backdrop
// rather than relying on lipgloss.WithWhitespaceBackground, which does not
// paint cells that already have an ANSI reset from syntax highlighting.
func (m Model) placeOverlay(overlay string) string {
	if colorBg == "" {
		return lipgloss.Place(m.width, m.height-2,
			lipgloss.Center, lipgloss.Center, overlay)
	}

	bgStyle := lipgloss.NewStyle().Background(colorBg)
	bgLine := bgStyle.Render(strings.Repeat(" ", m.width))

	overlayLines := strings.Split(overlay, "\n")
	oh := len(overlayLines)
	ow := lipgloss.Width(overlay)

	totalH := m.height - 1
	topPad := (totalH - oh) / 2
	if topPad < 0 {
		topPad = 0
	}
	leftPad := (m.width - ow) / 2
	if leftPad < 0 {
		leftPad = 0
	}

	rows := make([]string, totalH)
	for i := range rows {
		oi := i - topPad
		if oi < 0 || oi >= len(overlayLines) {
			rows[i] = bgLine
			continue
		}
		line := overlayLines[oi]
		lw := lipgloss.Width(line)
		rpad := m.width - leftPad - lw
		if rpad < 0 {
			rpad = 0
		}
		rows[i] = bgStyle.Render(strings.Repeat(" ", leftPad)) +
			line +
			bgStyle.Render(strings.Repeat(" ", rpad))
	}
	return strings.Join(rows, "\n")
}

func (m Model) viewHint() string {
	envLabel := "no env"
	if m.activeEnvName != "" {
		envLabel = m.activeEnvName
	}
	env := envBadge.Render(envLabel)

	// context breadcrumb: collection / request name
	var breadcrumb string
	if name := m.editor.requestName; name != "" {
		breadcrumb = statusBar.Render("  " + name)
	}

	// plugin indicator
	var pluginIndicator string
	if m.pluginInfo != "" {
		pluginIndicator = lipgloss.NewStyle().Foreground(colorMuted).Render("  " + m.pluginInfo)
	}

	left := env + breadcrumb + pluginIndicator

	var mid string
	if m.status != "" {
		mid = hint.Render(m.status)
	} else {
		switch m.focus {
		case rfSidebar:
			if m.sidebar.PendingDelete() {
				mid = statusErr.Render("d again to confirm delete   esc cancel")
			} else if m.sidebar.InReqsMode() {
				mid = "↑↓ navigate   enter open   d delete   esc back   tab→editor"
			} else {
				mid = "↑↓ navigate   enter open   d delete   tab→editor"
			}
		case rfEditor:
			mid = "ctrl+r send   ctrl+s save   alt+c curl   alt+e editor   alt+j format   [/] tab"
		case rfResponse:
			if m.response.InVisualMode() {
				mid = "j/k extend selection   y copy   esc cancel"
			} else {
				mid = "j/k scroll   g/G top/btm   v select   / search   y copy   w save   D diff   [/] tab"
			}
		}
		mid = statusBar.Render(mid)
	}

	right := statusBar.Render("alt+p palette  ctrl+e env  alt+o settings  alt+q quit")

	gap := m.width - lipgloss.Width(left) - lipgloss.Width(mid) - lipgloss.Width(right) - 2
	if gap < 1 {
		gap = 1
	}
	bar := left + "  " + mid + strings.Repeat(" ", gap) + right
	sep := lipgloss.NewStyle().Foreground(colorBorder).Render(strings.Repeat("─", m.width))
	return sep + "\n" + bar
}

func (m Model) execPaletteAction(msg paletteExecMsg) (tea.Model, tea.Cmd) {
	switch msg.action {
	case "send":
		return m.doSend()
	case "save":
		return m.doSave()
	case "new":
		m.editor = newEditor(m.editor.width, m.editor.height)
		m.editor = m.editor.Focus()
		m.focus = rfEditor
		m.status = ""
		return m, nil
	case "sidebar":
		m.sidebarVisible = !m.sidebarVisible
		if !m.sidebarVisible && m.focus == rfSidebar {
			m = m.focusPanel(rfEditor)
		}
		m.relayout()
		return m, nil
	case "settings":
		m.settings = newSettings(m.theme, int(m.timeout.Seconds()), m.configDir)
		m.settingsVisible = true
		return m, nil
	case "env":
		m.envPickerVisible = true
		return m, nil
	case "import":
		m.importActive = true
		m.importInput.Focus()
		m.status = ""
		return m, textinput.Blink
	case "export":
		m.status = ""
		return m.doExport()
	case "curl_export":
		return m.doCurlExport()
	case "external_editor":
		return m.doExternalEditor()
	case "quit":
		return m, tea.Quit
	case "load":
		if msg.req != nil {
			m.editor = m.editor.Load(msg.req, msg.collID)
			m = m.focusPanel(rfEditor)
		}
		return m, nil
	}
	return m, nil
}

func (m Model) updatePalette(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyEsc {
		m.paletteVisible = false
		return m, nil
	}
	var cmd tea.Cmd
	m.palette, cmd = m.palette.Update(msg)
	return m, cmd
}

func (m Model) updateEnvPicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.envPickerVisible = false
		return m, nil
	case tea.KeyEnter:
		if item := m.envPicker.SelectedItem(); item != nil {
			if ei, ok := item.(envItem); ok {
				m = m.applyEnv(ei.e)
			}
		}
		m.envPickerVisible = false
		return m, nil
	}
	var cmd tea.Cmd
	m.envPicker, cmd = m.envPicker.Update(msg)
	return m, cmd
}

func (m *Model) refreshEnvPicker() {
	items := make([]list.Item, len(m.environments))
	for i, e := range m.environments {
		items[i] = envItem{e}
	}
	m.envPicker.SetItems(items)
}

func (m Model) applyEnv(e *store.Environment) Model {
	m.activeEnvID = e.ID
	m.activeEnvName = e.Name
	vars := make(map[string]string, len(e.Variables))
	// dotenv file vars come first so inline variables can override them
	if e.DotenvPath != "" {
		for k, v := range parseDotenv(e.DotenvPath) {
			vars[k] = v
		}
	}
	for _, v := range e.Variables {
		if v.Enabled {
			vars[v.Key] = v.Value
		}
	}
	m.envVars = vars
	return m
}

// Supports: KEY=value, KEY="value", KEY='value', export KEY=value, # comments.
func parseDotenv(path string) map[string]string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	vars := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		idx := strings.IndexByte(line, '=')
		if idx <= 0 {
			continue
		}
		k := strings.TrimSpace(line[:idx])
		v := strings.TrimSpace(line[idx+1:])
		// strip surrounding quotes
		if len(v) >= 2 {
			if (v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'') {
				v = v[1 : len(v)-1]
			}
		}
		if k != "" {
			vars[k] = v
		}
	}
	return vars
}

func (m Model) updateImportOverlay(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.importActive = false
		m.importInput.Blur()
		m.importInput.SetValue("")
		return m, nil
	case tea.KeyEnter:
		path := strings.TrimSpace(m.importInput.Value())
		if path == "" {
			return m, nil
		}
		m.importActive = false
		m.importInput.Blur()
		m.importInput.SetValue("")
		return m, importFileCmd(m.store, path)
	}
	var cmd tea.Cmd
	m.importInput, cmd = m.importInput.Update(msg)
	return m, cmd
}

func (m Model) updateSettings(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyEsc || key.Matches(msg, keys.Settings) {
		if m.settings.InThemeEditor() {
			// Esc from theme editor goes back to the theme list, not out.
			m.settings = m.settings.ExitThemeEditor()
			return m, nil
		}
		m.settingsVisible = false
		secs := m.settings.TimeoutValue()
		m.timeout = time.Duration(secs) * time.Second
		return m, saveSettingsCmd(m.store, m.theme, secs)
	}
	var cmd tea.Cmd
	m.settings, cmd = m.settings.Update(msg)
	return m, cmd
}

func (m Model) doSend() (tea.Model, tea.Cmd) {
	if m.response.loading {
		return m, nil
	}
	m.response = m.response.SetLoading(true)
	m.status = ""

	if m.activeStream != nil {
		m.activeStream.Close()
		m.activeStream = nil
	}

	req := m.editor.BuildRequest()

	// Safety net: if the user pressed ctrl+r while the URL field still
	// contains a raw curl command (without pressing Tab first), parse it now.
	if curlparse.LooksLikeCurl(req.URL) {
		if parsed, err := curlparse.Parse(req.URL); err == nil {
			m.editor = m.editor.Load(&parsed, m.editor.collectionID)
			req = m.editor.BuildRequest()
		}
	}
	timeout := m.timeout
	if req.Options.TimeoutSecs > 0 {
		timeout = time.Duration(req.Options.TimeoutSecs) * time.Second
	}
	envVars := m.envVars
	pluginRunner := m.plugins

	return m, tea.Batch(m.spin.Tick, func() tea.Msg {
		// pre_request plugins can modify method, url, headers, body
		var pluginLogs []string
		if pluginRunner != nil {
			req, pluginLogs = pluginRunner.RunPreRequest(req, envVars)
		}

		cr := buildClientRequest(req, envVars)

		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		resp, err := client.Send(ctx, cr)
		if err != nil {
			return responseMsg{err: err, pluginLogs: pluginLogs}
		}
		if resp.Stream != nil {
			return streamReadyMsg{
				stream:     resp.Stream,
				start:      resp.StartTime,
				statusCode: resp.StatusCode,
				status:     resp.Status,
				headers:    resp.Headers,
			}
		}

		r := buildResult(resp.Body, resp.Headers, resp.Status, resp.StatusCode, resp.Elapsed)

		// post_response plugins can extract env variables
		var envUpdates map[string]string
		if pluginRunner != nil {
			snap := plugins.RespSnapshot{
				StatusCode:  resp.StatusCode,
				ElapsedMs:   resp.Elapsed.Milliseconds(),
				SizeBytes:   len(resp.Body),
				Body:        string(resp.Body),
				ContentType: resp.Headers.Get("Content-Type"),
			}
			var postLogs []string
			envUpdates, postLogs = pluginRunner.RunPostResponse(req, snap, envVars)
			pluginLogs = append(pluginLogs, postLogs...)
		}

		return responseMsg{
			r:          r,
			rawBody:    resp.Body,
			envUpdates: envUpdates,
			pluginLogs: pluginLogs,
		}
	})
}

func buildClientRequest(req store.Request, envVars map[string]string) client.Request {
	cr := client.Request{
		Method:  req.Method,
		URL:     req.URL,
		Body:    req.Body.Raw,
		Env:     envVars,
		Auth:    client.Auth{Type: req.Auth.Type, Token: req.Auth.Token, User: req.Auth.User, Pass: req.Auth.Pass},
		Headers: make(map[string]string, len(req.Headers)),
		Options: client.Options{
			SkipTLSVerify:    req.Options.SkipTLSVerify,
			DisableRedirects: req.Options.DisableRedirects,
			ProxyURL:         req.Options.ProxyURL,
		},
	}
	for _, h := range req.Headers {
		if h.Enabled {
			cr.Headers[h.Key] = h.Value
		}
	}
	switch req.Body.Mode {
	case "form":
		vals := url.Values{}
		for _, line := range strings.Split(req.Body.Raw, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			idx := strings.Index(line, ":")
			if idx < 0 {
				continue
			}
			vals.Set(strings.TrimSpace(line[:idx]), strings.TrimSpace(line[idx+1:]))
		}
		cr.Body = vals.Encode()
		cr.Headers["Content-Type"] = "application/x-www-form-urlencoded"
	case "graphql":
		cr.Body = req.Body.Raw
		if _, ok := cr.Headers["Content-Type"]; !ok {
			cr.Headers["Content-Type"] = "application/json"
		}
	}
	return cr
}

func (m Model) doSave() (tea.Model, tea.Cmd) {
	m.status = ""
	req := m.editor.BuildRequest()
	if req.Name == "" {
		req.Name = req.URL
	}
	if req.Name == "" {
		req.Name = "Untitled"
	}

	collID := m.editor.collectionID
	st := m.store

	return m, func() tea.Msg {
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
		return collectionsUpdatedMsg{cols: cols, status: "saved"}
	}
}

func (m Model) doExport() (tea.Model, tea.Cmd) {
	collID := m.sidebar.activeCollID
	if collID == "" {
		m.status = "no collection open to export"
		return m, clearStatusCmd()
	}
	st := m.store
	return m, func() tea.Msg {
		cols, err := st.LoadCollections()
		if err != nil {
			return exportDoneMsg{err: err}
		}
		var target *store.Collection
		for _, c := range cols {
			if c.ID == collID {
				target = c
				break
			}
		}
		if target == nil {
			return exportDoneMsg{err: fmt.Errorf("collection not found")}
		}
		data, err := store.ExportPostman(target)
		if err != nil {
			return exportDoneMsg{err: err}
		}
		name := sanitizeFilename(target.Name) + ".json"
		if err := os.WriteFile(name, data, 0o644); err != nil {
			return exportDoneMsg{err: err}
		}
		abs, _ := filepath.Abs(name)
		return exportDoneMsg{path: abs}
	}
}

func (m *Model) relayout() {
	ph := m.height - 4 // 2 border rows + 2 hint rows (separator + bar)

	sideW := m.sidebarPaneWidth()
	edW := m.editorPaneWidth()
	respW := m.responsePaneWidth()

	if m.sidebarVisible {
		m.sidebar = m.sidebar.SetSize(sideW-4, ph-2)
	}
	m.editor = m.editor.setSize(edW, ph)
	m.response = m.response.SetSize(respW, ph)

	m.envPicker.SetSize(44, min(len(m.environments)+4, 14))
}

func (m Model) sidebarPaneWidth() int {
	if !m.sidebarVisible {
		return 0
	}
	return 32
}

func (m Model) editorPaneWidth() int {
	avail := m.width - 2
	if m.sidebarVisible {
		avail -= m.sidebarPaneWidth() + 1
	}
	return avail / 2
}

func (m Model) responsePaneWidth() int {
	avail := m.width - 2
	if m.sidebarVisible {
		avail -= m.sidebarPaneWidth() + 1
	}
	return avail - m.editorPaneWidth()
}

func (m Model) focusPanel(p rootFocus) Model {
	m.sidebar = m.sidebar.Blur()
	m.editor = m.editor.Blur()
	m.response = m.response.Blur()
	m.focus = p
	switch p {
	case rfSidebar:
		m.sidebar = m.sidebar.Focus()
	case rfEditor:
		m.editor = m.editor.Focus()
	case rfResponse:
		m.response = m.response.Focus()
	}
	return m
}

type dataLoadedMsg struct {
	cols         []*store.Collection
	envs         []*store.Environment
	state        *store.AppState
	timeout      time.Duration
	theme        string
	loadErr      string
	keybindings  map[string]string
}

func loadDataCmd(st *store.Store) tea.Cmd {
	return func() tea.Msg {
		var loadErr string
		cols, err := st.LoadCollections()
		if err != nil {
			loadErr = err.Error()
		}
		envs, _ := st.LoadEnvironments()
		state, _ := st.LoadState()
		if state == nil {
			state = &store.AppState{}
		}
		cfg, _ := st.LoadConfig()
		timeout := 30 * time.Second
		theme := "dark"
		if cfg != nil {
			if cfg.TimeoutSecs > 0 {
				timeout = time.Duration(cfg.TimeoutSecs) * time.Second
			}
			switch cfg.Theme {
			case "light", "nord", "gruvbox":
				theme = cfg.Theme
			}
		}
		kb, _ := st.LoadKeybindings()
		return dataLoadedMsg{
			cols:        cols,
			envs:        envs,
			state:       state,
			timeout:     timeout,
			theme:       theme,
			loadErr:     loadErr,
			keybindings: kb,
		}
	}
}

func appendHistoryCmd(st *store.Store, r *result, ed EditorModel) tea.Cmd {
	if r == nil {
		return nil
	}
	req := ed.BuildRequest()
	body := r.plainBody
	if len(body) > 100*1024 {
		body = body[:100*1024]
	}
	entry := store.HistoryEntry{
		Request: store.HistReq{
			Method: req.Method,
			URL:    req.URL,
			Body:   req.Body.Raw,
		},
		Response: store.HistResp{
			StatusCode:  r.code,
			ElapsedMs:   r.elapsed.Milliseconds(),
			SizeBytes:   r.size,
			Body:        body,
			ContentType: r.contentType,
		},
	}
	return func() tea.Msg {
		_ = st.AppendHistory(entry)
		return historyWrittenMsg{}
	}
}

func deleteRequestCmd(st *store.Store, collID, reqID string) tea.Cmd {
	return func() tea.Msg {
		if err := st.DeleteRequest(collID, reqID); err != nil {
			return errMsg{err}
		}
		cols, err := st.LoadCollections()
		if err != nil {
			return errMsg{err}
		}
		return collectionsUpdatedMsg{cols: cols, status: "deleted"}
	}
}

func deleteCollectionCmd(st *store.Store, collID string) tea.Cmd {
	return func() tea.Msg {
		if err := st.DeleteCollection(collID); err != nil {
			return errMsg{err}
		}
		cols, err := st.LoadCollections()
		if err != nil {
			return errMsg{err}
		}
		return collectionsUpdatedMsg{cols: cols, status: "collection deleted"}
	}
}

func importFileCmd(st *store.Store, path string) tea.Cmd {
	return func() tea.Msg {
		data, err := os.ReadFile(path)
		if err != nil {
			return importDoneMsg{err: fmt.Errorf("read file: %w", err)}
		}
		col, err := store.ImportPostman(data)
		if err != nil {
			return importDoneMsg{err: err}
		}
		if err := st.SaveCollection(col); err != nil {
			return importDoneMsg{err: fmt.Errorf("save: %w", err)}
		}
		return importDoneMsg{col: col}
	}
}

func saveBodyCmd(body, contentType string) tea.Cmd {
	return func() tea.Msg {
		ext := extForContentType(contentType)
		name := fmt.Sprintf("response_%s%s", time.Now().Format("20060102_150405"), ext)
		if err := os.WriteFile(name, []byte(body), 0o644); err != nil {
			return fileWrittenMsg{err: err}
		}
		abs, _ := filepath.Abs(name)
		return fileWrittenMsg{path: abs}
	}
}

func extForContentType(ct string) string {
	ct = strings.ToLower(ct)
	switch {
	case strings.Contains(ct, "json"):
		return ".json"
	case strings.Contains(ct, "html"):
		return ".html"
	case strings.Contains(ct, "xml"):
		return ".xml"
	case strings.Contains(ct, "javascript"):
		return ".js"
	case strings.Contains(ct, "css"):
		return ".css"
	case strings.Contains(ct, "yaml"):
		return ".yaml"
	default:
		return ".txt"
	}
}

func saveThemeCmd(st *store.Store, theme string) tea.Cmd {
	return func() tea.Msg {
		cfg, _ := st.LoadConfig()
		if cfg == nil {
			cfg = &store.Config{TimeoutSecs: 30}
		}
		cfg.Theme = theme
		_ = st.SaveConfig(cfg)
		return nil
	}
}

func saveSettingsCmd(st *store.Store, theme string, timeoutSecs int) tea.Cmd {
	return func() tea.Msg {
		cfg, _ := st.LoadConfig()
		if cfg == nil {
			cfg = &store.Config{}
		}
		cfg.Theme = theme
		cfg.TimeoutSecs = timeoutSecs
		_ = st.SaveConfig(cfg)
		return nil
	}
}

func runNextRequestCmd(st *store.Store, runner *RunnerModel, idx int, env map[string]string, timeout time.Duration) tea.Cmd {
	if idx < 0 || idx >= len(runner.order) {
		return nil
	}
	reqID := runner.order[idx]
	req := runner.collection.Requests[reqID]
	if req == nil {
		return func() tea.Msg {
			return runnerResultMsg{idx: idx, result: runnerResult{name: reqID, done: true, err: "not found"}}
		}
	}
	collectionReq := *req
	return func() tea.Msg {
		cr := client.Request{
			Method:  collectionReq.Method,
			URL:     collectionReq.URL,
			Env:     env,
			Headers: make(map[string]string),
			Auth: client.Auth{
				Type:  collectionReq.Auth.Type,
				Token: collectionReq.Auth.Token,
				User:  collectionReq.Auth.User,
				Pass:  collectionReq.Auth.Pass,
			},
		}
		for _, h := range collectionReq.Headers {
			if h.Enabled {
				cr.Headers[h.Key] = h.Value
			}
		}
		if collectionReq.Body.Mode == "form" {
			vals := url.Values{}
			for _, line := range strings.Split(collectionReq.Body.Raw, "\n") {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				i := strings.Index(line, ":")
				if i < 0 {
					continue
				}
				vals.Set(strings.TrimSpace(line[:i]), strings.TrimSpace(line[i+1:]))
			}
			cr.Body = vals.Encode()
			cr.Headers["Content-Type"] = "application/x-www-form-urlencoded"
		} else {
			cr.Body = collectionReq.Body.Raw
		}
		if collectionReq.Options.SkipTLSVerify || collectionReq.Options.DisableRedirects || collectionReq.Options.ProxyURL != "" {
			cr.Options = client.Options{
				SkipTLSVerify:    collectionReq.Options.SkipTLSVerify,
				DisableRedirects: collectionReq.Options.DisableRedirects,
				ProxyURL:         collectionReq.Options.ProxyURL,
			}
		}
		t := timeout
		if collectionReq.Options.TimeoutSecs > 0 {
			t = time.Duration(collectionReq.Options.TimeoutSecs) * time.Second
		}
		ctx, cancel := context.WithTimeout(context.Background(), t)
		defer cancel()
		start := time.Now()
		resp, err := client.Send(ctx, cr)
		elapsed := time.Since(start)
		rr := runnerResult{name: collectionReq.Name, done: true, elapsed: elapsed}
		if err != nil {
			rr.err = err.Error()
			return runnerResultMsg{idx: idx, result: rr}
		}
		if resp.Stream != nil {
			resp.Stream.Close()
		}
		rr.code = resp.StatusCode
		if collectionReq.Tests != "" {
			res := tests.Run(collectionReq.Tests, resp.StatusCode, resp.Body)
			for _, a := range res.Assertions {
				if a.Pass {
					rr.pass++
				} else {
					rr.fail++
				}
			}
		}
		return runnerResultMsg{idx: idx, result: rr}
	}
}

func loadHistoryForDiffCmd(st *store.Store, currentBody string) tea.Cmd {
	return func() tea.Msg {
		h, _ := st.LoadHistory()
		if h == nil {
			return historyLoadedMsg{currentBody: currentBody}
		}
		return historyLoadedMsg{entries: h.Entries, currentBody: currentBody}
	}
}

func streamFirstLineCmd(body io.ReadCloser, start time.Time, code int, status string, hdrs http.Header) tea.Cmd {
	scanner := bufio.NewScanner(body)
	var accumulated []byte
	var readLine func() tea.Msg
	readLine = func() tea.Msg {
		if !scanner.Scan() {
			body.Close()
			return streamDoneMsg{
				elapsed: time.Since(start),
				code:    code,
				status:  status,
				headers: hdrs,
				body:    accumulated,
			}
		}
		line := scanner.Text()
		accumulated = append(accumulated, []byte(line+"\n")...)
		return streamLineMsg{line: line, next: readLine}
	}
	return readLine
}

func markWelcomeSeenCmd(st *store.Store) tea.Cmd {
	return func() tea.Msg {
		state, _ := st.LoadState()
		if state == nil {
			state = &store.AppState{}
		}
		state.SeenWelcome = true
		_ = st.SaveState(state)
		return nil
	}
}

func clearStatusCmd() tea.Cmd {
	return tea.Tick(3*time.Second, func(time.Time) tea.Msg {
		return clearStatusMsg{}
	})
}

func loadPluginsCmd(dir string) tea.Cmd {
	return func() tea.Msg {
		return pluginsLoadedMsg{runner: plugins.Load(dir)}
	}
}

func loadUserThemesCmd(configDir string) tea.Cmd {
	return func() tea.Msg {
		themesDir := filepath.Join(configDir, "themes")
		WriteExampleTheme(themesDir)
		LoadUserThemes(themesDir)
		return userThemesLoadedMsg{}
	}
}

type userThemesLoadedMsg struct{}

func (m Model) doCurlExport() (tea.Model, tea.Cmd) {
	req := m.editor.BuildRequest()
	curl := requestToCurl(req)
	return m, func() tea.Msg {
		if err := copyToClipboard(curl); err != nil {
			return errMsg{err}
		}
		return curlCopiedMsg{}
	}
}

func requestToCurl(req store.Request) string {
	var parts []string
	parts = append(parts, "curl")
	if req.Method != "GET" && req.Method != "" {
		parts = append(parts, "-X", req.Method)
	}
	for _, h := range req.Headers {
		if h.Enabled && h.Key != "" {
			parts = append(parts, "-H", "'"+h.Key+": "+h.Value+"'")
		}
	}
	switch req.Auth.Type {
	case "bearer":
		if req.Auth.Token != "" {
			parts = append(parts, "-H", "'Authorization: Bearer "+req.Auth.Token+"'")
		}
	case "basic":
		if req.Auth.User != "" {
			parts = append(parts, "-u", "'"+req.Auth.User+":"+req.Auth.Pass+"'")
		}
	}
	if req.Body.Raw != "" {
		parts = append(parts, "--data-raw", "'"+req.Body.Raw+"'")
	}
	rawURL := req.URL
	if rawURL == "" {
		rawURL = "https://example.com"
	}
	parts = append(parts, "'"+rawURL+"'")
	return strings.Join(parts, " \\\n  ")
}

func (m Model) doExternalEditor() (tea.Model, tea.Cmd) {
	body := m.editor.BodyValue()
	tmp, err := os.CreateTemp("", "gopull-body-*.txt")
	if err != nil {
		m.status = "could not create temp file: " + err.Error()
		return m, clearStatusCmd()
	}
	if _, err := tmp.WriteString(body); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		m.status = "could not write temp file: " + err.Error()
		return m, clearStatusCmd()
	}
	tmp.Close()

	editor := resolveEditor()
	tmpPath := tmp.Name()
	cmd := externalEditorCmd(editor, tmpPath)
	return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
		return externalEditorDoneMsg{tmpFile: tmpPath, err: err}
	})
}

func sanitizeFilename(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, s)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func resolveEditor() string {
	if e := os.Getenv("EDITOR"); e != "" {
		return e
	}
	if e := os.Getenv("VISUAL"); e != "" {
		return e
	}
	if runtime.GOOS == "windows" {
		return "notepad"
	}
	return "nano"
}

func externalEditorCmd(editor, file string) *exec.Cmd {
	// Editors like "code --wait" come as a single string with arguments.
	parts := strings.Fields(editor)
	if len(parts) == 1 {
		return exec.Command(editor, file)
	}
	args := append(parts[1:], file)
	return exec.Command(parts[0], args...)
}

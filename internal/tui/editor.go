package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/d0mkaaa/gopull/internal/curlparse"
	"github.com/d0mkaaa/gopull/internal/store"
)

type editorTab int

const (
	etBody editorTab = iota
	etParams
	etHeaders
	etAuth
	etTests
	etOpts
)

var editorTabNames = []string{"body", "params", "headers", "auth", "tests", "opts"}

type authKind int

const (
	akNone authKind = iota
	akBearer
	akBasic
)

var authKindNames = []string{"none", "bearer", "basic"}

type editorInner int

const (
	eiMethod editorInner = iota
	eiURL
	eiContent
)

type EditorModel struct {
	method       textinput.Model
	url          textinput.Model
	bodyInput    textarea.Model
	queryInput   textarea.Model
	pathInput    textarea.Model
	headersInput textarea.Model
	testsInput   textarea.Model
	tokenInput   textinput.Model
	userInput    textinput.Model
	passInput    textinput.Model

	tab         editorTab
	bodyMode    string // raw, form, graphql, multipart, file
	authKind    authKind
	authField   int // 0=first, 1=second; -1=type selector
	paramsField int // 0=query, 1=path
	inner       editorInner
	focused     bool

	methodCustom  bool   // true when user is typing a non-standard method
	lastStdMethod string // method value before entering custom mode

	optsField        int // 0=skipVerify, 1=disableRedirects, 2=cookieJar, 3=proxyURL, 4=timeout, 5=CA, 6=cert, 7=key
	skipVerify       bool
	disableRedirects bool
	useCookieJar     bool
	proxyInput       textinput.Model
	perReqTimeout    textinput.Model
	caBundleInput    textinput.Model
	clientCertInput  textinput.Model
	clientKeyInput   textinput.Model

	requestID    string
	collectionID string
	requestName  string

	width  int
	height int
}

func newEditor(w, h int) EditorModel {
	m := textinput.New()
	m.Placeholder = "GET"
	m.SetValue("GET")
	m.Width = 8
	m.CharLimit = 10

	u := textinput.New()
	u.Placeholder = "https://httpbin.org/get  (or paste a curl command and press tab)"
	u.CharLimit = 1000

	b := textarea.New()
	b.Placeholder = "Request body..."
	b.ShowLineNumbers = false
	b.Prompt = ""

	q := textarea.New()
	q.Placeholder = "page=1\n# debug=true"
	q.ShowLineNumbers = false
	q.Prompt = ""

	p := textarea.New()
	p.Placeholder = "id=123\norg=acme"
	p.ShowLineNumbers = false
	p.Prompt = ""

	h2 := textarea.New()
	h2.Placeholder = "Content-Type: application/json\nAuthorization: Bearer token\n# X-Disabled-Header: value"
	h2.ShowLineNumbers = false
	h2.Prompt = ""

	t := textarea.New()
	t.Placeholder = "assert status == 200\nassert body contains \"id\"\nset TOKEN = $.data.access_token"
	t.ShowLineNumbers = false
	t.Prompt = ""

	tok := textinput.New()
	tok.Placeholder = "token"
	tok.EchoMode = textinput.EchoPassword
	tok.EchoCharacter = '*'

	usr := textinput.New()
	usr.Placeholder = "username"

	pass := textinput.New()
	pass.Placeholder = "password"
	pass.EchoMode = textinput.EchoPassword
	pass.EchoCharacter = '*'

	proxy := textinput.New()
	proxy.Placeholder = "http://proxy:8080"
	proxy.CharLimit = 256

	tout := textinput.New()
	tout.Placeholder = "0"
	tout.CharLimit = 6

	ca := textinput.New()
	ca.Placeholder = "/path/to/ca.pem"
	ca.CharLimit = 1024

	cert := textinput.New()
	cert.Placeholder = "/path/to/client.pem"
	cert.CharLimit = 1024

	key := textinput.New()
	key.Placeholder = "/path/to/client.key"
	key.CharLimit = 1024

	ed := EditorModel{
		method:          m,
		url:             u,
		bodyInput:       b,
		queryInput:      q,
		pathInput:       p,
		headersInput:    h2,
		testsInput:      t,
		tokenInput:      tok,
		userInput:       usr,
		passInput:       pass,
		proxyInput:      proxy,
		perReqTimeout:   tout,
		caBundleInput:   ca,
		clientCertInput: cert,
		clientKeyInput:  key,
		bodyMode:        "raw",
		authField:       -1,
		width:           w,
		height:          h,
	}
	return ed.setSize(w, h).RefreshTheme()
}

// applyTextareaTheme stamps the current theme colors onto a textarea widget.
func applyTextareaTheme(t *textarea.Model) {
	base := lipgloss.NewStyle()
	if colorBg != "" {
		base = base.Background(colorBg)
	}
	text := lipgloss.NewStyle().Foreground(colorText)
	placeholder := lipgloss.NewStyle().Foreground(colorMuted)
	cursorLine := lipgloss.NewStyle().Foreground(colorText)
	endOfBuffer := lipgloss.NewStyle().Foreground(colorBorder)
	prompt := lipgloss.NewStyle().Foreground(colorSubtle)
	blurredText := lipgloss.NewStyle().Foreground(colorSubtle)
	blurredPlaceholder := lipgloss.NewStyle().Foreground(colorBorder)
	cursor := lipgloss.NewStyle().Foreground(colorAccent)
	if colorBg != "" {
		text = text.Background(colorBg)
		placeholder = placeholder.Background(colorBg)
		cursorLine = cursorLine.Background(colorBg)
		endOfBuffer = endOfBuffer.Background(colorBg)
		prompt = prompt.Background(colorBg)
		blurredText = blurredText.Background(colorBg)
		blurredPlaceholder = blurredPlaceholder.Background(colorBg)
		cursor = cursor.Background(colorBg)
	}

	t.FocusedStyle = textarea.Style{
		Base:        base,
		Text:        text,
		Placeholder: placeholder,
		CursorLine:  cursorLine,
		EndOfBuffer: endOfBuffer,
		Prompt:      prompt,
	}
	t.BlurredStyle = textarea.Style{
		Base:        base,
		Text:        blurredText,
		Placeholder: blurredPlaceholder,
		CursorLine:  base,
		EndOfBuffer: endOfBuffer,
		Prompt:      blurredPlaceholder,
	}
	t.Cursor.Style = cursor
}

// applyTextinputTheme stamps the current theme colors onto a textinput widget.
func applyTextinputTheme(t *textinput.Model) {
	placeholder := lipgloss.NewStyle().Foreground(colorMuted)
	text := lipgloss.NewStyle().Foreground(colorText)
	prompt := lipgloss.NewStyle().Foreground(colorSubtle)
	cursor := lipgloss.NewStyle().Foreground(colorAccent)
	if colorBg != "" {
		placeholder = placeholder.Background(colorBg)
		text = text.Background(colorBg)
		prompt = prompt.Background(colorBg)
		cursor = cursor.Background(colorBg)
	}
	t.PlaceholderStyle = placeholder
	t.TextStyle = text
	t.PromptStyle = prompt
	t.Cursor.Style = cursor
}

// RefreshTheme re-applies the current theme palette to every input widget.
// Call this after applyTheme() to keep input colors in sync.
func (m EditorModel) RefreshTheme() EditorModel {
	applyTextareaTheme(&m.bodyInput)
	applyTextareaTheme(&m.queryInput)
	applyTextareaTheme(&m.pathInput)
	applyTextareaTheme(&m.headersInput)
	applyTextareaTheme(&m.testsInput)
	applyTextinputTheme(&m.method)
	applyTextinputTheme(&m.url)
	applyTextinputTheme(&m.tokenInput)
	applyTextinputTheme(&m.userInput)
	applyTextinputTheme(&m.passInput)
	applyTextinputTheme(&m.proxyInput)
	applyTextinputTheme(&m.perReqTimeout)
	applyTextinputTheme(&m.caBundleInput)
	applyTextinputTheme(&m.clientCertInput)
	applyTextinputTheme(&m.clientKeyInput)
	return m
}

func (m EditorModel) Update(msg tea.Msg) (EditorModel, tea.Cmd) {
	if !m.focused {
		return m, nil
	}

	if km, ok := msg.(tea.KeyMsg); ok {
		switch {
		case key.Matches(km, keys.Next):
			return m.tabForward()
		case key.Matches(km, keys.Prev):
			return m.tabBackward()
		case key.Matches(km, keys.TabLeft):
			m = m.prevTab()
			return m, nil
		case key.Matches(km, keys.TabRight):
			m = m.nextTab()
			return m, nil
		case key.Matches(km, keys.BodyMode) && m.inner == eiContent && m.tab == etBody:
			switch m.bodyMode {
			case "raw":
				m.bodyMode = "form"
				m.bodyInput.Placeholder = "Key: Value\nKey2: Value2"
			case "form":
				m.bodyMode = "graphql"
				if strings.TrimSpace(m.bodyInput.Value()) == "" {
					m.bodyInput.SetValue("{\n  \"query\": \"\",\n  \"variables\": {}\n}")
				}
				m.bodyInput.Placeholder = "{\"query\": \"\", \"variables\": {}}"
			case "graphql":
				m.bodyMode = "multipart"
				m.bodyInput.Placeholder = "name=value\nfile avatar=C:\\path\\avatar.png"
			case "multipart":
				m.bodyMode = "file"
				m.bodyInput.Placeholder = "C:\\path\\body.json"
			default:
				m.bodyMode = "raw"
				m.bodyInput.Placeholder = "Request body..."
			}
			return m, nil

		case key.Matches(km, keys.PrettyPrint) && m.inner == eiContent && m.tab == etBody:
			v := m.bodyInput.Value()
			if formatted := prettyJSON([]byte(v)); formatted != v {
				m.bodyInput.SetValue(formatted)
			}
			return m, nil
		}

		if m.inner == eiMethod {
			switch {
			case (km.Type == tea.KeyUp || km.Type == tea.KeyDown) && !m.methodCustom:
				m.cycleMethod(km.Type == tea.KeyDown)
				return m, nil

			case km.Type == tea.KeySpace && !m.methodCustom:
				m.cycleMethod(true)
				return m, nil

			case km.Type == tea.KeyRunes && !m.methodCustom:
				// First letter key: enter custom input mode.
				m.lastStdMethod = strings.ToUpper(strings.TrimSpace(m.method.Value()))
				if m.lastStdMethod == "" {
					m.lastStdMethod = "GET"
				}
				m.methodCustom = true
				m.method.SetValue(strings.ToUpper(string(km.Runes)))
				return m, nil

			case km.Type == tea.KeyEsc && m.methodCustom:
				m.methodCustom = false
				m.method.SetValue(m.lastStdMethod)
				return m, nil
			}
		}

		if m.inner == eiContent && m.tab == etAuth && m.authField < 0 {
			switch km.Type {
			case tea.KeyLeft:
				m.authKind = authKind((int(m.authKind) + len(authKindNames) - 1) % len(authKindNames))
				return m, nil
			case tea.KeyRight:
				m.authKind = authKind((int(m.authKind) + 1) % len(authKindNames))
				return m, nil
			case tea.KeyEnter:
				if m.authKind != akNone {
					m.authField = 0
					m = m.focusAuthField()
				}
				return m, nil
			}
		}

		if m.inner == eiContent && m.tab == etParams {
			switch km.Type {
			case tea.KeyUp, tea.KeyDown:
				if m.paramsField == 0 {
					m.paramsField = 1
				} else {
					m.paramsField = 0
				}
				m = m.focusContent()
				return m, nil
			}
		}

		if m.inner == eiContent && m.tab == etOpts {
			switch km.Type {
			case tea.KeyUp:
				if m.optsField > 0 {
					m.optsField--
					m = m.focusOptsField()
				}
				return m, nil
			case tea.KeyDown:
				if m.optsField < 7 {
					m.optsField++
					m = m.focusOptsField()
				}
				return m, nil
			case tea.KeyLeft, tea.KeyRight:
				switch m.optsField {
				case 0:
					m.skipVerify = !m.skipVerify
				case 1:
					m.disableRedirects = !m.disableRedirects
				case 2:
					m.useCookieJar = !m.useCookieJar
				}
				return m, nil
			}
		}

		// detect curl paste: on Tab (leaving URL field) or Enter (confirm in URL field)
		if m.inner == eiURL && (km.Type == tea.KeyTab || km.Type == tea.KeyEnter) {
			if val := strings.TrimSpace(m.url.Value()); curlparse.LooksLikeCurl(val) {
				if parsed, err := curlparse.Parse(val); err == nil {
					return m.Load(&parsed, m.collectionID), nil
				}
			}
		}
	}

	var cmd tea.Cmd
	switch m.inner {
	case eiMethod:
		m.method, cmd = m.method.Update(msg)
		if m.methodCustom {
			if v := strings.ToUpper(m.method.Value()); v != m.method.Value() {
				m.method.SetValue(v)
			}
		}
	case eiURL:
		m.url, cmd = m.url.Update(msg)
	case eiContent:
		switch m.tab {
		case etBody:
			m.bodyInput, cmd = m.bodyInput.Update(msg)
		case etParams:
			switch m.paramsField {
			case 1:
				m.pathInput, cmd = m.pathInput.Update(msg)
			default:
				m.queryInput, cmd = m.queryInput.Update(msg)
			}
		case etHeaders:
			m.headersInput, cmd = m.headersInput.Update(msg)
		case etTests:
			m.testsInput, cmd = m.testsInput.Update(msg)
		case etAuth:
			if m.authField >= 0 {
				cmd = m.updateAuthInput(msg)
			}
		case etOpts:
			switch m.optsField {
			case 3:
				m.proxyInput, cmd = m.proxyInput.Update(msg)
			case 4:
				m.perReqTimeout, cmd = m.perReqTimeout.Update(msg)
			case 5:
				m.caBundleInput, cmd = m.caBundleInput.Update(msg)
			case 6:
				m.clientCertInput, cmd = m.clientCertInput.Update(msg)
			case 7:
				m.clientKeyInput, cmd = m.clientKeyInput.Update(msg)
			}
		}
	}
	return m, cmd
}

func (m EditorModel) tabForward() (EditorModel, tea.Cmd) {
	switch m.inner {
	case eiMethod:
		m.inner = eiURL
		m.method.Blur()
		m.url.Focus()
	case eiURL:
		m.inner = eiContent
		m.url.Blur()
		m = m.focusContent()
	case eiContent:
		if m.tab == etAuth && m.authField >= 0 {
			max := 0
			if m.authKind == akBasic {
				max = 1
			}
			if m.authField < max {
				m.authField++
				m = m.focusAuthField()
				return m, nil
			}
		}
		m = m.blurContent()
		return m, func() tea.Msg { return focusResponseMsg{} }
	}
	return m, nil
}

func (m EditorModel) tabBackward() (EditorModel, tea.Cmd) {
	switch m.inner {
	case eiURL:
		m.inner = eiMethod
		m.url.Blur()
		m.method.Focus()
	case eiContent:
		if m.tab == etAuth && m.authField > 0 {
			m.authField--
			m = m.focusAuthField()
			return m, nil
		}
		m.inner = eiURL
		m = m.blurContent()
		m.url.Focus()
	case eiMethod:
		m = m.blurContent()
		return m, func() tea.Msg { return focusSidebarMsg{} }
	}
	return m, nil
}

func (m EditorModel) prevTab() EditorModel {
	if m.tab > 0 {
		m.tab--
	}
	m.authField = -1
	if m.inner == eiContent {
		return m.focusContent()
	}
	return m
}

func (m EditorModel) nextTab() EditorModel {
	if int(m.tab) < len(editorTabNames)-1 {
		m.tab++
	}
	m.authField = -1
	if m.inner == eiContent {
		return m.focusContent()
	}
	return m
}

func (m EditorModel) focusContent() EditorModel {
	m.bodyInput.Blur()
	m.queryInput.Blur()
	m.pathInput.Blur()
	m.headersInput.Blur()
	m.testsInput.Blur()
	m.tokenInput.Blur()
	m.userInput.Blur()
	m.passInput.Blur()
	m.proxyInput.Blur()
	m.perReqTimeout.Blur()
	switch m.tab {
	case etBody:
		m.bodyInput.Focus()
	case etParams:
		if m.paramsField == 1 {
			m.pathInput.Focus()
		} else {
			m.queryInput.Focus()
		}
	case etHeaders:
		m.headersInput.Focus()
	case etTests:
		m.testsInput.Focus()
	case etAuth:
		if m.authField >= 0 {
			m = m.focusAuthField()
		}
	case etOpts:
		m = m.focusOptsField()
	}
	return m
}

func (m EditorModel) blurContent() EditorModel {
	m.bodyInput.Blur()
	m.queryInput.Blur()
	m.pathInput.Blur()
	m.headersInput.Blur()
	m.testsInput.Blur()
	m.tokenInput.Blur()
	m.userInput.Blur()
	m.passInput.Blur()
	m.proxyInput.Blur()
	m.perReqTimeout.Blur()
	return m
}

func (m EditorModel) focusOptsField() EditorModel {
	m.proxyInput.Blur()
	m.perReqTimeout.Blur()
	m.caBundleInput.Blur()
	m.clientCertInput.Blur()
	m.clientKeyInput.Blur()
	switch m.optsField {
	case 3:
		m.proxyInput.Focus()
	case 4:
		m.perReqTimeout.Focus()
	case 5:
		m.caBundleInput.Focus()
	case 6:
		m.clientCertInput.Focus()
	case 7:
		m.clientKeyInput.Focus()
	}
	return m
}

func (m EditorModel) focusAuthField() EditorModel {
	m.tokenInput.Blur()
	m.userInput.Blur()
	m.passInput.Blur()
	switch m.authKind {
	case akBearer:
		m.tokenInput.Focus()
	case akBasic:
		if m.authField == 0 {
			m.userInput.Focus()
		} else {
			m.passInput.Focus()
		}
	}
	return m
}

func (m *EditorModel) updateAuthInput(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	switch m.authKind {
	case akBearer:
		m.tokenInput, cmd = m.tokenInput.Update(msg)
	case akBasic:
		if m.authField == 0 {
			m.userInput, cmd = m.userInput.Update(msg)
		} else {
			m.passInput, cmd = m.passInput.Update(msg)
		}
	}
	return cmd
}

func (m *EditorModel) cycleMethod(forward bool) {
	current := strings.ToUpper(strings.TrimSpace(m.method.Value()))
	idx := 0
	for i, v := range methods {
		if v == current {
			idx = i
			break
		}
	}
	if forward {
		idx = (idx + 1) % len(methods)
	} else {
		idx = (idx + len(methods) - 1) % len(methods)
	}
	m.method.SetValue(methods[idx])
}

func (m EditorModel) View() string {
	mv := m.method.Value()
	if mv == "" {
		mv = "GET"
	}

	methodHint := ""
	if m.inner == eiMethod && m.focused {
		if m.methodCustom {
			methodHint = hint.Render("  enter confirm  esc cancel")
		} else {
			methodHint = hint.Render("  up/down space cycle  letter custom")
		}
	}
	badge := methodBadge(mv)
	if lipgloss.Width(badge)+lipgloss.Width(methodHint)+10 > m.width {
		methodHint = ""
	}
	url := m.url
	url.Width = max(1, m.width-lipgloss.Width(badge)-lipgloss.Width(methodHint)-2)
	topRow := lipgloss.JoinHorizontal(lipgloss.Center,
		badge, methodHint, "  ", url.View(),
	)

	focusedContent := m.inner == eiContent && m.focused
	tabNames := editorTabNames
	if m.width < 40 {
		tabNames = []string{"body", "par", "hdr", "auth", "test", "opt"}
	}
	tabs := renderTabs(tabNames, int(m.tab), focusedContent)

	var content string
	switch m.tab {
	case etBody:
		var modeLabel string
		switch m.bodyMode {
		case "form":
			modeLabel = formMode.Render("form")
		case "graphql":
			modeLabel = formMode.Render("graphql")
		case "multipart":
			modeLabel = formMode.Render("multipart")
		case "file":
			modeLabel = formMode.Render("file")
		default:
			modeLabel = hint.Render("raw")
		}
		content = lipgloss.JoinVertical(lipgloss.Left,
			modeLabel+hint.Render("  alt+m toggle"),
			m.bodyInput.View(),
		)
	case etParams:
		queryLabel := hint.Render("query params")
		pathLabel := hint.Render("path params")
		if m.paramsField == 0 && focusedContent {
			queryLabel = tabActive.Render("query params")
		}
		if m.paramsField == 1 && focusedContent {
			pathLabel = tabActive.Render("path params")
		}
		content = lipgloss.JoinVertical(lipgloss.Left,
			queryLabel+hint.Render("  key=value, # disabled"),
			m.queryInput.View(),
			pathLabel+hint.Render("  :id placeholders in URL"),
			m.pathInput.View(),
		)
	case etHeaders:
		content = m.headersInput.View()
	case etTests:
		content = m.testsInput.View()
	case etAuth:
		content = m.viewAuth()
	case etOpts:
		content = m.viewOpts()
	}

	return lipgloss.JoinVertical(lipgloss.Left, topRow, mutedRule(m.ruleWidth()), tabs, content)
}

func (m EditorModel) ruleWidth() int {
	return max(0, m.width-2)
}

func (m EditorModel) viewAuth() string {
	var typeParts []string
	for i, name := range authKindNames {
		if authKind(i) == m.authKind {
			typeParts = append(typeParts, tabActive.Render(name))
		} else {
			typeParts = append(typeParts, tabInactive.Render(name))
		}
	}
	if m.authField < 0 && m.focused {
		typeParts = append([]string{hint.Render("<--> ")}, typeParts...)
	}
	typeRow := strings.Join(typeParts, "  ")

	var fields string
	switch m.authKind {
	case akNone:
		fields = hint.Render("no auth")
	case akBearer:
		fields = lipgloss.JoinVertical(lipgloss.Left,
			hint.Render("token"), m.tokenInput.View(),
		)
	case akBasic:
		fields = lipgloss.JoinVertical(lipgloss.Left,
			hint.Render("username"), m.userInput.View(),
			hint.Render("password"), m.passInput.View(),
		)
	}
	return lipgloss.JoinVertical(lipgloss.Left, typeRow, "", fields)
}

func (m EditorModel) viewOpts() string {
	boolStr := func(v bool) string {
		if v {
			return tabActive.Render("yes")
		}
		return hint.Render("no")
	}
	focused := m.inner == eiContent && m.focused

	line := func(label string, val string, active bool) string {
		sel := "  "
		if active && focused {
			sel = hint.Render("-> ")
		}
		return sel + hint.Render(fmt.Sprintf("%-20s", label)) + val
	}

	skipVal := boolStr(m.skipVerify) + hint.Render("  <- ->")
	redirVal := boolStr(m.disableRedirects) + hint.Render("  <- ->")
	cookieVal := boolStr(m.useCookieJar) + hint.Render("  <- ->")

	return lipgloss.JoinVertical(lipgloss.Left,
		line("skip TLS verify", skipVal, m.optsField == 0),
		line("disable redirects", redirVal, m.optsField == 1),
		line("cookie jar", cookieVal, m.optsField == 2),
		line("proxy URL", m.proxyInput.View(), m.optsField == 3),
		line("timeout override", m.perReqTimeout.View()+hint.Render(" sec  (0 = global)"), m.optsField == 4),
		line("CA bundle", m.caBundleInput.View(), m.optsField == 5),
		line("client cert", m.clientCertInput.View(), m.optsField == 6),
		line("client key", m.clientKeyInput.View(), m.optsField == 7),
	)
}

// IsEditingContent returns true when the user is actively typing in a textarea
// or text field - used to suppress global single-character shortcuts like `?`.
func (m EditorModel) IsEditingContent() bool {
	return m.focused && m.inner == eiContent
}

func (m EditorModel) Focus() EditorModel {
	m.focused = true
	m.inner = eiMethod
	m.methodCustom = false
	m.method.Focus()
	return m
}

func (m EditorModel) Blur() EditorModel {
	m.focused = false
	m.method.Blur()
	m.url.Blur()
	return m.blurContent()
}

func (m EditorModel) setSize(w, h int) EditorModel {
	m.width = w
	m.height = h
	inner := max(1, w)
	m.url.Width = max(10, inner-16)
	contentH := max(3, h-7)
	m.bodyInput.SetWidth(inner)
	m.bodyInput.SetHeight(contentH)
	paramH := max(2, (contentH-2)/2)
	m.queryInput.SetWidth(inner)
	m.queryInput.SetHeight(paramH)
	m.pathInput.SetWidth(inner)
	m.pathInput.SetHeight(max(2, contentH-paramH-2))
	m.headersInput.SetWidth(inner)
	m.headersInput.SetHeight(contentH)
	m.testsInput.SetWidth(inner)
	m.testsInput.SetHeight(contentH)
	tokW := max(20, inner-2)
	m.tokenInput.Width = tokW
	m.userInput.Width = tokW
	m.passInput.Width = tokW
	m.proxyInput.Width = max(20, inner-16)
	m.perReqTimeout.Width = 8
	m.caBundleInput.Width = max(20, inner-16)
	m.clientCertInput.Width = max(20, inner-16)
	m.clientKeyInput.Width = max(20, inner-16)
	return m
}

func (m EditorModel) Load(r *store.Request, collID string) EditorModel {
	m.method.SetValue(r.Method)
	m.url.SetValue(r.URL)
	m.bodyInput.SetValue(r.Body.Raw)
	m.queryInput.SetValue(formatParams(r.Query))
	m.pathInput.SetValue(formatParams(r.Path))
	m.bodyMode = r.Body.Mode
	if m.bodyMode == "" {
		m.bodyMode = "raw"
	}
	m.testsInput.SetValue(r.Tests)
	m.requestID = r.ID
	m.collectionID = collID
	m.requestName = r.Name

	var sb strings.Builder
	for _, h := range r.Headers {
		if h.Enabled {
			sb.WriteString(h.Key + ": " + h.Value + "\n")
		} else {
			sb.WriteString("# " + h.Key + ": " + h.Value + "\n")
		}
	}
	m.headersInput.SetValue(strings.TrimRight(sb.String(), "\n"))

	switch r.Auth.Type {
	case "bearer":
		m.authKind = akBearer
		m.tokenInput.SetValue(r.Auth.Token)
	case "basic":
		m.authKind = akBasic
		m.userInput.SetValue(r.Auth.User)
		m.passInput.SetValue(r.Auth.Pass)
	default:
		m.authKind = akNone
	}
	m.authField = -1

	m.skipVerify = r.Options.SkipTLSVerify
	m.disableRedirects = r.Options.DisableRedirects
	m.useCookieJar = r.Options.UseCookieJar
	m.proxyInput.SetValue(r.Options.ProxyURL)
	m.caBundleInput.SetValue(r.Options.CABundlePath)
	m.clientCertInput.SetValue(r.Options.ClientCertPath)
	m.clientKeyInput.SetValue(r.Options.ClientKeyPath)
	if r.Options.TimeoutSecs > 0 {
		m.perReqTimeout.SetValue(fmt.Sprintf("%d", r.Options.TimeoutSecs))
	} else {
		m.perReqTimeout.SetValue("")
	}
	return m
}

func (m EditorModel) BuildRequest() store.Request {
	r := store.Request{
		ID:    m.requestID,
		Name:  m.requestName,
		Tests: m.testsInput.Value(),
		Body:  store.Body{Mode: m.bodyMode, Raw: m.bodyInput.Value()},
	}
	r.Method = strings.ToUpper(strings.TrimSpace(m.method.Value()))
	r.URL = strings.TrimSpace(m.url.Value())
	if r.Method == "" {
		r.Method = "GET"
	}

	for _, line := range strings.Split(m.headersInput.Value(), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		enabled := true
		if strings.HasPrefix(line, "#") {
			enabled = false
			line = strings.TrimSpace(strings.TrimPrefix(line, "#"))
		}
		if line == "" {
			continue
		}
		idx := strings.Index(line, ":")
		if idx < 0 {
			continue
		}
		r.Headers = append(r.Headers, store.Header{
			Key:     strings.TrimSpace(line[:idx]),
			Value:   strings.TrimSpace(line[idx+1:]),
			Enabled: enabled,
		})
	}

	switch m.authKind {
	case akBearer:
		r.Auth = store.Auth{Type: "bearer", Token: m.tokenInput.Value()}
	case akBasic:
		r.Auth = store.Auth{Type: "basic", User: m.userInput.Value(), Pass: m.passInput.Value()}
	default:
		r.Auth = store.Auth{Type: "none"}
	}

	r.Options.SkipTLSVerify = m.skipVerify
	r.Options.DisableRedirects = m.disableRedirects
	r.Options.UseCookieJar = m.useCookieJar
	r.Options.ProxyURL = m.proxyInput.Value()
	r.Options.CABundlePath = m.caBundleInput.Value()
	r.Options.ClientCertPath = m.clientCertInput.Value()
	r.Options.ClientKeyPath = m.clientKeyInput.Value()
	r.Query = parseParams(m.queryInput.Value())
	r.Path = parseParams(m.pathInput.Value())
	if v := m.perReqTimeout.Value(); v != "" && v != "0" {
		n := 0
		fmt.Sscanf(v, "%d", &n)
		r.Options.TimeoutSecs = n
	}
	return r
}

func parseParams(s string) []store.Param {
	var out []store.Param
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		enabled := true
		if strings.HasPrefix(line, "#") {
			enabled = false
			line = strings.TrimSpace(strings.TrimPrefix(line, "#"))
		}
		idx := strings.Index(line, "=")
		if idx < 0 {
			idx = strings.Index(line, ":")
		}
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		if key == "" {
			continue
		}
		out = append(out, store.Param{
			Key:     key,
			Value:   strings.TrimSpace(line[idx+1:]),
			Enabled: enabled,
		})
	}
	return out
}

func formatParams(params []store.Param) string {
	var b strings.Builder
	for _, p := range params {
		if p.Key == "" {
			continue
		}
		if !p.Enabled {
			b.WriteString("# ")
		}
		b.WriteString(p.Key)
		b.WriteString("=")
		b.WriteString(p.Value)
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m EditorModel) TestsScript() string {
	return m.testsInput.Value()
}

// BodyValue returns the current raw body text.
func (m EditorModel) BodyValue() string {
	return m.bodyInput.Value()
}

// SetBody replaces the body textarea content.
func (m EditorModel) SetBody(s string) EditorModel {
	m.bodyInput.SetValue(s)
	return m
}

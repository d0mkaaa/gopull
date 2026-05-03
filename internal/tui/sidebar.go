package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/d0mkaaa/gopull/internal/store"
)

type sidebarMode int

const (
	modeCols sidebarMode = iota
	modeReqs
)

type collItem struct{ c *store.Collection }

func (i collItem) Title() string {
	return i.c.Name
}
func (i collItem) Description() string {
	n := len(i.c.Requests)
	if n == 1 {
		return "1 request"
	}
	return fmt.Sprintf("%d requests", n)
}
func (i collItem) FilterValue() string { return i.c.Name }

type reqItem struct {
	r      *store.Request
	collID string
}

func (i reqItem) Title() string { return i.r.Name }
func (i reqItem) Description() string {
	m := strings.ToUpper(i.r.Method)
	if m == "" {
		m = "GET"
	}
	return fmt.Sprintf("%-8s%s", m, i.r.URL)
}
func (i reqItem) FilterValue() string { return i.r.Name }

type backItem struct{}

func (i backItem) Title() string       { return "<- collections" }
func (i backItem) Description() string { return "" }
func (i backItem) FilterValue() string { return "" }

type SidebarModel struct {
	list         list.Model
	mode         sidebarMode
	collections  []*store.Collection
	activeCollID string
	focused      bool

	pendingDeleteID string

	// rename state
	renaming       bool
	renameInput    textinput.Model
	renameTargetID string
	renameIsColl   bool // renaming a collection vs a request

	width  int
	height int
}

func newSidebar(w, h int) SidebarModel {
	d := list.NewDefaultDelegate()
	d.Styles.SelectedTitle = d.Styles.SelectedTitle.
		Foreground(colorAccent).
		BorderForeground(colorAccent)
	d.Styles.SelectedDesc = d.Styles.SelectedDesc.
		Foreground(colorSubtle).
		BorderForeground(colorAccent)
	d.Styles.NormalTitle = d.Styles.NormalTitle.Foreground(colorText)
	d.Styles.NormalDesc = d.Styles.NormalDesc.Foreground(colorMuted)
	d.ShowDescription = true

	l := list.New(nil, d, w, h)
	l.SetShowTitle(false)
	l.SetShowHelp(false)
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.KeyMap.Quit.SetEnabled(false)
	l.Styles.StatusBar = lipgloss.NewStyle().Foreground(colorMuted).MarginLeft(2)

	ri := textinput.New()
	ri.CharLimit = 120
	ri.Prompt = ""

	return SidebarModel{list: l, mode: modeCols, width: w, height: h, renameInput: ri}
}

func (m SidebarModel) Update(msg tea.Msg) (SidebarModel, tea.Cmd) {
	if !m.focused {
		return m, nil
	}

	// Rename mode: route everything to the text input.
	if m.renaming {
		if km, ok := msg.(tea.KeyMsg); ok {
			switch km.Type {
			case tea.KeyEnter:
				name := strings.TrimSpace(m.renameInput.Value())
				m.renaming = false
				if name == "" {
					return m, nil
				}
				if m.renameIsColl {
					return m, func() tea.Msg {
						return renameCollectionMsg{collID: m.renameTargetID, name: name}
					}
				}
				return m, func() tea.Msg {
					return renameRequestMsg{collID: m.activeCollID, reqID: m.renameTargetID, name: name}
				}
			case tea.KeyEsc:
				m.renaming = false
				return m, nil
			}
		}
		var cmd tea.Cmd
		m.renameInput, cmd = m.renameInput.Update(msg)
		return m, cmd
	}

	if km, ok := msg.(tea.KeyMsg); ok && m.list.FilterState() != list.Filtering {
		switch km.Type {
		case tea.KeyEnter:
			m.pendingDeleteID = ""
			return m.handleEnter()

		case tea.KeyEsc:
			if m.pendingDeleteID != "" {
				m.pendingDeleteID = ""
				return m, nil
			}
			if m.mode == modeReqs {
				return m.showCollections(), nil
			}

		case tea.KeyTab:
			m.pendingDeleteID = ""
			return m, func() tea.Msg { return focusEditorMsg{} }

		case tea.KeyCtrlD:
			return m.handleDuplicate()

		case tea.KeyCtrlJ:
			return m.handleMove(1)

		case tea.KeyCtrlK:
			return m.handleMove(-1)

		case tea.KeyRunes:
			switch km.String() {
			case "d":
				return m.handleDelete()
			case "n":
				return m.handleRename()
			case "r":
				if m.mode == modeCols {
					if ci, ok := m.list.SelectedItem().(collItem); ok {
						return m, func() tea.Msg { return runCollectionMsg{collID: ci.c.ID} }
					}
				}
			}
			m.pendingDeleteID = ""

		default:
			m.pendingDeleteID = ""
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m SidebarModel) handleRename() (SidebarModel, tea.Cmd) {
	item := m.list.SelectedItem()
	if item == nil {
		return m, nil
	}
	switch i := item.(type) {
	case collItem:
		m.renameTargetID = i.c.ID
		m.renameIsColl = true
		m.renameInput.SetValue(i.c.Name)
		m.renameInput.CursorEnd()
	case reqItem:
		m.renameTargetID = i.r.ID
		m.renameIsColl = false
		m.renameInput.SetValue(i.r.Name)
		m.renameInput.CursorEnd()
	default:
		return m, nil
	}
	m.pendingDeleteID = ""
	m.renaming = true
	return m, m.renameInput.Focus()
}

func (m SidebarModel) handleDuplicate() (SidebarModel, tea.Cmd) {
	if m.mode != modeReqs {
		return m, nil
	}
	ri, ok := m.list.SelectedItem().(reqItem)
	if !ok {
		return m, nil
	}
	collID := m.activeCollID
	reqID := ri.r.ID
	return m, func() tea.Msg { return duplicateRequestMsg{collID: collID, reqID: reqID} }
}

func (m SidebarModel) handleMove(delta int) (SidebarModel, tea.Cmd) {
	if m.mode != modeReqs {
		return m, nil
	}
	ri, ok := m.list.SelectedItem().(reqItem)
	if !ok {
		return m, nil
	}
	collID := m.activeCollID
	reqID := ri.r.ID
	return m, func() tea.Msg { return moveRequestMsg{collID: collID, reqID: reqID, delta: delta} }
}

func (m SidebarModel) handleDelete() (SidebarModel, tea.Cmd) {
	item := m.list.SelectedItem()
	if item == nil {
		return m, nil
	}
	switch i := item.(type) {
	case reqItem:
		if m.pendingDeleteID == i.r.ID {
			m.pendingDeleteID = ""
			return m, func() tea.Msg { return deleteRequestMsg{collID: i.collID, reqID: i.r.ID} }
		}
		m.pendingDeleteID = i.r.ID
	case collItem:
		if m.pendingDeleteID == i.c.ID {
			m.pendingDeleteID = ""
			return m, func() tea.Msg { return deleteCollectionMsg{collID: i.c.ID} }
		}
		m.pendingDeleteID = i.c.ID
	}
	return m, nil
}

func (m SidebarModel) handleEnter() (SidebarModel, tea.Cmd) {
	item := m.list.SelectedItem()
	if item == nil {
		return m, nil
	}
	switch i := item.(type) {
	case backItem:
		return m.showCollections(), nil
	case collItem:
		return m.showRequests(i.c), nil
	case reqItem:
		return m, func() tea.Msg { return loadRequestMsg{req: i.r, collID: i.collID} }
	}
	return m, nil
}

func (m SidebarModel) showCollections() SidebarModel {
	m.mode = modeCols
	m.activeCollID = ""
	m.pendingDeleteID = ""
	m.renaming = false
	items := make([]list.Item, len(m.collections))
	for i, c := range m.collections {
		items[i] = collItem{c}
	}
	m.list.SetItems(items)
	return m
}

func (m SidebarModel) showRequests(c *store.Collection) SidebarModel {
	m.mode = modeReqs
	m.activeCollID = c.ID
	m.pendingDeleteID = ""
	m.renaming = false
	items := []list.Item{backItem{}}
	for _, id := range c.Order {
		if r, ok := c.Requests[id]; ok {
			items = append(items, reqItem{r: r, collID: c.ID})
		}
	}
	inOrder := make(map[string]bool, len(c.Order))
	for _, id := range c.Order {
		inOrder[id] = true
	}
	for _, r := range c.Requests {
		if !inOrder[r.ID] {
			items = append(items, reqItem{r: r, collID: c.ID})
		}
	}
	m.list.SetItems(items)
	return m
}

func (m SidebarModel) Refresh(cols []*store.Collection) SidebarModel {
	m.collections = cols
	if m.mode == modeCols {
		return m.showCollections()
	}
	for _, c := range cols {
		if c.ID == m.activeCollID {
			return m.showRequests(c)
		}
	}
	return m.showCollections()
}

func (m SidebarModel) SetSize(w, h int) SidebarModel {
	m.width = w
	m.height = h
	m.list.SetSize(w, h-2)
	m.renameInput.Width = max(10, w-4)
	return m
}

func (m SidebarModel) InReqsMode() bool                 { return m.mode == modeReqs }
func (m SidebarModel) PendingDelete() bool              { return m.pendingDeleteID != "" }
func (m SidebarModel) Collections() []*store.Collection { return m.collections }

func (m SidebarModel) RefreshTheme() SidebarModel {
	d := list.NewDefaultDelegate()
	d.Styles.SelectedTitle = d.Styles.SelectedTitle.
		Foreground(colorAccent).
		BorderForeground(colorAccent)
	d.Styles.SelectedDesc = d.Styles.SelectedDesc.
		Foreground(colorSubtle).
		BorderForeground(colorAccent)
	d.Styles.NormalTitle = d.Styles.NormalTitle.Foreground(colorText)
	d.Styles.NormalDesc = d.Styles.NormalDesc.Foreground(colorMuted)
	if colorBg != "" {
		d.Styles.SelectedTitle = d.Styles.SelectedTitle.Background(colorBg).BorderBackground(colorBg)
		d.Styles.SelectedDesc = d.Styles.SelectedDesc.Background(colorBg).BorderBackground(colorBg)
		d.Styles.NormalTitle = d.Styles.NormalTitle.Background(colorBg)
		d.Styles.NormalDesc = d.Styles.NormalDesc.Background(colorBg)
	}
	d.ShowDescription = true
	m.list.SetDelegate(d)
	sb := lipgloss.NewStyle().Foreground(colorMuted).MarginLeft(2)
	if colorBg != "" {
		sb = sb.Background(colorBg)
	}
	m.list.Styles.StatusBar = sb
	return m
}

func (m SidebarModel) Focus() SidebarModel {
	m.focused = true
	return m
}

func (m SidebarModel) Blur() SidebarModel {
	m.focused = false
	m.pendingDeleteID = ""
	m.renaming = false
	return m
}

func (m SidebarModel) View() string {
	title := "collections"
	if m.mode == modeReqs {
		for _, c := range m.collections {
			if c.ID == m.activeCollID {
				title = c.Name
				break
			}
		}
	}

	if m.renaming {
		what := "request"
		if m.renameIsColl {
			what = "collection"
		}
		return lipgloss.JoinVertical(lipgloss.Left,
			sidebarTitle.Render(title),
			"",
			hint.Render("  rename "+what+":"),
			"  "+m.renameInput.View(),
			"",
			hint.Render("  enter confirm   esc cancel"),
		)
	}

	if len(m.collections) == 0 && m.mode == modeCols {
		return lipgloss.JoinVertical(lipgloss.Left,
			sidebarTitle.Render(title),
			"",
			hint.Render("  no collections yet"),
			"",
			hint.Render("  ctrl+s  save a request"),
			hint.Render("  ctrl+i  import collection"),
		)
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		sidebarTitle.Render(title),
		m.list.View(),
	)
}

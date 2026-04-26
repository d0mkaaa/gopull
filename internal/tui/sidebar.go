package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
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

func (i backItem) Title() string       { return "← collections" }
func (i backItem) Description() string { return "" }
func (i backItem) FilterValue() string { return "" }

type SidebarModel struct {
	list         list.Model
	mode         sidebarMode
	collections  []*store.Collection
	activeCollID string
	focused      bool
	// pending delete confirmation: ID of the item waiting for second 'd'
	pendingDeleteID string
	width           int
	height          int
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

	return SidebarModel{list: l, mode: modeCols, width: w, height: h}
}

func (m SidebarModel) Update(msg tea.Msg) (SidebarModel, tea.Cmd) {
	if !m.focused {
		return m, nil
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
		case tea.KeyRunes:
			if km.String() == "d" {
				return m.handleDelete()
			}
			if km.String() == "r" && m.mode == modeCols {
				item := m.list.SelectedItem()
				if ci, ok := item.(collItem); ok {
					return m, func() tea.Msg { return runCollectionMsg{collID: ci.c.ID} }
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
	return m
}

func (m SidebarModel) InReqsMode() bool             { return m.mode == modeReqs }
func (m SidebarModel) PendingDelete() bool           { return m.pendingDeleteID != "" }
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
	d.ShowDescription = true
	m.list.SetDelegate(d)
	m.list.Styles.StatusBar = lipgloss.NewStyle().Foreground(colorMuted).MarginLeft(2)
	return m
}

func (m SidebarModel) Focus() SidebarModel {
	m.focused = true
	return m
}

func (m SidebarModel) Blur() SidebarModel {
	m.focused = false
	m.pendingDeleteID = ""
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

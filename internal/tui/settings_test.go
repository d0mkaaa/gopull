package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/d0mkaaa/gopull/internal/store"
)

func TestSettingsMaxDisplayValue(t *testing.T) {
	m := newSettings("dark", 30, 5*1024*1024, t.TempDir(), "")
	m.maxDisplayInput.SetValue("12")

	if got := m.MaxDisplayBytesValue(); got != 12*1024*1024 {
		t.Fatalf("MaxDisplayBytesValue = %d", got)
	}
}

func TestSettingsViewShowsWorkspace(t *testing.T) {
	m := newSettings("dark", 30, 5*1024*1024, t.TempDir(), `C:\work\api`)

	if got := m.View(); !strings.Contains(got, "workspace") || !strings.Contains(got, `C:\work\api`) {
		t.Fatalf("workspace missing from settings view:\n%s", got)
	}
}

func TestSettingsTabCyclesToMaxDisplay(t *testing.T) {
	m := newSettings("dark", 30, 5*1024*1024, t.TempDir(), "")

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if next.focus != sfTimeout {
		t.Fatalf("first tab focus = %v", next.focus)
	}
	next, _ = next.Update(tea.KeyMsg{Type: tea.KeyTab})
	if next.focus != sfMaxDisplay {
		t.Fatalf("second tab focus = %v", next.focus)
	}
}

func TestSaveSettingsPersistsMaxDisplay(t *testing.T) {
	st, err := store.NewAt(t.TempDir())
	if err != nil {
		t.Fatalf("NewAt: %v", err)
	}

	cmd := saveSettingsCmd(st, "nord", 42, 9*1024*1024)
	cmd()

	cfg, err := st.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.TimeoutSecs != 42 || cfg.Theme != "nord" || cfg.MaxDisplayBytes != 9*1024*1024 {
		t.Fatalf("config not persisted: %#v", cfg)
	}
}

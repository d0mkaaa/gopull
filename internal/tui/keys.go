package tui

import "github.com/charmbracelet/bubbles/key"

type keyMap struct {
	Send           key.Binding
	Save           key.Binding
	NewRequest     key.Binding
	Next           key.Binding
	Prev           key.Binding
	TabLeft        key.Binding
	TabRight       key.Binding
	ToggleSidebar  key.Binding
	Settings       key.Binding
	EnvPicker      key.Binding
	BodyMode       key.Binding
	PrettyPrint    key.Binding
	Search         key.Binding
	Import         key.Binding
	Export         key.Binding
	Quit           key.Binding
	Palette        key.Binding
	CurlExport     key.Binding
	ExternalEditor key.Binding
}

var keys = keyMap{
	Send: key.NewBinding(
		key.WithKeys("ctrl+r"),
		key.WithHelp("ctrl+r", "send"),
	),
	Save: key.NewBinding(
		key.WithKeys("ctrl+s"),
		key.WithHelp("ctrl+s", "save"),
	),
	// alt+n avoids VS Code "New File" interception of ctrl+n
	NewRequest: key.NewBinding(
		key.WithKeys("alt+n"),
		key.WithHelp("alt+n", "new"),
	),
	Next: key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "next"),
	),
	Prev: key.NewBinding(
		key.WithKeys("shift+tab"),
		key.WithHelp("shift+tab", "prev"),
	),
	TabLeft: key.NewBinding(
		key.WithKeys("["),
		key.WithHelp("[/]", "tab"),
	),
	TabRight: key.NewBinding(
		key.WithKeys("]"),
		key.WithHelp("[/]", "tab"),
	),
	// alt+b avoids VS Code "Toggle Sidebar" interception of ctrl+b
	ToggleSidebar: key.NewBinding(
		key.WithKeys("alt+b"),
		key.WithHelp("alt+b", "sidebar"),
	),
	// alt+o avoids VS Code "Open Folder" interception of ctrl+o
	Settings: key.NewBinding(
		key.WithKeys("alt+o"),
		key.WithHelp("alt+o", "settings"),
	),
	EnvPicker: key.NewBinding(
		key.WithKeys("ctrl+e"),
		key.WithHelp("ctrl+e", "env"),
	),
	// alt+m avoids VS Code terminal tab interception of ctrl+t
	BodyMode: key.NewBinding(
		key.WithKeys("alt+m"),
		key.WithHelp("alt+m", "raw/form"),
	),
	// alt+j avoids VS Code "Toggle Panel" interception of ctrl+j
	PrettyPrint: key.NewBinding(
		key.WithKeys("alt+j"),
		key.WithHelp("alt+j", "format JSON"),
	),
	Search: key.NewBinding(
		key.WithKeys("/"),
		key.WithHelp("/", "search"),
	),
	Import: key.NewBinding(
		key.WithKeys("ctrl+i"),
		key.WithHelp("ctrl+i", "import"),
	),
	Export: key.NewBinding(
		key.WithKeys("ctrl+x"),
		key.WithHelp("ctrl+x", "export"),
	),
	// alt+q avoids VS Code interception; f10 kept as alias for other terminals
	Quit: key.NewBinding(
		key.WithKeys("alt+q", "f10"),
		key.WithHelp("alt+q", "quit"),
	),
	// alt+p avoids VS Code "Quick Open" interception of ctrl+p
	Palette: key.NewBinding(
		key.WithKeys("alt+p"),
		key.WithHelp("alt+p", "palette"),
	),
	// alt+c: copy current request as a curl command
	CurlExport: key.NewBinding(
		key.WithKeys("alt+c"),
		key.WithHelp("alt+c", "copy as curl"),
	),
	// alt+e: open body in $EDITOR
	ExternalEditor: key.NewBinding(
		key.WithKeys("alt+e"),
		key.WithHelp("alt+e", "open in editor"),
	),
}

func applyKeyOverrides(kb map[string]string) {
	set := func(b *key.Binding, name string) {
		if v := kb[name]; v != "" {
			b.SetKeys(v)
		}
	}
	set(&keys.Send, "send")
	set(&keys.Save, "save")
	set(&keys.NewRequest, "new")
	set(&keys.ToggleSidebar, "sidebar")
	set(&keys.Settings, "settings")
	set(&keys.EnvPicker, "env")
	set(&keys.BodyMode, "body_mode")
	set(&keys.PrettyPrint, "format")
	set(&keys.Search, "search")
	set(&keys.Import, "import")
	set(&keys.Export, "export")
	set(&keys.Quit, "quit")
	set(&keys.Palette, "palette")
	set(&keys.CurlExport, "curl_export")
	set(&keys.ExternalEditor, "external_editor")
}

package tui

import (
	"io"
	"net/http"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/d0mkaaa/gopull/internal/plugins"
	"github.com/d0mkaaa/gopull/internal/store"
)

type focusEditorMsg struct{}
type focusResponseMsg struct{}
type focusSidebarMsg struct{}

type loadRequestMsg struct {
	req    *store.Request
	collID string
}

type collectionsUpdatedMsg struct {
	cols   []*store.Collection
	status string
}

type responseMsg struct {
	r          *result
	rawBody    []byte
	err        error
	envUpdates map[string]string // from post_response plugins
	pluginLogs []string
}

type historyWrittenMsg struct{}
type clearStatusMsg struct{}

type errMsg struct{ err error }

type deleteRequestMsg struct {
	collID string
	reqID  string
}

type deleteCollectionMsg struct {
	collID string
}

type saveResponseMsg struct {
	body        string
	contentType string
}

type fileWrittenMsg struct {
	path string
	err  error
}

type importDoneMsg struct {
	col *store.Collection
	err error
}

type exportDoneMsg struct {
	path string
	err  error
}

type themeAppliedMsg struct {
	theme string
}

type streamReadyMsg struct {
	stream     io.ReadCloser
	start      time.Time
	statusCode int
	status     string
	headers    http.Header
}

type streamLineMsg struct {
	line string
	next tea.Cmd
}

type streamDoneMsg struct {
	elapsed time.Duration
	code    int
	status  string
	headers http.Header
	body    []byte
}

type openDiffMsg struct {
	currentBody string
}

type historyLoadedMsg struct {
	entries     []store.HistoryEntry
	currentBody string
}

type historyBrowserLoadedMsg struct {
	entries []store.HistoryEntry
	err     error
}

type historyActionMsg struct {
	action string
	entry  store.HistoryEntry
}

type environmentsUpdatedMsg struct {
	envs        []*store.Environment
	activeEnvID string
	status      string
	err         error
}

type paletteExecMsg struct {
	action string
	req    *store.Request
	collID string
}

type runCollectionMsg struct {
	collID string
}

type runnerResultMsg struct {
	idx    int
	result runnerResult
}

type runnerDoneMsg struct{}

type pluginsLoadedMsg struct {
	runner *plugins.Runner
}

type externalEditorDoneMsg struct {
	tmpFile string
	err     error
}

type curlCopiedMsg struct{}

type renameCollectionMsg struct {
	collID string
	name   string
}

type renameRequestMsg struct {
	collID string
	reqID  string
	name   string
}

type duplicateRequestMsg struct {
	collID string
	reqID  string
}

type moveRequestMsg struct {
	collID string
	reqID  string
	delta  int
}

type customThemeSavedMsg struct {
	themeID string
	theme   Theme
	err     error
}

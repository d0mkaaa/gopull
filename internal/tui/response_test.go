package tui

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

func makeResponse() ResponseModel {
	return newResponse(120, 40)
}

func makeResult(code int, body string) *result {
	return buildResult(
		[]byte(body),
		http.Header{"Content-Type": []string{"application/json"}},
		http.StatusText(code),
		code,
		100*time.Millisecond,
	)
}

func TestResponseSetResultClearsLoadingAndError(t *testing.T) {
	m := makeResponse()
	m.loading = true
	m.err = errors.New("previous error")

	m = m.SetResult(makeResult(200, `{"ok":true}`))

	if m.loading {
		t.Error("loading should be false after SetResult")
	}
	if m.err != nil {
		t.Errorf("err should be nil after SetResult, got %v", m.err)
	}
	if m.result == nil {
		t.Error("result should be set")
	}
}

func TestResponseSetResultClearsVisualMode(t *testing.T) {
	m := makeResponse()
	m.visualMode = true
	m.visualAnchor = 5

	m = m.SetResult(makeResult(200, `{}`))

	if m.visualMode {
		t.Error("visualMode should be cleared by SetResult")
	}
}

func TestResponseSetResultResetsToBodyTab(t *testing.T) {
	m := makeResponse()
	m.tab = rtHeaders

	m = m.SetResult(makeResult(200, `{}`))

	if m.tab != rtBody {
		t.Errorf("tab: got %v, want rtBody", m.tab)
	}
}

func TestResponseSetResultClearsSearch(t *testing.T) {
	m := makeResponse()
	m.query = "foo"
	m.matchPositions = []int{0, 5}
	m.matchIndex = 1

	m = m.SetResult(makeResult(200, `{}`))

	if m.query != "" {
		t.Errorf("query should be cleared, got %q", m.query)
	}
	if len(m.matchPositions) != 0 {
		t.Errorf("matchPositions should be cleared, len=%d", len(m.matchPositions))
	}
}

func TestResponseSetLoadingClearsResult(t *testing.T) {
	m := makeResponse()
	m = m.SetResult(makeResult(200, `{"key":"value"}`))
	m = m.SetLoading(true)

	if !m.loading {
		t.Error("loading should be true")
	}
	if m.result != nil {
		t.Error("result should be nil after SetLoading(true)")
	}
	if m.err != nil {
		t.Error("err should be nil after SetLoading(true)")
	}
	if m.visualMode {
		t.Error("visualMode should be cleared by SetLoading")
	}
}

func TestResponseSetLoadingFalseDoesNotClearResult(t *testing.T) {
	m := makeResponse()
	m = m.SetResult(makeResult(200, `{}`))
	m = m.SetLoading(false)

	if m.result == nil {
		t.Error("SetLoading(false) should not clear an existing result")
	}
}

func TestResponseSetError(t *testing.T) {
	m := makeResponse()
	m = m.SetResult(makeResult(200, `{}`))

	sentinel := errors.New("connection refused")
	m = m.SetError(sentinel)

	if m.err != sentinel {
		t.Errorf("err: got %v, want %v", m.err, sentinel)
	}
	if m.result != nil {
		t.Error("result should be nil after SetError")
	}
	if m.loading {
		t.Error("loading should be false after SetError")
	}
	if m.visualMode {
		t.Error("visualMode should be cleared by SetError")
	}
}

func TestRenderTabsContainsAllNames(t *testing.T) {
	names := []string{"body", "headers", "tests"}
	out := renderTabs(names, 0, true)
	for _, name := range names {
		if !strings.Contains(out, name) {
			t.Errorf("renderTabs output missing %q", name)
		}
	}
}

func TestRenderTabsActiveIndexInRange(t *testing.T) {
	names := []string{"body", "headers", "tests"}
	// Rendering should not panic for any valid active index.
	for i := range names {
		out := renderTabs(names, i, true)
		if !strings.Contains(out, names[i]) {
			t.Errorf("active tab %d name %q missing from output", i, names[i])
		}
	}
}

func TestRenderTabsSingleTab(t *testing.T) {
	out := renderTabs([]string{"body"}, 0, true)
	if !strings.Contains(out, "body") {
		t.Errorf("single tab output missing name: %q", out)
	}
	if strings.Contains(out, "  ") {
		t.Errorf("single tab should have no spacer, got: %q", out)
	}
}

func TestRenderTabsActiveMarkerAndSpacerPresent(t *testing.T) {
	out := renderTabs([]string{"a", "b", "c"}, 0, true)
	if !strings.Contains(out, " a ") {
		t.Errorf("active tab should be padded, got: %q", out)
	}
	if !strings.Contains(out, " ") {
		t.Errorf("tabs should be spaced, got: %q", out)
	}
}

func TestBuildResultParsesJSONBody(t *testing.T) {
	raw := `{"name":"Alice","age":30}`
	r := makeResult(200, raw)

	if r.code != 200 {
		t.Errorf("code: got %d, want 200", r.code)
	}
	if r.size != len(raw) {
		t.Errorf("size: got %d, want %d", r.size, len(raw))
	}
	if !strings.Contains(r.plainBody, "Alice") {
		t.Errorf("plainBody should contain 'Alice': %q", r.plainBody)
	}
}

func TestBuildResultSetsElapsed(t *testing.T) {
	r := makeResult(200, `{}`)
	if r.elapsed != 100*time.Millisecond {
		t.Errorf("elapsed: got %v, want 100ms", r.elapsed)
	}
}

func TestBuildResultTrimsMIMEParams(t *testing.T) {
	r := buildResult(
		[]byte(`{}`),
		http.Header{"Content-Type": []string{"application/json; charset=utf-8"}},
		"200 OK", 200,
		0,
	)
	if r.contentType != "application/json" {
		t.Errorf("contentType: got %q, want %q", r.contentType, "application/json")
	}
}

func TestBuildResultNon200Code(t *testing.T) {
	r := makeResult(404, `{"error":"not found"}`)
	if r.code != 404 {
		t.Errorf("code: got %d, want 404", r.code)
	}
	if !strings.Contains(r.plainBody, "not found") {
		t.Errorf("plainBody should contain error: %q", r.plainBody)
	}
}

package tui

import "testing"

func TestEditorCompletesEnvVarInBody(t *testing.T) {
	m := newEditor(80, 24).SetEnvVars(map[string]string{
		"BASE_URL": "https://api.example.com",
	})
	m.focused = true
	m.inner = eiContent
	m.tab = etBody
	m = m.focusContent()
	m.bodyInput.SetValue("{{BA")
	m.bodyInput.CursorEnd()

	got, ok := m.applyCompletion()
	if !ok {
		t.Fatal("expected body completion")
	}
	if got.bodyInput.Value() != "{{BASE_URL}}" {
		t.Fatalf("body value = %q, want env placeholder", got.bodyInput.Value())
	}
}

func TestEditorCompletesHeaderName(t *testing.T) {
	m := newEditor(80, 24)
	m.focused = true
	m.inner = eiContent
	m.tab = etHeaders
	m = m.focusContent()
	m.headersInput.SetValue("Cont")
	m.headersInput.CursorEnd()

	got, ok := m.applyCompletion()
	if !ok {
		t.Fatal("expected header completion")
	}
	if got.headersInput.Value() != "Content-Type: " {
		t.Fatalf("header value = %q, want Content-Type", got.headersInput.Value())
	}
}

func TestEditorCompletesContentTypeValue(t *testing.T) {
	m := newEditor(80, 24)
	m.focused = true
	m.inner = eiContent
	m.tab = etHeaders
	m = m.focusContent()
	m.headersInput.SetValue("Content-Type: app")
	m.headersInput.CursorEnd()

	got, ok := m.applyCompletion()
	if !ok {
		t.Fatal("expected content type completion")
	}
	if got.headersInput.Value() != "Content-Type: application/json" {
		t.Fatalf("header value = %q, want application/json", got.headersInput.Value())
	}
}

func TestEditorCompletesPathParamFromURL(t *testing.T) {
	m := newEditor(80, 24)
	m.focused = true
	m.inner = eiContent
	m.tab = etParams
	m.paramsField = 1
	m.url.SetValue("https://example.com/users/:id/posts/:post_id")
	m.pathInput.SetValue("id=42\npo")
	m.pathInput.CursorEnd()
	m = m.focusContent()

	got, ok := m.applyCompletion()
	if !ok {
		t.Fatal("expected path param completion")
	}
	if got.pathInput.Value() != "id=42\npost_id=" {
		t.Fatalf("path params = %q, want missing post_id", got.pathInput.Value())
	}
}

func TestEditorCompletesFirstMissingPathParamOnEmptyLine(t *testing.T) {
	m := newEditor(80, 24)
	m.focused = true
	m.inner = eiContent
	m.tab = etParams
	m.paramsField = 1
	m.url.SetValue("https://example.com/users/:id")
	m.pathInput.SetValue("")
	m.pathInput.CursorEnd()
	m = m.focusContent()

	got, ok := m.applyCompletion()
	if !ok {
		t.Fatal("expected path param completion")
	}
	if got.pathInput.Value() != "id=" {
		t.Fatalf("path params = %q, want id=", got.pathInput.Value())
	}
}

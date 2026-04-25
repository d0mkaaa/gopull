package client

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSubstituteReplaces(t *testing.T) {
	got := substitute("https://{{HOST}}/{{PATH}}", map[string]string{
		"HOST": "example.com",
		"PATH": "api/v1",
	})
	want := "https://example.com/api/v1"
	if got != want {
		t.Fatalf("substitute: got %q, want %q", got, want)
	}
}

func TestSubstituteNoEnvReturnsOriginal(t *testing.T) {
	s := "https://{{HOST}}/path"
	if got := substitute(s, nil); got != s {
		t.Fatalf("got %q, want %q", got, s)
	}
}

func TestSubstituteNoPlaceholderReturnsOriginal(t *testing.T) {
	s := "https://example.com"
	if got := substitute(s, map[string]string{"X": "y"}); got != s {
		t.Fatalf("got %q, want %q", got, s)
	}
}

func TestClientSendsGET(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method: got %s, want GET", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, `{"ok":true}`)
	}))
	defer srv.Close()

	resp, err := Send(context.Background(), Request{Method: "GET", URL: srv.URL})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("status: got %d, want 200", resp.StatusCode)
	}
	if !strings.Contains(string(resp.Body), "ok") {
		t.Fatalf("body missing expected content: %s", resp.Body)
	}
}

func TestClientSendsPOSTWithBody(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	_, err := Send(context.Background(), Request{
		Method: "POST",
		URL:    srv.URL,
		Body:   `{"name":"test"}`,
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if gotBody != `{"name":"test"}` {
		t.Fatalf("body: got %q", gotBody)
	}
}

func TestClientSendsCustomHeaders(t *testing.T) {
	var gotHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("X-Custom")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	_, err := Send(context.Background(), Request{
		URL:     srv.URL,
		Headers: map[string]string{"X-Custom": "hello"},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if gotHeader != "hello" {
		t.Fatalf("X-Custom header: got %q, want %q", gotHeader, "hello")
	}
}

func TestClientBearerAuth(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	_, err := Send(context.Background(), Request{
		URL:  srv.URL,
		Auth: Auth{Type: "bearer", Token: "mytoken"},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if gotAuth != "Bearer mytoken" {
		t.Fatalf("Authorization: got %q, want %q", gotAuth, "Bearer mytoken")
	}
}

func TestClientBasicAuth(t *testing.T) {
	var gotUser, gotPass string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser, gotPass, _ = r.BasicAuth()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	_, err := Send(context.Background(), Request{
		URL:  srv.URL,
		Auth: Auth{Type: "basic", User: "alice", Pass: "s3cr3t"},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if gotUser != "alice" || gotPass != "s3cr3t" {
		t.Fatalf("basic auth: got user=%q pass=%q", gotUser, gotPass)
	}
}

func TestClientBearerAuthUsesEnvSubstitution(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	_, err := Send(context.Background(), Request{
		URL:  srv.URL,
		Auth: Auth{Type: "bearer", Token: "{{TOKEN}}"},
		Env:  map[string]string{"TOKEN": "secret"},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if gotAuth != "Bearer secret" {
		t.Fatalf("Authorization: got %q", gotAuth)
	}
}

func TestClientSubstitutesEnvInURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// Replace the host portion via env var.
	_, err := Send(context.Background(), Request{
		URL: "{{BASE}}",
		Env: map[string]string{"BASE": srv.URL},
	})
	if err != nil {
		t.Fatalf("Send with env URL: %v", err)
	}
}

func TestClientSubstitutesEnvInBody(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	_, err := Send(context.Background(), Request{
		Method: "POST",
		URL:    srv.URL,
		Body:   `{"user":"{{USER}}"}`,
		Env:    map[string]string{"USER": "bob"},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if gotBody != `{"user":"bob"}` {
		t.Fatalf("body: got %q", gotBody)
	}
}

func TestClientDisableRedirects(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "/dest", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	resp, err := Send(context.Background(), Request{
		URL:     srv.URL + "/redirect",
		Options: Options{DisableRedirects: true},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("expected 302 (redirect not followed), got %d", resp.StatusCode)
	}
}

func TestClientFollowsRedirectsByDefault(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "/dest", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, "arrived")
	}))
	defer srv.Close()

	resp, err := Send(context.Background(), Request{URL: srv.URL + "/redirect"})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 after redirect, got %d", resp.StatusCode)
	}
}

func TestClientContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second) // hang
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := Send(ctx, Request{URL: srv.URL})
	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
}

func TestClientEmptyMethodDefaultsToGET(t *testing.T) {
	var gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	_, err := Send(context.Background(), Request{URL: srv.URL})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Fatalf("method: got %q, want GET", gotMethod)
	}
}

func TestClientElapsedIsSet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	resp, err := Send(context.Background(), Request{URL: srv.URL})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if resp.Elapsed < time.Millisecond {
		t.Fatalf("Elapsed should be at least 1ms, got %v", resp.Elapsed)
	}
}

// Verify the basic auth wire format is correct for known inputs.
func TestBasicAuthEncoding(t *testing.T) {
	var gotHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	_, err := Send(context.Background(), Request{
		URL:  srv.URL,
		Auth: Auth{Type: "basic", User: "user", Pass: "pass"},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("user:pass"))
	if gotHeader != want {
		t.Fatalf("Authorization: got %q, want %q", gotHeader, want)
	}
}

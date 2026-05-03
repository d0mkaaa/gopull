package plugins

import (
	"testing"

	"github.com/d0mkaaa/gopull/internal/store"
)

func TestV1PluginPayloadOmitsSecretsWithoutPermission(t *testing.T) {
	req := store.Request{
		Method: "GET",
		URL:    "https://example.com",
		Auth:   store.Auth{Type: "bearer", Token: "literal-token", Pass: "password"},
		Body:   store.Body{Mode: "raw", Raw: "secret body"},
	}
	ctx := HookContext{
		Env:        map[string]string{"BASE_URL": "https://example.com", "TOKEN": "secret"},
		SecretKeys: map[string]bool{"TOKEN": true},
	}
	payload := buildPreRequestPayload(req, Manifest{APIVersion: "v1", Permissions: []string{PermissionReadEnv}}, ctx)

	if _, ok := payload.Env["TOKEN"]; ok {
		t.Fatal("secret env var should be omitted")
	}
	if payload.Auth.Token != "" || payload.Auth.Pass != "" {
		t.Fatalf("auth secrets should be redacted: %+v", payload.Auth)
	}
	if payload.Body.Raw != "" {
		t.Fatalf("body should be omitted without read_body permission: %+v", payload.Body)
	}
	if payload.Env["BASE_URL"] == "" {
		t.Fatal("non-secret env var should be present")
	}
}

func TestV1PluginPayloadIncludesSecretsWithPermission(t *testing.T) {
	req := store.Request{
		Auth: store.Auth{Type: "bearer", Token: "literal-token", Pass: "password"},
		Body: store.Body{Mode: "raw", Raw: "request body"},
	}
	ctx := HookContext{
		Env:        map[string]string{"TOKEN": "secret"},
		SecretKeys: map[string]bool{"TOKEN": true},
	}
	payload := buildPreRequestPayload(req, Manifest{APIVersion: "v1", Permissions: []string{PermissionReadBody, PermissionReadSecrets}}, ctx)

	if payload.Env["TOKEN"] != "secret" {
		t.Fatalf("secret env var missing: %+v", payload.Env)
	}
	if payload.Auth.Token != "literal-token" || payload.Auth.Pass != "password" {
		t.Fatalf("auth secrets should be present: %+v", payload.Auth)
	}
	if payload.Body.Raw != "request body" {
		t.Fatalf("body should be present: %+v", payload.Body)
	}
}

func TestLegacyPluginPayloadKeepsSecretCompatibility(t *testing.T) {
	ctx := HookContext{
		Env:        map[string]string{"TOKEN": "secret"},
		SecretKeys: map[string]bool{"TOKEN": true},
	}
	payload := buildPreRequestPayload(store.Request{}, Manifest{}, ctx)
	if payload.Env["TOKEN"] != "secret" {
		t.Fatal("legacy manifest should keep existing secret access")
	}
}

func TestV1PluginPayloadOmitsEnvWithoutPermission(t *testing.T) {
	ctx := HookContext{Env: map[string]string{"BASE_URL": "https://example.com"}}
	payload := buildPreRequestPayload(store.Request{}, Manifest{APIVersion: "v1"}, ctx)
	if payload.Env != nil {
		t.Fatalf("env should be omitted without read_env permission: %+v", payload.Env)
	}
}

func TestV1WritePermissionsAreExplicit(t *testing.T) {
	empty := Manifest{APIVersion: "v1"}
	if canReadBody(empty) || canWriteBody(empty) || canWriteEnv(empty) || canWriteHeaders(empty) || canWriteRequest(empty) {
		t.Fatal("v1 plugins should not read body or mutate requests without permissions")
	}
	full := Manifest{
		APIVersion: "v1",
		Permissions: []string{
			PermissionWriteBody,
			PermissionWriteEnv,
			PermissionWriteHeaders,
			PermissionWriteRequest,
		},
	}
	if !canWriteBody(full) || !canWriteEnv(full) || !canWriteHeaders(full) || !canWriteRequest(full) {
		t.Fatal("declared write permissions should be honored")
	}
}

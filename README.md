# gopull

[![CI](https://github.com/d0mkaaa/gopull/actions/workflows/ci.yml/badge.svg)](https://github.com/d0mkaaa/gopull/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/d0mkaaa/gopull?color=a6e3a1)](https://github.com/d0mkaaa/gopull/releases/latest)
[![Go](https://img.shields.io/badge/go-1.22+-89b4fa)](https://go.dev)
[![License](https://img.shields.io/github/license/d0mkaaa/gopull?color=cba6f7)](LICENSE)

A terminal HTTP client. Send requests, manage collections, inspect responses - keyboard-driven, single binary, no Electron.

---

## Install

```sh
# macOS (Intel + Apple Silicon)
brew install d0mkaaa/tap/gopull
# if macOS blocks the binary: xattr -d com.apple.quarantine "$(which gopull)"

# Windows
scoop bucket add d0mkaaa https://github.com/d0mkaaa/scoop-gopull
scoop install gopull

# Go toolchain
go install github.com/d0mkaaa/gopull@latest
```

Or grab a binary from [releases](https://github.com/d0mkaaa/gopull/releases/latest) - Linux, macOS, Windows all covered.

Build from source:

```sh
git clone https://github.com/d0mkaaa/gopull
cd gopull
go build -o gopull .
```

---

## Features

- **Collections** - save and organise requests, run them in sequence with pass/fail reporting
- **Environments** - create, edit, select, and delete named variable sets with `{{VAR}}` substitution
- **History** - browse the last 500 requests, replay them, save them, or diff responses
- **Streaming** - SSE responses arrive live line by line
- **Themes** - dark, light, nord, gruvbox - or create your own in JSON
- **Curl import** - paste a curl command into the URL field and press tab
- **Import/export** - Postman collections, `.http` files (VS Code REST Client), OpenAPI v2/v3
- **Search** - `/` to search the response body, `n`/`N` to jump between matches
- **JSON tree** - `t` to collapse/expand JSON responses interactively
- **Assertions** - write tests that run after every response; extract values into env vars with `set TOKEN = $.data.token`
- **Proxy** - per-request proxy URL or `HTTP_PROXY` / `HTTPS_PROXY` env vars
- **Custom methods** - type any method (PROPFIND, REPORT, etc.) in the method field

---

## Usage

```sh
gopull
```

Three panels: sidebar (collections), editor (request builder), response. `tab` / `shift+tab` moves focus.

### Editor

`[` and `]` cycle tabs: body, headers, auth, tests, opts.

- Method: `space` / `up` / `down` cycle standard methods; press any letter to type a custom one
- Headers: `Key: Value`, one per line. Prefix with `#` to disable a header
- Body: raw by default, `alt+m` toggles form / GraphQL mode
- Auth: bearer token or basic auth under the auth tab
- `ctrl+r` sends

### Collections

`ctrl+s` saves the current request. Auto-names from method + URL path if no name is set.

From the sidebar: `up` / `down` navigate, `enter` open, `r` run all requests in sequence, `n` rename, `ctrl+d` duplicate a request, `ctrl+j` / `ctrl+k` move a request, `d` twice to delete.

### Environments

`ctrl+e` opens the environment manager. `enter` selects, `n` creates, `e` edits, and `d` deletes.

Variables use a compact line format:

```text
BASE_URL=https://api.example.com
secret TOKEN=value
# DISABLED=value
```

`secret` values are masked in the picker. If an environment has a dotenv path, file variables load first and inline variables override them.

### History

`alt+h` opens the history browser. Filter by typing, `enter` loads a request into the editor, `ctrl+r` replays it, `s` saves it to the active collection, and `D` diffs its response body against another history entry.

### Response

`j`/`k` scroll, `/` search, `t` JSON tree view, `y` copy body, `w` save to file, `D` diff against history.

In tree view: `space` collapse/expand, `c` collapse all, `e` expand all, `{`/`}` jump between siblings.

### Assertions

Write assertions in the Tests tab:

```text
assert status == 200
assert header Content-Type contains json
assert body contains "token"
assert jsonpath $.data.id > 0
assert response_time < 500
set TOKEN = $.data.access_token
```

`set` extracts a JSONPath value and stores it in the active environment for use in the next request.

### Import

`ctrl+i` accepts:
- Postman collection JSON
- `.http` / `.rest` files (VS Code REST Client format)
- OpenAPI v2 (Swagger) or v3 JSON files
- OpenAPI spec URLs (`https://...`)

### Command palette

`alt+p` - fuzzy search over all actions and saved requests.

---

## Keybindings

| Key | Action |
|-----|--------|
| `ctrl+r` | Send request |
| `ctrl+s` | Save request |
| `alt+n` | New request |
| `alt+p` | Command palette |
| `alt+b` | Toggle sidebar |
| `alt+o` | Settings |
| `ctrl+e` | Environment manager |
| `alt+h` | History browser |
| `ctrl+i` | Import collection |
| `ctrl+x` | Export collection (Postman JSON) |
| `alt+j` | Format JSON body |
| `alt+m` | Toggle body mode |
| `alt+c` | Copy request as curl |
| `alt+e` | Open body in external editor |
| `[` / `]` | Switch editor tabs |
| `tab` / `shift+tab` | Move focus between panels |
| `/` | Search response body |
| `t` | Toggle JSON tree view |
| `y` | Copy response to clipboard |
| `w` | Save response to file |
| `D` | Diff response against history |
| `alt+q` | Quit |

Custom bindings go in `~/.config/gopull/keybindings.json`.

---

## Config

```text
~/.config/gopull/
  collections/        one JSON file per collection
  environments/       named variable sets
  history.json        last 500 requests
  config.json         theme, timeout, max display bytes
  keybindings.json    key overrides
  themes/             custom theme files
  plugins/            experimental local hooks
```

`config.json` options:

```json
{
  "timeout_secs": 30,
  "theme": "dark",
  "max_display_bytes": 5242880
}
```

---

## Themes

Built-in: `dark` (catppuccin mocha), `light` (catppuccin latte), `nord`, `gruvbox`. Switch in settings (`alt+o`), previews live.

Drop a JSON file in `~/.config/gopull/themes/` to add your own - a starter template is written there on first run.

## Experimental Hooks

Executables in `~/.config/gopull/plugins/` can register local `pre_request` and `post_response` hooks. Hooks are local executables that speak JSON over stdin/stdout; there is no marketplace, remote install, account, or sync layer.

Plugins return a manifest from `--manifest`:

```json
{
  "name": "aws-sigv4",
  "version": "0.1.0",
  "api_version": "v1",
  "hooks": ["pre_request"],
  "permissions": ["read_env", "read_body", "read_secrets", "write_headers"]
}
```

`pre_request` hooks receive request JSON and may return changed fields. `post_response` hooks receive the request plus a response snapshot and may return env updates.

v1 permissions:

- `read_env` - receive non-secret environment variables
- `read_body` - receive the request body
- `read_secrets` - receive secret environment variables and auth secrets
- `write_request` - change method or URL
- `write_headers` - replace request headers
- `write_body` - replace the request body
- `write_env` - write environment variables from `post_response`

Manifests without `api_version` run in legacy mode for compatibility. New plugins should use `api_version: "v1"` and declare the narrowest permissions they need.

---

## License

MIT

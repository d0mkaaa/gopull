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
- **Environments** - named variable sets with `{{VAR}}` substitution in URLs, headers, and body
- **History** - last 500 requests, diffable against the current response
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

- Method: `space` / `↑↓` cycle standard methods; press any letter to type a custom one
- Headers: `Key: Value`, one per line. Prefix with `#` to disable a header
- Body: raw by default, `alt+m` toggles form / GraphQL mode
- Auth: bearer token or basic auth under the auth tab
- `ctrl+r` sends

### Collections

`ctrl+s` saves the current request. Auto-names from method + URL path if no name is set.

From the sidebar: `↑↓` navigate, `enter` open, `r` run all requests in sequence, `d` twice to delete.

### Environments

`ctrl+e` opens the picker. `{{VAR_NAME}}` in any field gets substituted at send time.

### Response

`j`/`k` scroll, `/` search, `t` JSON tree view, `y` copy body, `w` save to file, `D` diff against history.

In tree view: `space` collapse/expand, `c` collapse all, `e` expand all, `{`/`}` jump between siblings.

### Assertions

Write assertions in the Tests tab:

```
assert status == 200
assert body contains "token"
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
| `ctrl+e` | Environment picker |
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

```
~/.config/gopull/
  collections/        one JSON file per collection
  environments/       named variable sets
  history.json        last 500 requests
  config.json         theme, timeout, max display bytes
  keybindings.json    key overrides
  themes/             custom theme files
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

---

## License

MIT

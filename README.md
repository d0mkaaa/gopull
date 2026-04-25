# gopull

[![CI](https://github.com/d0mkaaa/gopull/actions/workflows/ci.yml/badge.svg)](https://github.com/d0mkaaa/gopull/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/d0mkaaa/gopull?color=a6e3a1)](https://github.com/d0mkaaa/gopull/releases/latest)
[![Go](https://img.shields.io/badge/go-1.22+-89b4fa)](https://go.dev)
[![License](https://img.shields.io/github/license/d0mkaaa/gopull?color=cba6f7)](LICENSE)

A terminal HTTP client. Send requests, manage collections, inspect responses - keyboard-driven, single binary, no Electron.

---

## Install

```sh
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

- **Collections** - save and organise requests, run them in sequence
- **Environments** - named variable sets with `{{VAR}}` substitution in URLs, headers, and body
- **History** - last 500 requests, diffable against current response
- **Streaming** - SSE responses arrive live line by line
- **Themes** - dark, light, nord, gruvbox - or create your own
- **Curl import** - paste a curl command, get a saved request
- **Postman import/export** - collections round-trip cleanly
- **Search** - `/` to search the response body, `n`/`N` to jump between matches
- **Assertions** - write tests that run after every response

---

## Usage

```sh
gopull
```

Three panels: sidebar (collections), editor (request builder), response. `tab` / `shift+tab` moves focus.

### Editor

`[` and `]` cycle tabs: URL, headers, body, auth, options, tests.

- Method cycles with `space` or `enter`
- Headers: `Key: Value`, one per line
- Body: raw by default, `alt+m` toggles form encoding
- Auth: bearer token or basic auth under their own tab
- `ctrl+r` sends

### Collections

`ctrl+s` saves the current request. From the sidebar: navigate, open, delete (`d` twice). `r` runs all requests in sequence.

### Environments

`ctrl+e` opens the picker. `{{VAR_NAME}}` anywhere in the URL, headers, or body gets substituted at send time.

### Response

`j`/`k` scroll, `/` search, `y` copy body, `w` save to file, `D` diff against history.

### Command palette

`alt+p` - fuzzy search over all actions and saved requests.

### Assertions

Write assertions in the Tests tab of the editor. Results appear in the Tests tab on the response side after each send.

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
| `ctrl+i` | Import Postman collection |
| `ctrl+x` | Export collection |
| `alt+j` | Format JSON body |
| `alt+m` | Toggle body mode |
| `[` / `]` | Switch editor tabs |
| `tab` / `shift+tab` | Move focus between panels |
| `/` | Search response |
| `y` | Copy response to clipboard |
| `w` | Save response to file |
| `D` | Diff response against history |
| `alt+q` | Quit |

Custom bindings go in `~/.config/gopull/keybindings.json`.

---

## Config

```
~/.config/gopull/
  collections/      one JSON file per collection
  environments/     named variable sets
  history.json      last 500 requests
  config.json       theme, timeout
  keybindings.json  key overrides
  themes/           custom theme files
```

---

## Themes

Built-in: `dark` (catppuccin mocha), `light` (catppuccin latte), `nord`, `gruvbox`. Switch in settings (`alt+o`), previews live.

Drop a JSON file in `~/.config/gopull/themes/` to add your own - a starter template gets written there on first run.

---

## License

MIT

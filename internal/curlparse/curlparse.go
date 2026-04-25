package curlparse

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/d0mkaaa/gopull/internal/store"
)

// LooksLikeCurl reports whether s appears to be a curl command.
// It handles common shell prompts ($ % # >) and curl.exe.
func LooksLikeCurl(s string) bool {
	s = strings.TrimSpace(s)
	for _, prompt := range []string{"$ ", "% ", "# ", "> "} {
		if strings.HasPrefix(s, prompt) {
			s = strings.TrimSpace(s[len(prompt):])
			break
		}
	}
	low := strings.ToLower(s)
	return strings.HasPrefix(low, "curl ") ||
		strings.HasPrefix(low, "curl.exe ") ||
		low == "curl" || low == "curl.exe"
}

// Parse converts a curl command string into a store.Request.
// Handles all common curl flags and multi-line formats from Chrome DevTools,
// Postman, Insomnia, and plain terminal output.
func Parse(s string) (store.Request, error) {
	s = strings.TrimSpace(s)

	// Strip common shell prompts that end up in copy-paste
	for _, prompt := range []string{"$ ", "% ", "# ", "> "} {
		if strings.HasPrefix(s, prompt) {
			s = strings.TrimSpace(s[len(prompt):])
			break
		}
	}

	tokens, err := tokenize(s)
	if err != nil {
		return store.Request{}, err
	}
	if len(tokens) == 0 {
		return store.Request{}, fmt.Errorf("empty command")
	}

	first := strings.ToLower(tokens[0])
	if first != "curl" && first != "curl.exe" {
		return store.Request{}, fmt.Errorf("not a curl command")
	}

	req := store.Request{Method: "GET", Body: store.Body{Mode: "raw"}}
	headers := make(map[string]string)

	for i := 1; i < len(tokens); {
		t := tokens[i]

		// Split --flag=value into two tokens and re-process
		if strings.HasPrefix(t, "--") {
			if eq := strings.Index(t, "="); eq != -1 {
				tokens = tokenInsert(tokens, i, t[:eq], t[eq+1:])
				continue
			}
		}

		// Expand combined short flags: -sSL → -s -S -L
		if len(t) > 2 && t[0] == '-' && t[1] != '-' {
			var expanded []string
			for _, c := range t[1:] {
				expanded = append(expanded, "-"+string(c))
			}
			tokens = tokenInsert(tokens, i, expanded...)
			continue
		}

		switch t {
		case "-X", "--request":
			if i+1 < len(tokens) {
				req.Method = strings.ToUpper(tokens[i+1])
				i += 2
			} else {
				i++
			}

		case "-H", "--header":
			if i+1 < len(tokens) {
				parseHeader(tokens[i+1], headers)
				i += 2
			} else {
				i++
			}

		case "-d", "--data", "--data-raw", "--data-ascii", "--data-binary", "--data-urlencode":
			if i+1 < len(tokens) {
				body := tokens[i+1]
				// Strip bash $'...' quoting prefix
				if strings.HasPrefix(body, "$") {
					body = body[1:]
				}
				req.Body.Raw = body
				if req.Method == "GET" {
					req.Method = "POST"
				}
				i += 2
			} else {
				i++
			}

		case "--json":
			if i+1 < len(tokens) {
				req.Body.Raw = tokens[i+1]
				headers["Content-Type"] = "application/json"
				headers["Accept"] = "application/json"
				if req.Method == "GET" {
					req.Method = "POST"
				}
				i += 2
			} else {
				i++
			}

		case "-u", "--user":
			if i+1 < len(tokens) {
				parts := strings.SplitN(tokens[i+1], ":", 2)
				if len(parts) == 2 {
					req.Auth = store.Auth{Type: "basic", User: parts[0], Pass: parts[1]}
				}
				i += 2
			} else {
				i++
			}

		case "--oauth2-bearer":
			if i+1 < len(tokens) {
				req.Auth = store.Auth{Type: "bearer", Token: tokens[i+1]}
				i += 2
			} else {
				i++
			}

		// flags that take a value we don't use - must consume the next token
		case "-o", "--output",
			"--connect-timeout",
			"-m", "--max-time",
			"--limit-rate",
			"-e", "--referer",
			"--cert", "--key", "--cacert", "--capath",
			"-A", "--user-agent",
			"-x", "--proxy", "--proxy-user",
			"-b", "--cookie",
			"-c", "--cookie-jar",
			"-F", "--form",
			"--form-string",
			"--resolve",
			"--retry", "--retry-delay", "--retry-max-time",
			"-T", "--upload-file",
			"--unix-socket",
			"--interface",
			"--dns-servers",
			"-Y", "--speed-limit",
			"-y", "--speed-time":
			if i+1 < len(tokens) {
				i += 2
			} else {
				i++
			}

		// boolean flags that take no value - just skip
		case "-L", "--location",
			"-s", "--silent",
			"-S", "--show-error",
			"-v", "--verbose",
			"-k", "--insecure",
			"-i", "--include",
			"-I", "--head",
			"-f", "--fail", "--fail-with-body",
			"-g", "--globoff",
			"--compressed",
			"--no-keepalive", "--no-buffer",
			"--http1.0", "--http1.1",
			"--http2", "--http2-prior-knowledge", "--http3",
			"--ipv4", "--ipv6",
			"--digest", "--ntlm", "--basic", "--anyauth",
			"--tr-encoding",
			"--path-as-is",
			"-N",
			"-0":
			i++

		default:
			if !strings.HasPrefix(t, "-") && req.URL == "" {
				req.URL = t
			}
			i++
		}
	}

	// HEAD override - -I anywhere in the command forces HEAD
	for _, t := range tokens {
		if t == "-I" || t == "--head" {
			req.Method = "HEAD"
			break
		}
	}

	for k, v := range headers {
		req.Headers = append(req.Headers, store.Header{Key: k, Value: v, Enabled: true})
	}

	if req.URL == "" {
		return store.Request{}, fmt.Errorf("no URL found in curl command")
	}
	return req, nil
}

func parseHeader(s string, out map[string]string) {
	idx := strings.Index(s, ":")
	if idx <= 0 {
		return
	}
	k := strings.TrimSpace(s[:idx])
	v := strings.TrimSpace(s[idx+1:])
	out[k] = v
}

// tokenInsert replaces tokens[i] with replacements in-place.
func tokenInsert(tokens []string, i int, replacements ...string) []string {
	tail := make([]string, len(tokens[i+1:]))
	copy(tail, tokens[i+1:])
	return append(tokens[:i], append(replacements, tail...)...)
}

// tokenize splits a shell command string into tokens, handling:
//   - single and double quoted strings
//   - backslash escapes
//   - line continuations: \ (bash), ^ (CMD), ` (PowerShell)
//   - proper UTF-8 rune iteration
func tokenize(s string) ([]string, error) {
	// Normalize all line-continuation styles before tokenizing
	for _, pair := range [][2]string{
		{"\\\r\n", " "}, // bash \ on Windows paste
		{"\\\n", " "},   // bash \
		{"^\r\n", " "},  // CMD ^
		{"^\n", " "},    // CMD ^
		{"`\r\n", " "},  // PowerShell `
		{"`\n", " "},    // PowerShell `
	} {
		s = strings.ReplaceAll(s, pair[0], pair[1])
	}
	// Treat bare newlines as spaces (handles partial multi-line paste)
	s = strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ").Replace(s)
	s = strings.TrimSpace(s)

	var tokens []string
	var cur strings.Builder
	inSingle := false
	inDouble := false

	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])

		switch {
		case r == '\'' && !inDouble:
			inSingle = !inSingle
			i += size
		case r == '"' && !inSingle:
			inDouble = !inDouble
			i += size
		case r == '\\' && !inSingle && i+size < len(s):
			// consume the backslash then write the next rune literally
			i += size
			nr, ns := utf8.DecodeRuneInString(s[i:])
			cur.WriteRune(nr)
			i += ns
		case unicode.IsSpace(r) && !inSingle && !inDouble:
			if cur.Len() > 0 {
				tokens = append(tokens, cur.String())
				cur.Reset()
			}
			i += size
		default:
			cur.WriteRune(r)
			i += size
		}
	}

	if inSingle {
		return nil, fmt.Errorf("unterminated single quote in curl command")
	}
	if inDouble {
		return nil, fmt.Errorf("unterminated double quote in curl command")
	}
	if cur.Len() > 0 {
		tokens = append(tokens, cur.String())
	}
	return tokens, nil
}

package curlparse

import (
	"strings"
	"testing"

	"github.com/d0mkaaa/gopull/internal/store"
)

// hdrMap converts a Header slice to a map for easy lookup in assertions.
func hdrMap(hdrs []store.Header) map[string]string {
	m := make(map[string]string, len(hdrs))
	for _, h := range hdrs {
		m[h.Key] = h.Value
	}
	return m
}

func TestParse(t *testing.T) {
	cases := []struct {
		label   string
		input   string
		method  string
		url     string
		bodyHas string
		hdrLen  int
	}{
		{
			label:   "chrome devtools multiline",
			input:   "curl 'https://api.example.com/v1/users' \\\n  -H 'Authorization: Bearer tok' \\\n  --data-raw '{\"name\":\"test\"}'",
			method:  "POST",
			url:     "https://api.example.com/v1/users",
			bodyHas: `"name"`,
			hdrLen:  1,
		},
		{
			label:  "powershell backtick continuation",
			input:  "curl https://api.example.com `\n  -H 'Content-Type: application/json'",
			method: "GET",
			url:    "https://api.example.com",
			hdrLen: 1,
		},
		{
			label:  "windows cmd caret continuation",
			input:  "curl https://api.example.com ^\n  -H \"Content-Type: application/json\"",
			method: "GET",
			url:    "https://api.example.com",
			hdrLen: 1,
		},
		{
			label:  "shell dollar prompt",
			input:  "$ curl https://httpbin.org/get",
			method: "GET",
			url:    "https://httpbin.org/get",
		},
		{
			label:  "curl.exe on windows",
			input:  "curl.exe https://httpbin.org/get",
			method: "GET",
			url:    "https://httpbin.org/get",
		},
		{
			label:  "combined boolean flags -sSL",
			input:  "curl -sSL https://httpbin.org/get",
			method: "GET",
			url:    "https://httpbin.org/get",
		},
		{
			label:  "long flag equals syntax",
			input:  "curl --max-time=30 --compressed https://httpbin.org/get",
			method: "GET",
			url:    "https://httpbin.org/get",
		},
		{
			label:   "postman export format",
			input:   "curl --location --request POST 'https://api.example.com/' \\\n--header 'Content-Type: application/json' \\\n--data-raw '{\"key\": \"value\"}'",
			method:  "POST",
			url:     "https://api.example.com/",
			bodyHas: `"key"`,
			hdrLen:  1,
		},
		{
			label:   "openai style with bearer token",
			input:   `curl https://api.openai.com/v1/chat -H "Authorization: Bearer sk-abc" -H "Content-Type: application/json" -d '{"model":"gpt-4"}'`,
			method:  "POST",
			url:     "https://api.openai.com/v1/chat",
			bodyHas: `"model"`,
			hdrLen:  2,
		},
		{
			label:  "basic auth",
			input:  "curl -u user:pass https://api.example.com/protected",
			method: "GET",
			url:    "https://api.example.com/protected",
		},
	}

	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			r, err := Parse(tc.input)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			if r.Method != tc.method {
				t.Errorf("method: got %q want %q", r.Method, tc.method)
			}
			if r.URL != tc.url {
				t.Errorf("url: got %q want %q", r.URL, tc.url)
			}
			if tc.bodyHas != "" && !strings.Contains(r.Body.Raw, tc.bodyHas) {
				t.Errorf("body %q missing %q", r.Body.Raw, tc.bodyHas)
			}
			if len(r.Headers) != tc.hdrLen {
				t.Errorf("headers: got %d want %d - %+v", len(r.Headers), tc.hdrLen, r.Headers)
			}
		})
	}
}

func TestParseSimpleURL(t *testing.T) {
	// User pastes "curl https://www.example.com/" into the URL bar and
	// presses ctrl+r without pressing Tab first. Parser must still work.
	r, err := Parse("curl https://www.example.com/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.URL != "https://www.example.com/" {
		t.Errorf("url: got %q", r.URL)
	}
	if r.Method != "GET" {
		t.Errorf("method: got %q", r.Method)
	}
	if r.Body.Raw != "" {
		t.Errorf("body should be empty, got %q", r.Body.Raw)
	}
	if len(r.Headers) != 0 {
		t.Errorf("no headers expected, got %+v", r.Headers)
	}
}

func TestParseSimpleHTTP(t *testing.T) {
	r, err := Parse("curl http://localhost:8080/api/health")
	if err != nil {
		t.Fatal(err)
	}
	if r.URL != "http://localhost:8080/api/health" {
		t.Errorf("url: got %q", r.URL)
	}
	if r.Method != "GET" {
		t.Errorf("method: got %q", r.Method)
	}
}

func TestParseMethod(t *testing.T) {
	cases := []struct{ input, want string }{
		{"curl -X DELETE https://example.com/res/1", "DELETE"},
		{"curl -X PUT https://example.com/res/1", "PUT"},
		{"curl -X PATCH https://example.com/res/1", "PATCH"},
		{"curl --request POST https://example.com", "POST"},
		{"curl -X post https://example.com", "POST"},
		{"curl -X OPTIONS https://example.com", "OPTIONS"},
	}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			r, err := Parse(tc.input)
			if err != nil {
				t.Fatal(err)
			}
			if r.Method != tc.want {
				t.Errorf("got %q want %q", r.Method, tc.want)
			}
		})
	}
}

func TestParseHEAD(t *testing.T) {
	cases := []struct {
		label string
		input string
	}{
		{"short -I", "curl -I https://example.com/"},
		{"long --head", "curl --head https://example.com/"},
		{"-I with other flags", "curl -sSL -I https://example.com/"},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			r, err := Parse(tc.input)
			if err != nil {
				t.Fatal(err)
			}
			if r.Method != "HEAD" {
				t.Errorf("method: got %q want HEAD", r.Method)
			}
		})
	}
}

func TestParseBodyPromotesMethod(t *testing.T) {
	cases := []struct {
		label string
		input string
	}{
		{"short -d", `curl -d '{"x":1}' https://example.com`},
		{"--data-raw", `curl --data-raw '{"x":1}' https://example.com`},
		{"--data-binary", `curl --data-binary '{"x":1}' https://example.com`},
		{"--data-ascii", `curl --data-ascii '{"x":1}' https://example.com`},
		{"--data-urlencode", `curl --data-urlencode 'x=1' https://example.com`},
		{"--json", `curl --json '{"x":1}' https://example.com`},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			r, err := Parse(tc.input)
			if err != nil {
				t.Fatal(err)
			}
			if r.Method != "POST" {
				t.Errorf("expected POST, got %q", r.Method)
			}
			if r.Body.Raw == "" {
				t.Error("body should not be empty")
			}
		})
	}
}

func TestParseJSONFlag(t *testing.T) {
	r, err := Parse(`curl --json '{"q":"test"}' https://api.example.com/search`)
	if err != nil {
		t.Fatal(err)
	}
	hdrs := hdrMap(r.Headers)
	if hdrs["Content-Type"] != "application/json" {
		t.Errorf("Content-Type: got %q", hdrs["Content-Type"])
	}
	if hdrs["Accept"] != "application/json" {
		t.Errorf("Accept: got %q", hdrs["Accept"])
	}
}

func TestParseMultipleHeaders(t *testing.T) {
	input := `curl https://api.example.com` +
		` -H "Content-Type: application/json"` +
		` -H "Accept: application/json"` +
		` -H "X-Request-ID: abc123"`
	r, err := Parse(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Headers) != 3 {
		t.Errorf("headers: got %d want 3 - %+v", len(r.Headers), r.Headers)
	}
	hdrs := hdrMap(r.Headers)
	if hdrs["X-Request-ID"] != "abc123" {
		t.Errorf("X-Request-ID: got %q", hdrs["X-Request-ID"])
	}
}

func TestParseHeaderWithColonInValue(t *testing.T) {
	// Header values that contain colons must be preserved in full.
	r, err := Parse(`curl -H "Authorization: Bearer ey:foo:bar" https://example.com`)
	if err != nil {
		t.Fatal(err)
	}
	hdrs := hdrMap(r.Headers)
	if !strings.Contains(hdrs["Authorization"], "ey:foo:bar") {
		t.Errorf("header value truncated: %q", hdrs["Authorization"])
	}
}

func TestParseURLWithQuery(t *testing.T) {
	r, err := Parse("curl 'https://api.example.com/search?q=hello+world&page=1'")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.URL, "q=hello") {
		t.Errorf("URL missing query params: %q", r.URL)
	}
	if !strings.Contains(r.URL, "page=1") {
		t.Errorf("URL missing page param: %q", r.URL)
	}
}

func TestParseBasicAuth(t *testing.T) {
	r, err := Parse("curl -u alice:secret https://api.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if r.Auth.Type != "basic" {
		t.Errorf("auth type: got %q want basic", r.Auth.Type)
	}
	if r.Auth.User != "alice" {
		t.Errorf("user: got %q", r.Auth.User)
	}
	if r.Auth.Pass != "secret" {
		t.Errorf("pass: got %q", r.Auth.Pass)
	}
}

func TestParseBasicAuthUserOnly(t *testing.T) {
	// curl -u user: (empty password, colon present)
	r, err := Parse("curl -u apikey: https://api.stripe.com/v1/charges")
	if err != nil {
		t.Fatal(err)
	}
	if r.Auth.User != "apikey" {
		t.Errorf("user: got %q", r.Auth.User)
	}
}

func TestParseShellPrompts(t *testing.T) {
	for _, p := range []string{"$ ", "% ", "# ", "> "} {
		t.Run(p, func(t *testing.T) {
			r, err := Parse(p + "curl https://example.com")
			if err != nil {
				t.Fatal(err)
			}
			if r.URL != "https://example.com" {
				t.Errorf("url: got %q", r.URL)
			}
		})
	}
}

func TestParseSkippedFlagsDoNotEatURL(t *testing.T) {
	boolFlags := []string{
		"-L", "--location",
		"-s", "--silent",
		"-S", "--show-error",
		"-v", "--verbose",
		"-k", "--insecure",
		"-i", "--include",
		"-f", "--fail",
		"-g", "--globoff",
		"--compressed",
		"--http2",
		"--http1.1",
		"--no-keepalive",
		"--no-buffer",
	}
	for _, flag := range boolFlags {
		t.Run(flag, func(t *testing.T) {
			r, err := Parse("curl " + flag + " https://httpbin.org/get")
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			if r.URL != "https://httpbin.org/get" {
				t.Errorf("url: got %q", r.URL)
			}
		})
	}
}

func TestParseValueFlagsDoNotEatURL(t *testing.T) {
	cases := []struct {
		flag string
		arg  string
	}{
		{"-o", "/tmp/out"},
		{"-A", "MyAgent/1.0"},
		{"-m", "30"},
		{"--max-time", "30"},
		{"-b", "session=abc"},
		{"--cookie", "session=abc"},
		{"-x", "http://proxy:8080"},
		{"--proxy", "http://proxy:8080"},
		{"--limit-rate", "1000k"},
		{"--connect-timeout", "5"},
		{"--retry", "3"},
	}
	for _, tc := range cases {
		t.Run(tc.flag, func(t *testing.T) {
			r, err := Parse("curl " + tc.flag + " " + tc.arg + " https://httpbin.org/get")
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			if r.URL != "https://httpbin.org/get" {
				t.Errorf("url: got %q (flag=%s arg=%s)", r.URL, tc.flag, tc.arg)
			}
		})
	}
}

func TestParseEqualsSyntax(t *testing.T) {
	r, err := Parse("curl --max-time=30 --retry=3 --limit-rate=500k https://example.com")
	if err != nil {
		t.Fatal(err)
	}
	if r.URL != "https://example.com" {
		t.Errorf("url: got %q", r.URL)
	}
}

func TestParseCombinedFlags(t *testing.T) {
	cases := []string{
		"curl -sSL https://example.com",
		"curl -sSkL https://example.com",
		"curl -vksSL https://example.com",
		"curl -fsSL https://example.com",
	}
	for _, input := range cases {
		t.Run(input, func(t *testing.T) {
			r, err := Parse(input)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			if r.URL != "https://example.com" {
				t.Errorf("url: got %q", r.URL)
			}
		})
	}
}

func TestParseLineContinuations(t *testing.T) {
	cases := []struct {
		label string
		input string
	}{
		{
			"bash backslash",
			"curl \\\n  -H 'X-Foo: bar' \\\n  https://example.com",
		},
		{
			"cmd caret",
			"curl ^\n  -H \"X-Foo: bar\" ^\n  https://example.com",
		},
		{
			"powershell backtick",
			"curl `\n  -H 'X-Foo: bar' `\n  https://example.com",
		},
		{
			"windows CRLF bash",
			"curl \\\r\n  -H 'X-Foo: bar' \\\r\n  https://example.com",
		},
		{
			"windows CRLF cmd",
			"curl ^\r\n  -H 'X-Foo: bar' ^\r\n  https://example.com",
		},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			r, err := Parse(tc.input)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			if r.URL != "https://example.com" {
				t.Errorf("url: got %q", r.URL)
			}
			if len(r.Headers) != 1 {
				t.Errorf("expected 1 header, got %d - %+v", len(r.Headers), r.Headers)
			}
		})
	}
}

func TestParseRealWorld(t *testing.T) {
	cases := []struct {
		label  string
		input  string
		method string
		url    string
	}{
		{
			"httpbin GET",
			"curl -s https://httpbin.org/get",
			"GET", "https://httpbin.org/get",
		},
		{
			"httpbin POST json",
			`curl -s -X POST https://httpbin.org/post -H "Content-Type: application/json" -d '{"hello":"world"}'`,
			"POST", "https://httpbin.org/post",
		},
		{
			"GitHub API",
			`curl -H "Accept: application/vnd.github+json" -H "Authorization: Bearer ghp_token" https://api.github.com/repos/owner/repo`,
			"GET", "https://api.github.com/repos/owner/repo",
		},
		{
			"Stripe API basic auth",
			`curl https://api.stripe.com/v1/charges -u sk_test_xxx:`,
			"GET", "https://api.stripe.com/v1/charges",
		},
		{
			"Slack webhook",
			`curl -X POST -H 'Content-type: application/json' --data '{"text":"Hello"}' https://hooks.slack.com/services/T00/B00/xxx`,
			"POST", "https://hooks.slack.com/services/T00/B00/xxx",
		},
		{
			"OpenAI chat",
			`curl https://api.openai.com/v1/chat/completions -H "Content-Type: application/json" -H "Authorization: Bearer sk-xxx" -d '{"model":"gpt-4","messages":[{"role":"user","content":"hi"}]}'`,
			"POST", "https://api.openai.com/v1/chat/completions",
		},
		{
			"install script",
			"curl -fsSL https://raw.githubusercontent.com/owner/repo/main/install.sh",
			"GET", "https://raw.githubusercontent.com/owner/repo/main/install.sh",
		},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			r, err := Parse(tc.input)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			if r.Method != tc.method {
				t.Errorf("method: got %q want %q", r.Method, tc.method)
			}
			if r.URL != tc.url {
				t.Errorf("url: got %q want %q", r.URL, tc.url)
			}
		})
	}
}

func TestParseErrors(t *testing.T) {
	cases := []struct {
		label string
		input string
	}{
		{"empty string", ""},
		{"not curl - wget", "wget https://example.com"},
		{"not curl - bare URL", "https://example.com"},
		{"no URL in command", "curl -H 'X-Foo: bar'"},
		{"unterminated single quote", "curl 'https://example.com"},
		{"unterminated double quote", `curl "https://example.com`},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			_, err := Parse(tc.input)
			if err == nil {
				t.Errorf("expected error for input %q", tc.input)
			}
		})
	}
}

func TestLooksLikeCurl(t *testing.T) {
	yes := []string{
		"curl https://example.com",
		"curl.exe https://example.com",
		"CURL https://example.com",
		"Curl https://example.com",
		"$ curl https://example.com",
		"% curl https://example.com",
		"# curl https://example.com",
		"> curl https://example.com",
		"  curl   https://example.com",
		"curl", // bare "curl" counts
	}
	no := []string{
		"https://example.com",
		"wget https://example.com",
		"GET https://example.com",
		"",
		"curlnotcurl https://example.com",
		"nocurl https://example.com",
	}
	for _, s := range yes {
		t.Run("yes:"+s, func(t *testing.T) {
			if !LooksLikeCurl(s) {
				t.Errorf("LooksLikeCurl(%q) = false, want true", s)
			}
		})
	}
	for _, s := range no {
		t.Run("no:"+s, func(t *testing.T) {
			if LooksLikeCurl(s) {
				t.Errorf("LooksLikeCurl(%q) = true, want false", s)
			}
		})
	}
}

func TestTokenize(t *testing.T) {
	cases := []struct {
		input string
		want  []string
	}{
		{"curl https://a.com", []string{"curl", "https://a.com"}},
		{`curl "a b"`, []string{"curl", "a b"}},
		{`curl 'a b'`, []string{"curl", "a b"}},
		{`curl "it's"`, []string{"curl", "it's"}},
		{`curl 'say "hi"'`, []string{"curl", `say "hi"`}},
		{`curl a\ b`, []string{"curl", "a b"}},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got, err := tokenize(tc.input)
			if err != nil {
				t.Fatalf("tokenize error: %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("len: got %d want %d - %v", len(got), len(tc.want), got)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("token[%d]: got %q want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

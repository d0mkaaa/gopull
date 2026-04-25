package tui

import (
	"bytes"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
)

var chromaStyle = "dracula"

func highlight(body []byte, contentType string) string {
	if len(body) == 0 {
		return ""
	}

	ct := strings.ToLower(contentType)
	ct = strings.Split(ct, ";")[0]
	ct = strings.TrimSpace(ct)

	lexer := lexerForContentType(ct)

	if lexer == nil {
		lexer = lexers.Analyse(string(body))
	}
	if lexer == nil {
		return string(body)
	}
	lexer = chroma.Coalesce(lexer)

	style := styles.Get(chromaStyle)
	if style == nil {
		style = styles.Fallback
	}

	formatter := formatters.Get("terminal256")
	if formatter == nil {
		return string(body)
	}

	iter, err := lexer.Tokenise(nil, string(body))
	if err != nil {
		return string(body)
	}

	var buf bytes.Buffer
	if err := formatter.Format(&buf, style, iter); err != nil {
		return string(body)
	}
	return buf.String()
}

func lexerForContentType(ct string) chroma.Lexer {
	switch {
	case strings.Contains(ct, "json") || strings.HasSuffix(ct, "+json"):
		return lexers.Get("json")
	case strings.Contains(ct, "html"):
		return lexers.Get("html")
	case strings.Contains(ct, "xml") || strings.HasSuffix(ct, "+xml"):
		return lexers.Get("xml")
	case strings.Contains(ct, "javascript") || ct == "application/js":
		return lexers.Get("javascript")
	case strings.Contains(ct, "css"):
		return lexers.Get("css")
	case strings.Contains(ct, "yaml"):
		return lexers.Get("yaml")
	case strings.Contains(ct, "toml"):
		return lexers.Get("toml")
	case strings.Contains(ct, "graphql"):
		return lexers.Get("graphql")
	case strings.Contains(ct, "markdown"):
		return lexers.Get("markdown")
	case ct == "text/plain", ct == "":
		return nil // let auto-detect decide, or fall back to plain
	}
	return nil
}

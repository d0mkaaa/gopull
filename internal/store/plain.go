package store

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const PlainCollectionFile = "gopull.collection"

func LoadCollectionPath(path string) (*Collection, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return LoadPlainCollection(path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Collection
	if err := json.Unmarshal(data, &c); err != nil {
		if looksLikePlainRequest(path) {
			r, plainErr := parsePlainRequest(path)
			if plainErr != nil {
				return nil, plainErr
			}
			if r.ID == "" {
				r.ID = safePlainName(r.Name)
				if r.ID == "" {
					r.ID = safePlainName(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
				}
			}
			return &Collection{
				ID:       strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)),
				Name:     strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)),
				Requests: map[string]*Request{r.ID: r},
				Order:    []string{r.ID},
			}, nil
		}
		return nil, err
	}
	if c.Requests == nil {
		c.Requests = map[string]*Request{}
	}
	return &c, nil
}

func LoadPlainCollection(dir string) (*Collection, error) {
	c := &Collection{
		ID:       filepath.Base(dir),
		Name:     filepath.Base(dir),
		Requests: map[string]*Request{},
	}
	if meta, err := os.ReadFile(filepath.Join(dir, PlainCollectionFile)); err == nil {
		for _, line := range strings.Split(string(meta), "\n") {
			k, v, ok := splitPlainLine(line)
			if ok && strings.EqualFold(k, "name") {
				c.Name = v
			}
		}
	}
	var files []string
	if err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		name := d.Name()
		if strings.HasSuffix(name, ".gopull") || strings.HasSuffix(name, ".gopull.txt") {
			files = append(files, path)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	sort.Strings(files)
	for _, path := range files {
		r, err := parsePlainRequest(path)
		if err != nil {
			return nil, err
		}
		rel, _ := filepath.Rel(dir, path)
		r.ID = filepath.ToSlash(strings.TrimSuffix(strings.TrimSuffix(rel, ".txt"), ".gopull"))
		if r.Name == "" {
			r.Name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		}
		c.Requests[r.ID] = r
		c.Order = append(c.Order, r.ID)
	}
	return c, nil
}

func ExportPlainCollection(c *Collection, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	meta := "name: " + c.Name + "\n"
	if err := writeAtomic(filepath.Join(dir, PlainCollectionFile), []byte(meta)); err != nil {
		return err
	}
	usedNames := map[string]int{}
	for _, id := range c.Order {
		r := c.Requests[id]
		if r == nil {
			continue
		}
		name := safePlainName(r.Name)
		if name == "" {
			name = safePlainName(id)
		}
		name = uniquePlainName(name, usedNames)
		path := filepath.Join(dir, name+".gopull")
		if err := writeAtomic(path, []byte(formatPlainRequest(r))); err != nil {
			return err
		}
	}
	return nil
}

func parsePlainRequest(path string) (*Request, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := &Request{Method: "GET", Body: Body{Mode: "raw"}}
	section := ""
	bodyMode := "raw"
	var body, tests strings.Builder
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 2*1024*1024)
	for sc.Scan() {
		line := sc.Text()
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			section = strings.Trim(strings.TrimSuffix(strings.TrimPrefix(trimmed, "["), "]"), " ")
			if strings.HasPrefix(section, "body") {
				bodyMode = strings.TrimSpace(strings.TrimPrefix(section, "body"))
				if bodyMode == "" {
					bodyMode = "raw"
				}
			}
			continue
		}
		switch {
		case strings.HasPrefix(section, "body"):
			body.WriteString(line)
			body.WriteByte('\n')
		case section == "tests":
			tests.WriteString(line)
			tests.WriteByte('\n')
		case section == "headers":
			h := parseHeaderLine(line)
			if h.Key != "" {
				r.Headers = append(r.Headers, h)
			}
		case section == "query":
			if p, ok := parseParamLine(line); ok {
				r.Query = append(r.Query, p)
			}
		case section == "path":
			if p, ok := parseParamLine(line); ok {
				r.Path = append(r.Path, p)
			}
		case section == "auth":
			applyAuthLine(&r.Auth, line)
		case section == "options":
			applyOptionsLine(&r.Options, line)
		default:
			applyRequestLine(r, line)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	r.Body.Mode = bodyMode
	r.Body.Raw = strings.TrimRight(body.String(), "\n")
	r.Tests = strings.TrimRight(tests.String(), "\n")
	if r.Body.Mode == "" {
		r.Body.Mode = "raw"
	}
	return r, nil
}

func formatPlainRequest(r *Request) string {
	var b strings.Builder
	fmt.Fprintf(&b, "name: %s\nmethod: %s\nurl: %s\n", r.Name, r.Method, r.URL)
	if len(r.Tags) > 0 {
		fmt.Fprintf(&b, "tags: %s\n", strings.Join(r.Tags, ", "))
	}
	writeParams := func(title string, params []Param) {
		if len(params) == 0 {
			return
		}
		fmt.Fprintf(&b, "\n[%s]\n", title)
		for _, p := range params {
			if !p.Enabled {
				b.WriteString("# ")
			}
			fmt.Fprintf(&b, "%s=%s\n", p.Key, p.Value)
		}
	}
	writeParams("query", r.Query)
	writeParams("path", r.Path)
	if len(r.Headers) > 0 {
		b.WriteString("\n[headers]\n")
		for _, h := range r.Headers {
			if !h.Enabled {
				b.WriteString("# ")
			}
			fmt.Fprintf(&b, "%s: %s\n", h.Key, h.Value)
		}
	}
	if r.Auth.Type != "" && r.Auth.Type != "none" {
		b.WriteString("\n[auth]\n")
		fmt.Fprintf(&b, "type: %s\n", r.Auth.Type)
		fmt.Fprintf(&b, "token: %s\nuser: %s\npass: %s\n", r.Auth.Token, r.Auth.User, r.Auth.Pass)
	}
	if hasOptions(r.Options) {
		b.WriteString("\n[options]\n")
		fmt.Fprintf(&b, "skip_tls_verify=%t\ndisable_redirects=%t\ncookie_jar=%t\n", r.Options.SkipTLSVerify, r.Options.DisableRedirects, r.Options.UseCookieJar)
		fmt.Fprintf(&b, "proxy_url=%s\ntimeout_secs=%d\nca_bundle_path=%s\nclient_cert_path=%s\nclient_key_path=%s\n", r.Options.ProxyURL, r.Options.TimeoutSecs, r.Options.CABundlePath, r.Options.ClientCertPath, r.Options.ClientKeyPath)
	}
	if r.Body.Raw != "" {
		mode := r.Body.Mode
		if mode == "" {
			mode = "raw"
		}
		fmt.Fprintf(&b, "\n[body %s]\n%s\n", mode, r.Body.Raw)
	}
	if r.Tests != "" {
		fmt.Fprintf(&b, "\n[tests]\n%s\n", r.Tests)
	}
	return b.String()
}

func applyRequestLine(r *Request, line string) {
	k, v, ok := splitPlainLine(line)
	if !ok {
		return
	}
	switch strings.ToLower(k) {
	case "name":
		r.Name = v
	case "method":
		r.Method = strings.ToUpper(v)
	case "url":
		r.URL = v
	case "tags":
		r.Tags = splitTags(v)
	}
}

func applyAuthLine(a *Auth, line string) {
	k, v, ok := splitPlainLine(line)
	if !ok {
		return
	}
	switch strings.ToLower(k) {
	case "type":
		a.Type = v
	case "token":
		a.Token = v
	case "user":
		a.User = v
	case "pass":
		a.Pass = v
	}
}

func applyOptionsLine(o *RequestOptions, line string) {
	k, v, ok := splitPlainLine(line)
	if !ok {
		return
	}
	switch strings.ToLower(k) {
	case "skip_tls_verify":
		o.SkipTLSVerify = parseBool(v)
	case "disable_redirects":
		o.DisableRedirects = parseBool(v)
	case "cookie_jar":
		o.UseCookieJar = parseBool(v)
	case "proxy_url":
		o.ProxyURL = v
	case "timeout_secs":
		fmt.Sscanf(v, "%d", &o.TimeoutSecs)
	case "ca_bundle_path":
		o.CABundlePath = v
	case "client_cert_path":
		o.ClientCertPath = v
	case "client_key_path":
		o.ClientKeyPath = v
	}
}

func looksLikePlainRequest(path string) bool {
	name := strings.ToLower(filepath.Base(path))
	return strings.HasSuffix(name, ".gopull") || strings.HasSuffix(name, ".gopull.txt")
}

func uniquePlainName(base string, used map[string]int) string {
	if base == "" {
		base = "request"
	}
	used[base]++
	if used[base] == 1 {
		return base
	}
	return fmt.Sprintf("%s-%d", base, used[base])
}

func parseBool(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true", "yes", "on", "1":
		return true
	default:
		return false
	}
}

func parseHeaderLine(line string) Header {
	enabled := true
	line = strings.TrimSpace(line)
	if strings.HasPrefix(line, "#") {
		enabled = false
		line = strings.TrimSpace(strings.TrimPrefix(line, "#"))
	}
	idx := strings.Index(line, ":")
	if idx < 0 {
		return Header{}
	}
	return Header{Key: strings.TrimSpace(line[:idx]), Value: strings.TrimSpace(line[idx+1:]), Enabled: enabled}
}

func parseParamLine(line string) (Param, bool) {
	enabled := true
	line = strings.TrimSpace(line)
	if line == "" {
		return Param{}, false
	}
	if strings.HasPrefix(line, "#") {
		enabled = false
		line = strings.TrimSpace(strings.TrimPrefix(line, "#"))
	}
	k, v, ok := splitPlainLine(line)
	return Param{Key: k, Value: v, Enabled: enabled}, ok
}

func splitPlainLine(line string) (string, string, bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}
	idx := strings.Index(line, "=")
	if idx < 0 {
		idx = strings.Index(line, ":")
	}
	if idx < 0 {
		return "", "", false
	}
	k := strings.TrimSpace(line[:idx])
	v := strings.TrimSpace(line[idx+1:])
	return k, v, k != ""
}

func safePlainName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	replacer := strings.NewReplacer("\\", "-", "/", "-", ":", "-", "*", "-", "?", "-", "\"", "", "<", "-", ">", "-", "|", "-")
	s = replacer.Replace(s)
	s = strings.Join(strings.Fields(s), "-")
	return strings.Trim(s, "-")
}

func hasOptions(o RequestOptions) bool {
	return o.SkipTLSVerify || o.DisableRedirects || o.ProxyURL != "" || o.TimeoutSecs > 0 || o.UseCookieJar || o.CABundlePath != "" || o.ClientCertPath != "" || o.ClientKeyPath != ""
}

func splitTags(s string) []string {
	var tags []string
	for _, p := range strings.Split(s, ",") {
		tag := strings.TrimSpace(p)
		if tag != "" {
			tags = append(tags, tag)
		}
	}
	return tags
}

func NewPlainCollection(name string) *Collection {
	now := time.Now()
	return &Collection{Version: 1, ID: safePlainName(name), Name: name, CreatedAt: now, UpdatedAt: now, Requests: map[string]*Request{}}
}

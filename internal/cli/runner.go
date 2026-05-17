package cli

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/d0mkaaa/gopull/internal/core"
	"github.com/d0mkaaa/gopull/internal/plugins"
	"github.com/d0mkaaa/gopull/internal/store"
	"github.com/d0mkaaa/gopull/internal/tests"
)

type runRow struct {
	Name      string `json:"name"`
	Method    string `json:"method"`
	URL       string `json:"url"`
	Iteration int    `json:"iteration"`
	Status    int    `json:"status"`
	Elapsed   int64  `json:"elapsed_ms"`
	Passed    int    `json:"passed"`
	Failed    int    `json:"failed"`
	Error     string `json:"error,omitempty"`
	Skipped   bool   `json:"skipped,omitempty"`
}

type runReport struct {
	Collection string   `json:"collection"`
	Env        string   `json:"env,omitempty"`
	Results    []runRow `json:"results"`
	Passed     int      `json:"passed"`
	Failed     int      `json:"failed"`
	Duration   int64    `json:"duration_ms"`
}

type repeatFlags []string

func (f *repeatFlags) String() string { return strings.Join(*f, ",") }
func (f *repeatFlags) Set(v string) error {
	*f = append(*f, v)
	return nil
}

func Run(args []string, version string) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	envName := fs.String("env", "", "environment name or id")
	envFile := fs.String("env-file", "", "dotenv or JSON environment file")
	report := fs.String("report", "text", "text, json, or junit")
	bail := fs.Bool("bail", false, "stop on first failure")
	timeoutSecs := fs.Int("timeout", 30, "default timeout seconds")
	noPlugins := fs.Bool("no-plugins", false, "disable local plugins")
	offline := fs.Bool("offline", false, "print built requests without sending")
	iterations := fs.Int("iteration-count", 1, "number of collection iterations")
	includeTags := fs.String("tags", "", "comma-separated tags to include")
	excludeTags := fs.String("exclude-tags", "", "comma-separated tags to exclude")
	var envVars repeatFlags
	fs.Var(&envVars, "env-var", "environment override KEY=value")
	if err := fs.Parse(normalizeRunArgs(args)); err != nil {
		return 2
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: gopull run <collection.json|collection-dir> [--env name] [--report text|json|junit] [--bail]")
		return 2
	}
	reportMode := normalizeReport(*report)
	st, err := store.New()
	if err != nil {
		fmt.Fprintln(os.Stderr, "store:", err)
		return 1
	}
	c, err := store.LoadCollectionPath(fs.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, "collection:", err)
		return 1
	}
	resolved, activeEnv, err := loadEnv(st, *envName, *envFile, envVars)
	if err != nil {
		fmt.Fprintln(os.Stderr, "env:", err)
		return 1
	}
	var pr *plugins.Runner
	if !*noPlugins {
		pr = plugins.Load(filepath.Join(st.Dir(), "plugins"))
	}
	jar, _ := cookiejar.New(nil)
	start := time.Now()
	out := runReport{Collection: c.Name, Results: []runRow{}}
	if activeEnv != nil {
		out.Env = activeEnv.Name
	}
	include := tagSet(*includeTags)
	exclude := tagSet(*excludeTags)
	if *iterations < 1 {
		*iterations = 1
	}
	for iter := 1; iter <= *iterations; iter++ {
		for _, id := range c.Order {
			req := c.Requests[id]
			if req == nil || !matchesTags(req.Tags, include, exclude) {
				continue
			}
			if *offline {
				if !printOffline(*req, resolved) {
					out.Failed++
					if *bail {
						break
					}
				}
				continue
			}
			row := runOne(*req, iter, resolved, pr, jar, time.Duration(*timeoutSecs)*time.Second)
			out.Results = append(out.Results, row)
			if row.Error != "" || row.Failed > 0 || row.Status >= 400 {
				out.Failed++
				if *bail {
					break
				}
			} else {
				out.Passed++
			}
		}
		if *bail && out.Failed > 0 {
			break
		}
	}
	out.Duration = time.Since(start).Milliseconds()
	if *offline {
		if out.Failed > 0 {
			return 1
		}
		return 0
	}
	switch reportMode {
	case "json":
		data, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(data))
	case "junit":
		fmt.Println(formatJUnit(out))
	default:
		printTextReport(out, version)
	}
	if out.Failed > 0 {
		return 1
	}
	return 0
}

func normalizeRunArgs(args []string) []string {
	var flags []string
	var positional []string
	takesValue := map[string]bool{
		"-env": true, "--env": true, "-env-file": true, "--env-file": true, "-report": true, "--report": true, "-timeout": true, "--timeout": true, "-iteration-count": true, "--iteration-count": true, "-tags": true, "--tags": true, "-exclude-tags": true, "--exclude-tags": true,
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			flags = append(flags, arg)
			name := arg
			if idx := strings.Index(arg, "="); idx >= 0 {
				name = arg[:idx]
			}
			if (takesValue[name] || strings.HasPrefix(name, "--env-var")) && !strings.Contains(arg, "=") && i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
			continue
		}
		positional = append(positional, arg)
	}
	return append(flags, positional...)
}

func runOne(req store.Request, iteration int, env store.ResolvedEnvironment, pr *plugins.Runner, jar *cookiejar.Jar, fallback time.Duration) runRow {
	timeout := fallback
	if req.Options.TimeoutSecs > 0 {
		timeout = time.Duration(req.Options.TimeoutSecs) * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	start := time.Now()
	run, err := core.RunRequest(ctx, req, core.Env{Values: env.Values, SecretKeys: env.SecretKeys}, core.RunOptions{Plugins: pr, Jar: jar})
	row := runRow{Name: req.Name, Method: req.Method, URL: req.URL, Iteration: iteration, Elapsed: time.Since(start).Milliseconds()}
	if err != nil {
		row.Error = err.Error()
		return row
	}
	resp := run.Response
	row.Status = resp.StatusCode
	row.Elapsed = resp.Elapsed.Milliseconds()
	if req.Tests != "" {
		res := tests.Run(req.Tests, resp.StatusCode, resp.Body, formatHeaders(resp.Headers), resp.Elapsed)
		for _, a := range res.Assertions {
			if a.Pass {
				row.Passed++
			} else {
				row.Failed++
			}
		}
	}
	return row
}

func printTextReport(r runReport, version string) {
	fmt.Printf("gopull %s run %s", version, r.Collection)
	if r.Env != "" {
		fmt.Printf(" env=%s", r.Env)
	}
	fmt.Println()
	for _, row := range r.Results {
		state := "PASS"
		if row.Error != "" || row.Failed > 0 || row.Status >= 400 {
			state = "FAIL"
		}
		detail := fmt.Sprintf("%s %s", row.Method, row.URL)
		if row.Error != "" {
			detail += " error=" + row.Error
		}
		fmt.Printf("%s %3d %5dms %s\n", state, row.Status, row.Elapsed, detail)
	}
	fmt.Printf("passed=%d failed=%d duration=%dms\n", r.Passed, r.Failed, r.Duration)
}

func normalizeReport(report string) string {
	switch strings.ToLower(strings.TrimSpace(report)) {
	case "json", "junit", "text", "":
		if strings.TrimSpace(report) == "" {
			return "text"
		}
		return strings.ToLower(strings.TrimSpace(report))
	default:
		fmt.Fprintf(os.Stderr, "unknown report %q, using text\n", report)
		return "text"
	}
}

func printOffline(req store.Request, env store.ResolvedEnvironment) bool {
	cr, err := core.BuildClientRequest(req, env.Values)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", req.Name, err)
		return false
	}
	fmt.Printf("%s %s\n", cr.Method, cr.URL)
	for k, v := range cr.Headers {
		fmt.Printf("%s: %s\n", k, v)
	}
	if cr.Body != "" {
		fmt.Println()
		fmt.Println(cr.Body)
	}
	return true
}

func formatJUnit(r runReport) string {
	type failure struct {
		Message string `xml:"message,attr"`
		Text    string `xml:",chardata"`
	}
	type testcase struct {
		Name      string   `xml:"name,attr"`
		ClassName string   `xml:"classname,attr"`
		Time      string   `xml:"time,attr"`
		Failure   *failure `xml:"failure,omitempty"`
	}
	type testsuite struct {
		XMLName  xml.Name   `xml:"testsuite"`
		Name     string     `xml:"name,attr"`
		Tests    int        `xml:"tests,attr"`
		Failures int        `xml:"failures,attr"`
		Time     string     `xml:"time,attr"`
		Cases    []testcase `xml:"testcase"`
	}
	suite := testsuite{Name: r.Collection, Tests: len(r.Results), Failures: r.Failed, Time: fmt.Sprintf("%.3f", float64(r.Duration)/1000)}
	for _, row := range r.Results {
		tc := testcase{
			Name:      fmt.Sprintf("%s iteration %d", row.Name, row.Iteration),
			ClassName: r.Collection,
			Time:      fmt.Sprintf("%.3f", float64(row.Elapsed)/1000),
		}
		if row.Error != "" || row.Failed > 0 || row.Status >= 400 {
			msg := row.Error
			if msg == "" {
				msg = fmt.Sprintf("status=%d failed_assertions=%d", row.Status, row.Failed)
			}
			tc.Failure = &failure{Message: msg, Text: msg}
		}
		suite.Cases = append(suite.Cases, tc)
	}
	data, _ := xml.MarshalIndent(suite, "", "  ")
	return xml.Header + string(data)
}

func loadEnvFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if strings.EqualFold(filepath.Ext(path), ".json") {
		var raw map[string]string
		if err := json.Unmarshal(data, &raw); err == nil {
			return raw, nil
		}
		var gopull store.Environment
		if err := json.Unmarshal(data, &gopull); err == nil && hasGopullEnvKeys(gopull) {
			return store.ResolveEnvironment(&gopull).Values, nil
		}
		var bruno struct {
			Variables []struct {
				Name    string `json:"name"`
				Value   string `json:"value"`
				Enabled *bool  `json:"enabled"`
			} `json:"variables"`
		}
		if err := json.Unmarshal(data, &bruno); err == nil {
			out := map[string]string{}
			for _, v := range bruno.Variables {
				enabled := v.Enabled == nil || *v.Enabled
				if enabled && v.Name != "" {
					out[v.Name] = v.Value
				}
			}
			return out, nil
		}
		return nil, fmt.Errorf("parse env JSON: unsupported environment shape")
	}
	return store.ParseDotenv(path), nil
}

func hasGopullEnvKeys(env store.Environment) bool {
	for _, v := range env.Variables {
		if v.Key != "" {
			return true
		}
	}
	return false
}

func tagSet(s string) map[string]bool {
	out := map[string]bool{}
	for _, p := range strings.Split(s, ",") {
		tag := strings.ToLower(strings.TrimSpace(p))
		if tag != "" {
			out[tag] = true
		}
	}
	return out
}

func matchesTags(tags []string, include, exclude map[string]bool) bool {
	own := map[string]bool{}
	for _, tag := range tags {
		own[strings.ToLower(strings.TrimSpace(tag))] = true
	}
	for tag := range exclude {
		if own[tag] {
			return false
		}
	}
	if len(include) == 0 {
		return true
	}
	for tag := range include {
		if own[tag] {
			return true
		}
	}
	return false
}

func loadEnv(st *store.Store, name, envFile string, overrides []string) (store.ResolvedEnvironment, *store.Environment, error) {
	resolved := store.ResolvedEnvironment{Values: map[string]string{}, SecretKeys: map[string]bool{}}
	var active *store.Environment
	envs, err := st.LoadEnvironments()
	if err != nil {
		return store.ResolvedEnvironment{}, nil, err
	}
	if name != "" {
		found := false
		for _, e := range envs {
			if e.ID == name || strings.EqualFold(e.Name, name) {
				resolved = store.ResolveEnvironment(e)
				active = e
				found = true
				break
			}
		}
		if !found {
			return store.ResolvedEnvironment{}, nil, fmt.Errorf("%q not found", name)
		}
	}
	if envFile != "" {
		vars, err := loadEnvFile(envFile)
		if err != nil {
			return store.ResolvedEnvironment{}, nil, err
		}
		for k, v := range vars {
			resolved.Values[k] = v
		}
	}
	for _, pair := range overrides {
		k, v, ok := strings.Cut(pair, "=")
		if !ok || strings.TrimSpace(k) == "" {
			return store.ResolvedEnvironment{}, nil, fmt.Errorf("invalid --env-var %q, expected KEY=value", pair)
		}
		resolved.Values[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return resolved, active, nil
}

func formatHeaders(h http.Header) string {
	var b strings.Builder
	for k, vals := range h {
		for _, v := range vals {
			b.WriteString(k)
			b.WriteString(": ")
			b.WriteString(v)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

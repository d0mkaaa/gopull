package tests

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type AssertResult struct {
	Label  string
	Pass   bool
	Actual string
}

type RunResult struct {
	Assertions []AssertResult
	EnvUpdates map[string]string
}

// Run evaluates a test script against an HTTP response.
//
// Script syntax (one directive per line):
//
//	assert status == 200
//	assert status != 404
//	assert header Content-Type == application/json
//	assert body contains "token"
//	assert body !contains "error"
//	assert jsonpath $.data.id > 0
//	assert response_time < 500
//	set TOKEN = $.data.access_token
//	set ID    = $.id
func Run(script string, statusCode int, body []byte, rawHeaders string, elapsed time.Duration) RunResult {
	result := RunResult{EnvUpdates: make(map[string]string)}
	for _, raw := range strings.Split(script, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		switch {
		case strings.HasPrefix(line, "assert "):
			ar := evalAssert(strings.TrimPrefix(line, "assert "), statusCode, body, rawHeaders, elapsed)
			result.Assertions = append(result.Assertions, ar)
		case strings.HasPrefix(line, "set "):
			k, v := evalSet(strings.TrimPrefix(line, "set "), body)
			if k != "" {
				result.EnvUpdates[k] = v
			}
		}
	}
	return result
}

func evalAssert(expr string, status int, body []byte, rawHeaders string, elapsed time.Duration) AssertResult {
	label := "assert " + expr

	// assert header <name> <op> <value>
	if strings.HasPrefix(expr, "header ") {
		return evalAssertHeader(label, strings.TrimPrefix(expr, "header "), rawHeaders)
	}
	// assert jsonpath <path> <op> <value>
	if strings.HasPrefix(expr, "jsonpath ") {
		return evalAssertJSONPath(label, strings.TrimPrefix(expr, "jsonpath "), body)
	}
	// assert response_time <op> <ms>
	if strings.HasPrefix(expr, "response_time ") {
		return evalAssertResponseTime(label, strings.TrimPrefix(expr, "response_time "), elapsed)
	}

	parts := strings.SplitN(expr, " ", 3)
	if len(parts) < 3 {
		return AssertResult{Label: label, Pass: false, Actual: "parse error"}
	}
	subject, op, raw := parts[0], parts[1], parts[2]
	expected := strings.Trim(raw, `"'`)

	switch subject {
	case "status":
		actual := strconv.Itoa(status)
		return AssertResult{Label: label, Pass: cmp(actual, op, expected), Actual: actual}
	case "body":
		actual := string(body)
		pass := cmp(actual, op, expected)
		display := actual
		if len(display) > 60 {
			display = display[:60] + "..."
		}
		return AssertResult{Label: label, Pass: pass, Actual: display}
	}
	return AssertResult{Label: label, Pass: false, Actual: "unknown subject"}
}

func evalAssertHeader(label, expr, rawHeaders string) AssertResult {
	// expr: "<Header-Name> <op> <value>"
	parts := strings.SplitN(expr, " ", 3)
	if len(parts) < 3 {
		return AssertResult{Label: label, Pass: false, Actual: "parse error"}
	}
	headerName := parts[0]
	op := parts[1]
	expected := strings.Trim(parts[2], `"'`)

	// Scan rawHeaders for the header value.
	actual := ""
	for _, line := range strings.Split(rawHeaders, "\n") {
		if idx := strings.Index(line, ":"); idx > 0 {
			k := strings.TrimSpace(line[:idx])
			v := strings.TrimSpace(line[idx+1:])
			if strings.EqualFold(k, headerName) {
				actual = v
				break
			}
		}
	}
	return AssertResult{Label: label, Pass: cmp(actual, op, expected), Actual: actual}
}

func evalAssertJSONPath(label, expr string, body []byte) AssertResult {
	// expr: "<jsonpath> <op> <value>"
	parts := strings.SplitN(expr, " ", 3)
	if len(parts) < 3 {
		return AssertResult{Label: label, Pass: false, Actual: "parse error"}
	}
	path := parts[0]
	op := parts[1]
	expected := strings.Trim(parts[2], `"'`)

	var data interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return AssertResult{Label: label, Pass: false, Actual: "invalid JSON"}
	}
	val, ok := extractPath(data, path)
	if !ok {
		return AssertResult{Label: label, Pass: false, Actual: "path not found"}
	}
	actual := fmt.Sprintf("%v", val)
	return AssertResult{Label: label, Pass: cmp(actual, op, expected), Actual: actual}
}

func evalAssertResponseTime(label, expr string, elapsed time.Duration) AssertResult {
	// expr: "<op> <ms>"
	parts := strings.SplitN(expr, " ", 2)
	if len(parts) < 2 {
		return AssertResult{Label: label, Pass: false, Actual: "parse error"}
	}
	op := parts[0]
	expected := strings.TrimSpace(parts[1])
	actualMs := strconv.FormatInt(elapsed.Milliseconds(), 10)
	return AssertResult{Label: label, Pass: cmp(actualMs, op, expected), Actual: actualMs + "ms"}
}

func cmp(actual, op, expected string) bool {
	switch op {
	case "==":
		return actual == expected
	case "!=":
		return actual != expected
	case "contains":
		return strings.Contains(actual, expected)
	case "!contains":
		return !strings.Contains(actual, expected)
	case ">":
		a, err1 := strconv.ParseFloat(actual, 64)
		e, err2 := strconv.ParseFloat(expected, 64)
		return err1 == nil && err2 == nil && a > e
	case "<":
		a, err1 := strconv.ParseFloat(actual, 64)
		e, err2 := strconv.ParseFloat(expected, 64)
		return err1 == nil && err2 == nil && a < e
	case ">=":
		a, err1 := strconv.ParseFloat(actual, 64)
		e, err2 := strconv.ParseFloat(expected, 64)
		return err1 == nil && err2 == nil && a >= e
	case "<=":
		a, err1 := strconv.ParseFloat(actual, 64)
		e, err2 := strconv.ParseFloat(expected, 64)
		return err1 == nil && err2 == nil && a <= e
	case "matches":
		return strings.Contains(actual, expected)
	}
	return false
}

func evalSet(expr string, body []byte) (key, value string) {
	idx := strings.Index(expr, "=")
	if idx < 0 {
		return "", ""
	}
	key = strings.TrimSpace(expr[:idx])
	path := strings.TrimSpace(expr[idx+1:])

	if !strings.HasPrefix(path, "$") {
		return key, path
	}

	var data interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return key, ""
	}
	val, ok := extractPath(data, path)
	if !ok {
		return key, ""
	}
	return key, fmt.Sprintf("%v", val)
}

func extractPath(data interface{}, path string) (interface{}, bool) {
	path = strings.TrimPrefix(path, "$")
	path = strings.TrimPrefix(path, ".")
	if path == "" {
		return data, true
	}
	for _, part := range splitPath(path) {
		switch node := data.(type) {
		case map[string]interface{}:
			v, ok := node[part]
			if !ok {
				return nil, false
			}
			data = v
		case []interface{}:
			i, err := strconv.Atoi(part)
			if err != nil || i < 0 || i >= len(node) {
				return nil, false
			}
			data = node[i]
		default:
			return nil, false
		}
	}
	return data, true
}

func splitPath(path string) []string {
	var parts []string
	var cur strings.Builder
	for _, c := range path {
		switch c {
		case '.', '[', ']':
			if cur.Len() > 0 {
				parts = append(parts, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(c)
		}
	}
	if cur.Len() > 0 {
		parts = append(parts, cur.String())
	}
	return parts
}

package store

import (
	"os"
	"strings"
)

type ResolvedEnvironment struct {
	Values     map[string]string
	SecretKeys map[string]bool
}

func ResolveEnvironment(e *Environment) ResolvedEnvironment {
	values := map[string]string{}
	secrets := map[string]bool{}
	if e == nil {
		return ResolvedEnvironment{Values: values, SecretKeys: secrets}
	}
	if e.DotenvPath != "" {
		for k, v := range ParseDotenv(e.DotenvPath) {
			values[k] = v
		}
	}
	for _, v := range e.Variables {
		if v.Enabled && v.Key != "" {
			values[v.Key] = v.Value
			if v.Secret {
				secrets[v.Key] = true
			}
		}
	}
	return ResolvedEnvironment{Values: values, SecretKeys: secrets}
}

func ParseDotenv(path string) map[string]string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	out := map[string]string{}
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		idx := strings.Index(line, "=")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		val = strings.Trim(val, `"'`)
		if key != "" {
			out[key] = val
		}
	}
	return out
}

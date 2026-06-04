package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/d0mkaaa/gopull/internal/lint"
	"github.com/d0mkaaa/gopull/internal/store"
)

func Init(args []string) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	name := fs.String("name", "", "collection name")
	withEnv := fs.Bool("env", false, "write .env.example")
	force := fs.Bool("force", false, "overwrite existing scaffold files")
	if err := fs.Parse(normalizeWorkspaceArgs(args, map[string]bool{"-name": true, "--name": true})); err != nil {
		return 2
	}
	dir := "."
	if fs.NArg() > 0 {
		dir = fs.Arg(0)
	}
	if err := store.InitPlainWorkspace(dir, *name, *withEnv, *force); err != nil {
		fmt.Fprintln(os.Stderr, "init:", err)
		return 1
	}
	abs, _ := filepath.Abs(dir)
	fmt.Println("created gopull workspace:", abs)
	return 0
}

func Lint(args []string) int {
	fs := flag.NewFlagSet("lint", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	format := fs.String("format", "text", "text or json")
	if err := fs.Parse(normalizeWorkspaceArgs(args, map[string]bool{"-format": true, "--format": true})); err != nil {
		return 2
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: gopull lint <collection.json|collection-dir> [--format text|json]")
		return 2
	}
	findings, err := lint.CheckPath(fs.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, "lint:", err)
		return 2
	}
	switch *format {
	case "json":
		fmt.Print(lint.FormatJSON(findings))
	case "text", "":
		fmt.Print(lint.FormatText(findings))
	default:
		fmt.Fprintf(os.Stderr, "lint: unknown format %q\n", *format)
		return 2
	}
	if lint.HasErrors(findings) {
		return 1
	}
	return 0
}

func normalizeWorkspaceArgs(args []string, takesValue map[string]bool) []string {
	var flags []string
	var positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if len(arg) > 0 && arg[0] == '-' {
			flags = append(flags, arg)
			name := arg
			if idx := strings.Index(arg, "="); idx >= 0 {
				name = arg[:idx]
			}
			if takesValue[name] && !strings.Contains(arg, "=") && i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
			continue
		}
		positional = append(positional, arg)
	}
	return append(flags, positional...)
}

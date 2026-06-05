package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/d0mkaaa/gopull/internal/cli"
	"github.com/d0mkaaa/gopull/internal/store"
	"github.com/d0mkaaa/gopull/internal/tui"
)

const version = "0.6.0"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "run":
			os.Exit(cli.Run(os.Args[2:], version))
		case "init":
			os.Exit(cli.Init(os.Args[2:]))
		case "lint":
			os.Exit(cli.Lint(os.Args[2:]))
		case "version":
			fmt.Println("gopull " + version)
			os.Exit(0)
		}
	}

	ver := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *ver {
		fmt.Println("gopull " + version)
		os.Exit(0)
	}

	var st *store.Store
	var err error
	if flag.NArg() > 0 {
		st, err = store.NewWorkspace(flag.Arg(0))
	} else {
		st, err = store.New()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "store:", err)
		os.Exit(1)
	}

	p := tea.NewProgram(tui.New(st, version), tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, runErr := p.Run()

	// OSC 111 resets the terminal background to its configured default,
	// undoing the OSC 11 we set during the session.
	fmt.Print("\033]111\007")

	if runErr != nil {
		fmt.Fprintln(os.Stderr, runErr)
		os.Exit(1)
	}
}

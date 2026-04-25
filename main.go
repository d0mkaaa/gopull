package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/d0mkaaa/gopull/internal/store"
	"github.com/d0mkaaa/gopull/internal/tui"
)

const version = "0.1.0"

func main() {
	ver := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *ver {
		fmt.Println("gopull " + version)
		os.Exit(0)
	}

	st, err := store.New()
	if err != nil {
		fmt.Fprintln(os.Stderr, "store:", err)
		os.Exit(1)
	}

	p := tea.NewProgram(tui.New(st), tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, runErr := p.Run()

	// OSC 111 resets the terminal background to its configured default,
	// undoing the OSC 11 we set during the session.
	fmt.Print("\033]111\007")

	if runErr != nil {
		fmt.Fprintln(os.Stderr, runErr)
		os.Exit(1)
	}
}

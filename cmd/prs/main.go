package main

import (
	"fmt"
	"os"

	"github.com/barspielberg/prs-mng/internal/config"
	"github.com/barspielberg/prs-mng/internal/github"
	"github.com/barspielberg/prs-mng/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "prs:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	client, err := github.New()
	if err != nil {
		return err
	}
	_, err = tea.NewProgram(ui.New(cfg, client), tea.WithAltScreen()).Run()
	return err
}

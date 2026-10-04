package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/barspielberg/pr-pile/internal/config"
	"github.com/barspielberg/pr-pile/internal/github"
	"github.com/barspielberg/pr-pile/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "pile:", err)
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
	// An org IP allow list makes every search return zero rather than failing,
	// so an unreachable repo would render as a board with no PRs.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := client.CheckRepo(ctx, cfg.Repo); err != nil {
		return fmt.Errorf("cannot read %s: %w", cfg.Repo, err)
	}

	// Package-level styles capture the global colour profile at init, which is
	// detected from stdout before the TTY is set up -- backgrounds silently
	// vanish. Re-detect from the terminal itself first.
	lipgloss.SetColorProfile(termenv.NewOutput(os.Stdout).Profile)

	_, err = tea.NewProgram(ui.New(cfg, client), tea.WithAltScreen(), tea.WithOutput(ui.Terminal)).Run()
	return err
}

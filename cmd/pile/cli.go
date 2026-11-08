package main

import (
	"flag"
	"fmt"
	"io"
	"runtime/debug"
	"strings"
)

// Set by GoReleaser and the Homebrew formula. Source installs fall back to
// Go's embedded module and VCS metadata.
var (
	version = "dev"
	commit  = ""
	date    = ""
)

const usage = `pile — a terminal PR board for GitHub

Usage: pile [--help | --version]

Run pile inside a GitHub checkout. On first run, pile detects the repository
and writes ~/.config/pile/config.yml. Requires gh installed and logged in.

Options:
  -h, --help  Show this help and exit
  --version   Show version and build information and exit

Environment:
  PILE_CONFIG      Full path to the config file
  XDG_CONFIG_HOME  Config directory (default: ~/.config)

Documentation: https://barspielberg.github.io/pr-pile/
`

func runCLI(args []string, out io.Writer) error {
	flags := flag.NewFlagSet("pile", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var help, showVersion bool
	flags.BoolVar(&help, "help", false, "show help")
	flags.BoolVar(&help, "h", false, "show help")
	flags.BoolVar(&showVersion, "version", false, "show version")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected argument %q; use pile --help for usage", flags.Arg(0))
	}
	if help {
		_, err := io.WriteString(out, usage)
		return err
	}
	if showVersion {
		info, _ := debug.ReadBuildInfo()
		_, err := fmt.Fprintln(out, versionString(info))
		return err
	}
	return run()
}

func versionString(info *debug.BuildInfo) string {
	v, revision, built := version, commit, date
	if info != nil {
		if v == "dev" && info.Main.Version != "" && info.Main.Version != "(devel)" {
			v = info.Main.Version
		}
		var dirty bool
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				if revision == "" {
					revision = setting.Value
				}
			case "vcs.modified":
				dirty = setting.Value == "true"
			}
		}
		if len(revision) > 12 {
			revision = revision[:12]
		}
		if dirty && commit == "" && revision != "" {
			revision += "-dirty"
		}
	}
	result := "pile " + strings.TrimPrefix(v, "v")
	var details []string
	if revision != "" {
		details = append(details, "commit "+revision)
	}
	if built != "" {
		details = append(details, "built "+built)
	}
	if len(details) > 0 {
		result += " (" + strings.Join(details, ", ") + ")"
	}
	return result
}

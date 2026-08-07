// Command my-pi-package is an opinionated installer for the Pi coding agent.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/ipfans/my-pi-package/catalog"
	"github.com/ipfans/my-pi-package/install"
	"github.com/ipfans/my-pi-package/pi"
	"github.com/ipfans/my-pi-package/tui"
)

// Set by GoReleaser / task build -ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	// skills is nested and does not need the catalog
	if len(args) > 0 && args[0] == "skills" {
		return runSkills(args[1:])
	}

	flags, err := parseArgs(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		printHelp()
		return 2
	}
	if flags.help {
		printHelp()
		return 0
	}
	if flags.version {
		fmt.Printf("my-pi-package %s (%s) %s\n", version, commit, date)
		return 0
	}

	cat, err := loadCatalog(flags.catalogPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "catalog: %v\n", err)
		return 2
	}

	switch flags.command {
	case "install":
		return cmdInstall(cat, flags)
	case "status":
		return install.RunStatus(cat, flags.local)
	case "update":
		return install.RunUpdate(cat, install.Flags{
			Local:  flags.local,
			Yes:    flags.yes,
			Only:   flags.only,
			Except: flags.except,
		})
	case "remove":
		targets := flags.targets
		if len(targets) == 0 && tui.IsInteractive() {
			// interactive remove: list present packages via status-like picker later;
			// for now require explicit targets in non-picker path
			fmt.Fprintln(os.Stderr, "Usage: my-pi-package remove <id|source> [...]")
			fmt.Fprintln(os.Stderr, "Tip: run `my-pi-package status` to list package ids.")
			return 2
		}
		return install.RunRemove(cat, install.Flags{Local: flags.local}, targets)
	case "doctor":
		return install.RunDoctor(cat, flags.local)
	default:
		printHelp()
		return 2
	}
}

func loadCatalog(path string) (*catalog.Catalog, error) {
	if path != "" {
		return catalog.LoadFile(path)
	}
	return catalog.LoadEmbedded()
}

func cmdInstall(cat *catalog.Catalog, flags cliFlags) int {
	selected, err := cat.ResolveSelection(flags.only, flags.except)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	selected = cat.ExpandDependsOn(selected)

	usedSelectionFlag := len(flags.only) > 0 || len(flags.except) > 0
	interactive := !flags.yes && !usedSelectionFlag && tui.IsInteractive()

	if interactive {
		// ensurePi with TUI confirm if missing
		if !pi.HasCommand("pi") {
			fmt.Println("Could not find the `pi` command on PATH.")
			if !confirmYesNo("Install Pi now with npm install -g?", true) {
				fmt.Fprintln(os.Stderr, "Install Pi first, then re-run my-pi-package.")
				return 127
			}
			if !install.EnsurePi(cat, true, nil) {
				return 127
			}
		}
		res, err := tui.RunInstallPicker(cat, selected)
		if err != nil {
			fmt.Fprintf(os.Stderr, "tui: %v\n", err)
			return 1
		}
		if res.Aborted {
			fmt.Println("Aborted.")
			return 0
		}
		selected = cat.ExpandDependsOn(res.Selected)
	} else {
		if !install.EnsurePi(cat, flags.yes, nil) {
			return 127
		}
	}

	return install.RunInstall(cat, install.Flags{
		Local:  flags.local,
		Yes:    flags.yes,
		Only:   flags.only,
		Except: flags.except,
	}, selected)
}

func confirmYesNo(prompt string, defaultYes bool) bool {
	hint := "[Y/n]"
	if !defaultYes {
		hint = "[y/N]"
	}
	fmt.Printf("%s %s ", prompt, hint)
	var line string
	_, _ = fmt.Scanln(&line)
	line = strings.TrimSpace(strings.ToLower(line))
	if line == "" {
		return defaultYes
	}
	return line == "y" || line == "yes"
}

type cliFlags struct {
	command     string
	local       bool
	yes         bool
	help        bool
	version     bool
	only        []string
	except      []string
	targets     []string
	catalogPath string
}

func parseArgs(args []string) (cliFlags, error) {
	f := cliFlags{command: "install"}
	i := 0
	if len(args) > 0 && isCommand(args[0]) {
		f.command = args[0]
		i = 1
	}
	for ; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-l" || arg == "--local":
			f.local = true
		case arg == "-y" || arg == "--yes":
			f.yes = true
		case arg == "-h" || arg == "--help":
			f.help = true
		case arg == "-v" || arg == "--version":
			f.version = true
		case arg == "--only":
			if i+1 >= len(args) {
				return f, fmt.Errorf("--only requires a value")
			}
			i++
			f.only = splitList(args[i])
		case strings.HasPrefix(arg, "--only="):
			f.only = splitList(strings.TrimPrefix(arg, "--only="))
		case arg == "--except":
			if i+1 >= len(args) {
				return f, fmt.Errorf("--except requires a value")
			}
			i++
			f.except = splitList(args[i])
		case strings.HasPrefix(arg, "--except="):
			f.except = splitList(strings.TrimPrefix(arg, "--except="))
		case arg == "--catalog":
			if i+1 >= len(args) {
				return f, fmt.Errorf("--catalog requires a path")
			}
			i++
			f.catalogPath = args[i]
		case strings.HasPrefix(arg, "--catalog="):
			f.catalogPath = strings.TrimPrefix(arg, "--catalog=")
		case f.command == "remove" && !strings.HasPrefix(arg, "-"):
			f.targets = append(f.targets, arg)
		default:
			return f, fmt.Errorf("unknown argument: %s", arg)
		}
	}
	return f, nil
}

func isCommand(s string) bool {
	switch s {
	case "install", "status", "update", "doctor", "remove", "skills":
		return true
	default:
		return false
	}
}

func splitList(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func printHelp() {
	fmt.Printf(`my-pi-package — opinionated installer for Pi extensions
version %s (%s) built %s

Usage:
  my-pi-package [command] [options]

Commands:
  install   Install the selected catalog (default)
  remove    Remove a catalog package by id (or pass a raw pi source)
  status    Show which catalog packages are installed
  update    Run pi update for installed Pi packages
  doctor    Check your environment for common problems
  skills    Install or remove agent skills from a marketplace

Install options:
  --only <list>       Install only the given categories or package ids
  --except <list>     Install everything except the given categories or ids
  -l, --local         Install into the current project (.pi/settings.json)
  -y, --yes           Skip the picker and any confirmation prompt
  --catalog <path>    Use a custom catalog YAML (default: embedded)
  -h, --help          Show this help
  -v, --version       Print version

Default behaviour:
  - Every catalog package is installed by default.
  - On a TTY, an interactive picker appears; untick packages before confirming.
  - With --yes, --only, or --except the picker is skipped.

Examples:
  my-pi-package
  my-pi-package --yes
  my-pi-package --only core
  my-pi-package --only subagents,mcp
  my-pi-package status
  my-pi-package doctor
  my-pi-package skills install ipfans/dev-plugins
  my-pi-package skills remove
  my-pi-package skills --help
`, version, commit, date)
}

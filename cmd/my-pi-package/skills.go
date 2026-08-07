package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/ipfans/my-pi-package/settings"
	"github.com/ipfans/my-pi-package/skills"
	"github.com/ipfans/my-pi-package/tui"
)

type skillsFlags struct {
	action string // install | remove
	source string
	local  bool
	yes    bool
	help   bool
	all    bool
	only   []string
}

func parseSkillsArgs(args []string) (skillsFlags, error) {
	var f skillsFlags
	if len(args) == 0 {
		f.help = true
		return f, nil
	}
	switch args[0] {
	case "-h", "--help":
		f.help = true
		return f, nil
	case "install", "remove":
		f.action = args[0]
	default:
		return f, fmt.Errorf("unknown skills command %q (want install or remove)", args[0])
	}

	for i := 1; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-h" || arg == "--help":
			f.help = true
		case arg == "-l" || arg == "--local":
			f.local = true
		case arg == "-y" || arg == "--yes":
			f.yes = true
		case arg == "--all":
			f.all = true
		case arg == "--only":
			if i+1 >= len(args) {
				return f, fmt.Errorf("--only requires a value")
			}
			i++
			f.only = splitList(args[i])
		case strings.HasPrefix(arg, "--only="):
			f.only = splitList(strings.TrimPrefix(arg, "--only="))
		case f.action == "install" && !strings.HasPrefix(arg, "-"):
			if f.source != "" {
				return f, fmt.Errorf("unexpected argument %q", arg)
			}
			f.source = arg
		default:
			return f, fmt.Errorf("unknown argument: %s", arg)
		}
	}

	if f.help {
		return f, nil
	}
	if f.action == "install" && f.source == "" {
		return f, fmt.Errorf("usage: my-pi-package skills install <owner/repo|url|path>")
	}
	return f, nil
}

func runSkills(args []string) int {
	f, err := parseSkillsArgs(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		printSkillsHelp()
		return 2
	}
	if f.help || f.action == "" {
		printSkillsHelp()
		return 0
	}
	switch f.action {
	case "install":
		return cmdSkillsInstall(f)
	case "remove":
		return cmdSkillsRemove(f)
	default:
		printSkillsHelp()
		return 2
	}
}

func cmdSkillsInstall(f skillsFlags) int {
	paths := settings.Paths{Local: f.local}
	target := skills.Dir(paths)

	fmt.Printf("Opening %s …\n", f.source)
	repo, err := skills.OpenRepo(f.source)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		if strings.Contains(err.Error(), "git is not on PATH") {
			return 127
		}
		return 1
	}
	defer repo.Close()

	m, rel, err := skills.LoadMarketplace(repo.Root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 2
	}
	plugins, err := skills.DiscoverPluginsWithSkills(repo.Root, m)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	if len(plugins) == 0 {
		fmt.Fprintln(os.Stderr, "no plugins with a skills/ directory found in marketplace")
		return 2
	}

	fmt.Printf("Source:   %s\n", repo.SourceURL)
	name := m.Name
	if name == "" {
		name = "(unnamed)"
	}
	fmt.Printf("Manifest: %s (%s)\n", rel, name)
	fmt.Printf("Target:   %s\n", target)
	fmt.Printf("Plugins with skills: %d\n\n", len(plugins))

	selected, code := selectPlugins(f, plugins)
	if code != 0 {
		return code
	}
	if selected == nil {
		return 0 // aborted
	}

	fmt.Println("Installing skills …")
	res, err := skills.Install(target, plugins, selected)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	fmt.Printf("\nInstalled %d, replaced %d, failed %d\n", res.Installed, res.Replaced, res.Failed)
	if res.Failed > 0 {
		return 1
	}
	return 0
}

func selectPlugins(f skillsFlags, plugins []skills.PluginSkills) ([]string, int) {
	interactive := !f.yes && tui.IsInteractive()
	if interactive {
		items := make([]tui.PickItem, 0, len(plugins))
		for _, p := range plugins {
			n := len(p.Skills)
			desc := fmt.Sprintf("(%d skill%s)", n, plural(n))
			if p.Description != "" {
				desc = desc + "  " + p.Description
			}
			items = append(items, tui.PickItem{
				ID:          p.Name,
				Title:       p.Name,
				Description: desc,
			})
		}
		res, err := tui.RunMultiSelect("Select plugins to install skills from", items, tui.MultiSelectOpts{
			DefaultAll:  true,
			MinSelected: 1,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "tui: %v\n", err)
			return nil, 1
		}
		if res.Aborted {
			fmt.Println("Aborted.")
			return nil, 0
		}
		return res.Selected, 0
	}

	// non-interactive
	if f.all {
		return skills.PluginNames(plugins), 0
	}
	if len(f.only) > 0 {
		names, err := skills.SelectByOnly(plugins, f.only)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return nil, 2
		}
		return names, 0
	}
	fmt.Fprintln(os.Stderr, "non-interactive install requires --all or --only <plugins>")
	return nil, 2
}

func cmdSkillsRemove(f skillsFlags) int {
	paths := settings.Paths{Local: f.local}
	target := skills.Dir(paths)

	installed, err := skills.ListInstalled(target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	if len(installed) == 0 {
		fmt.Printf("No skills installed under %s\n", target)
		return 0
	}

	fmt.Printf("Target: %s (%d skill%s)\n\n", target, len(installed), plural(len(installed)))

	selected, code := selectSkillsToRemove(f, installed)
	if code != 0 {
		return code
	}
	if selected == nil {
		return 0
	}

	fmt.Println("Removing skills …")
	n, err := skills.Remove(target, selected)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	fmt.Printf("\nRemoved %d skill%s.\n", n, plural(n))
	return 0
}

func selectSkillsToRemove(f skillsFlags, installed []string) ([]string, int) {
	interactive := !f.yes && tui.IsInteractive()
	if interactive {
		items := make([]tui.PickItem, 0, len(installed))
		for _, name := range installed {
			items = append(items, tui.PickItem{ID: name, Title: name})
		}
		res, err := tui.RunMultiSelect("Select skills to remove", items, tui.MultiSelectOpts{
			DefaultAll:  false,
			MinSelected: 1,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "tui: %v\n", err)
			return nil, 1
		}
		if res.Aborted {
			fmt.Println("Aborted.")
			return nil, 0
		}
		return res.Selected, 0
	}

	if len(f.only) == 0 {
		fmt.Fprintln(os.Stderr, "non-interactive remove requires --only <skill names>")
		return nil, 2
	}
	if err := skills.ValidateSkillSelection(installed, f.only); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return nil, 2
	}
	return skills.FilterInstalled(installed, f.only), 0
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func printSkillsHelp() {
	fmt.Printf(`my-pi-package skills — install or remove Pi agent skills from marketplaces

Usage:
  my-pi-package skills install <source> [options]
  my-pi-package skills remove [options]

Source for install:
  owner/repo     Clone https://github.com/owner/repo.git (e.g. ipfans/dev-plugins)
  git URL        Any https:// or git@ remote
  local path     Existing marketplace directory

Options:
  -l, --local        Use project .pi/skills instead of ~/.pi/agent/skills
  -y, --yes          Non-interactive (no TUI)
  --all              With -y on install: install every plugin that has skills/
  --only <list>      With -y: plugin names (install) or skill names (remove)
  -h, --help         Show this help

Interactive (TTY, no -y):
  install  → multi-select plugins (default: all checked), then copy skills (overwrite)
  remove   → multi-select installed skills (must pick ≥1), then delete

Examples:
  my-pi-package skills install ipfans/dev-plugins
  my-pi-package skills install ./path/to/marketplace
  my-pi-package skills install ipfans/dev-plugins -y --all
  my-pi-package skills install ipfans/dev-plugins -y --only dev-flow,codex-goal
  my-pi-package skills remove
  my-pi-package skills remove -y --only ce-plan,ce-work
`)
}

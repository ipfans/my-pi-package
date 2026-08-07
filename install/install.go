// Package install orchestrates my-pi-package install, update, and remove flows.
package install

import (
	"fmt"
	"os"
	"strings"

	"github.com/ipfans/my-pi-package/catalog"
	"github.com/ipfans/my-pi-package/pi"
	"github.com/ipfans/my-pi-package/settings"
)

// Flags for install/update/remove.
type Flags struct {
	Local    bool
	Yes      bool
	Only     []string
	Except   []string
	ForceIDs map[string]struct{}
	Targets  []string // remove targets
}

// Result is a command exit code wrapper.
type Result struct {
	Code int
}

// EnsurePi installs pi if missing. Returns false if pi is unavailable.
func EnsurePi(cat *catalog.Catalog, yes bool, confirm func() bool) bool {
	if pi.HasCommand("pi") {
		return true
	}
	fmt.Fprintln(os.Stderr, "Could not find the `pi` command on PATH.")
	ok := yes
	if !ok && confirm != nil {
		ok = confirm()
	}
	if !ok {
		fmt.Fprintln(os.Stderr, "Install Pi first, then re-run my-pi-package.")
		return false
	}
	spec := cat.PiInstallSpec()
	fmt.Printf("Installing Pi via `npm install -g %s`\n", spec)
	if code := pi.InstallPiGlobal(spec); code != 0 {
		fmt.Fprintf(os.Stderr, "Failed to install Pi. On some systems try:\n  sudo npm install -g %s\n", spec)
		return false
	}
	if !pi.HasCommand("pi") {
		fmt.Fprintln(os.Stderr, "Installed Pi, but `pi` is still not on PATH. Open a new shell and re-run my-pi-package.")
		return false
	}
	return true
}

type installStatus struct {
	installed bool
	legacy    bool
	present   bool
}

func packageStatus(pkg catalog.Package, sources map[string]struct{}, paths settings.Paths) installStatus {
	if pkg.Type == catalog.TypeCompound {
		mode := readCompoundState(paths).mode
		return installStatus{
			installed: mode == compoundManifest,
			legacy:    mode == compoundLegacy,
			present:   mode == compoundManifest || mode == compoundLegacy,
		}
	}
	src := pkg.Source()
	_, installed := sources[src]
	var legacyHits []string
	for _, leg := range pkg.LegacySources {
		if _, ok := sources[leg]; ok {
			legacyHits = append(legacyHits, leg)
		}
	}
	return installStatus{
		installed: installed,
		legacy:    len(legacyHits) > 0,
		present:   installed || len(legacyHits) > 0,
	}
}

func findLegacySources(pkg catalog.Package, sources map[string]struct{}) []string {
	var out []string
	for _, leg := range pkg.LegacySources {
		if _, ok := sources[leg]; ok {
			out = append(out, leg)
		}
	}
	return out
}

// RunInstall installs the selected catalog packages.
func RunInstall(cat *catalog.Catalog, flags Flags, selected map[string]struct{}) int {
	selected = cat.ExpandDependsOn(selected)
	pkgs := cat.SelectedPackages(selected)
	if len(pkgs) == 0 {
		fmt.Println("Nothing selected — nothing to install.")
		return 0
	}

	paths := settings.Paths{Local: flags.Local}
	settingsPath := paths.SettingsPath()
	doc, _, err := settings.Read(settingsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not parse %s — %v\n", settingsPath, err)
	}
	sources := settings.InstalledSources(doc)

	var toInstall, already, legacy []catalog.Package
	for _, pkg := range pkgs {
		st := packageStatus(pkg, sources, paths)
		if _, force := flags.ForceIDs[pkg.ID]; force {
			toInstall = append(toInstall, pkg)
			continue
		}
		if pkg.Type == catalog.TypeCompound {
			if !isCompoundInstalled(paths) || compoundNeedsMigration(paths) {
				toInstall = append(toInstall, pkg)
				if compoundNeedsMigration(paths) {
					legacy = append(legacy, pkg)
				}
			} else {
				already = append(already, pkg)
			}
			continue
		}
		if st.installed {
			already = append(already, pkg)
			continue
		}
		if st.present {
			legacy = append(legacy, pkg)
		}
		toInstall = append(toInstall, pkg)
	}

	scope := fmt.Sprintf("global (%s)", settingsPath)
	if flags.Local {
		scope = "project (.pi/settings.json)"
	}
	auth := detectAuth(paths)
	fmt.Printf("Target:            %s\n", scope)
	fmt.Printf("Selected:          %d/%d\n", len(pkgs), len(cat.Packages))
	fmt.Printf("Already installed: %d\n", len(already))
	label := fmt.Sprintf("%d", len(toInstall))
	if len(legacy) > 0 {
		label = fmt.Sprintf("%d (%d migrations)", len(toInstall), len(legacy))
	}
	fmt.Printf("Will install:      %s\n", label)
	fmt.Printf("Pi credentials:    %s\n", formatAuth(auth))

	for _, pkg := range pkgs {
		if pkg.ID == "subagents" {
			if _, err := settings.WriteSubagentOverrides(settingsPath); err != nil {
				fmt.Fprintf(os.Stderr, "Refusing to update %s because it is not valid JSON (%v). Fix the file first, then rerun my-pi-package.\n", settingsPath, err)
				return 2
			}
			break
		}
	}

	if backup, changed, err := settings.NormalizeLoadOrder(settingsPath, cat); err != nil {
		fmt.Fprintf(os.Stderr, "Could not update package load order: %v\n", err)
	} else if changed {
		msg := fmt.Sprintf("Updated package load order in %s: extension-settings loads first.", settingsPath)
		if backup != "" {
			msg += " Backup: " + backup
		}
		fmt.Println(msg)
	}

	if len(toInstall) == 0 {
		printCheatsheet(pkgs)
		printNextSteps(auth, 0)
		fmt.Println("Nothing to do — every selected package is already installed.")
		return 0
	}

	var failed, skipped []catalog.Package
	for _, pkg := range toInstall {
		if pkg.Type == catalog.TypeCompound {
			status, reason, _ := installCompound(pkg, paths)
			switch status {
			case "failed":
				failed = append(failed, pkg)
				fmt.Fprintf(os.Stderr, "  ✗ failed to install %s (%s)\n", pkg.ID, reason)
			case "skipped":
				skipped = append(skipped, pkg)
				fmt.Printf("  ! %s\n", reason)
			}
			continue
		}

		legacySources := findLegacySources(pkg, sources)
		migOK := true
		for _, leg := range legacySources {
			fmt.Printf("\n→ pi remove %s\n", leg)
			if code := pi.RemovePackage(leg, flags.Local); code != 0 {
				failed = append(failed, pkg)
				fmt.Fprintf(os.Stderr, "  ✗ failed to migrate %s\n", pkg.ID)
				migOK = false
				break
			}
		}
		if !migOK {
			continue
		}

		src := pkg.Source()
		fmt.Printf("\n→ pi install %s\n", src)
		if code := pi.InstallPackage(src, flags.Local, pkg.Type == catalog.TypeGit); code != 0 {
			failed = append(failed, pkg)
			fmt.Fprintf(os.Stderr, "  ✗ failed to install %s\n", pkg.ID)
		}
	}

	if _, changed, err := settings.NormalizeLoadOrder(settingsPath, cat); err != nil {
		fmt.Fprintf(os.Stderr, "Could not update package load order: %v\n", err)
	} else if changed {
		fmt.Printf("Updated package load order in %s: extension-settings loads first.\n", settingsPath)
	}

	if len(failed) > 0 {
		fmt.Fprintf(os.Stderr, "\nmy-pi-package finished with %d failure(s):\n", len(failed))
		for _, p := range failed {
			fmt.Fprintf(os.Stderr, "- %s (%s)\n", p.ID, p.Source())
		}
		return 1
	}

	if len(skipped) > 0 {
		fmt.Println("\nSkipped packages:")
		for _, p := range skipped {
			fmt.Printf("- %s (%s)\n", p.ID, p.Source())
		}
	}
	installedCount := len(toInstall) - len(skipped)
	printCheatsheet(pkgs)
	printNextSteps(auth, installedCount)
	return 0
}

func printCheatsheet(pkgs []catalog.Package) {
	if len(pkgs) == 0 {
		return
	}
	fmt.Println("\nWhat you've got:")
	idWidth := 0
	for _, p := range pkgs {
		if len(p.ID) > idWidth {
			idWidth = len(p.ID)
		}
	}
	for _, p := range pkgs {
		fmt.Printf("  %-*s %s\n", idWidth+2, p.ID, p.Hint)
	}
}

func printNextSteps(auth authState, installedCount int) {
	title := "Next steps"
	if installedCount > 0 {
		title = fmt.Sprintf("Installed %d package(s) — next steps", installedCount)
	}
	fmt.Printf("\n%s:\n", title)
	if auth.authed {
		fmt.Printf("Pi credentials: %s\n\nYou're all set. Run `pi` to get started.\n", formatAuth(auth))
		return
	}
	fmt.Println("Pi credentials: none detected.")
	fmt.Println()
	fmt.Println("Run `pi`, then type `/login` inside Pi to sign in with a")
	fmt.Println("subscription (Claude Pro/Max, ChatGPT Plus/Pro, Copilot, Gemini)")
	fmt.Println("or set a provider env var (ANTHROPIC_API_KEY, OPENAI_API_KEY, …)")
	fmt.Println("before launching pi.")
}

// RunUpdate refreshes compound (if installed) then pi update.
func RunUpdate(cat *catalog.Catalog, flags Flags) int {
	if !EnsurePi(cat, flags.Yes, nil) {
		return 127
	}
	selected, err := cat.ResolveSelection(flags.Only, flags.Except)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	selected = cat.ExpandDependsOn(selected)
	paths := settings.Paths{Local: flags.Local}
	doc, _, _ := settings.Read(paths.SettingsPath())
	sources := settings.InstalledSources(doc)

	var presentIDs []string
	for _, pkg := range cat.SelectedPackages(selected) {
		if packageStatus(pkg, sources, paths).present {
			presentIDs = append(presentIDs, pkg.ID)
		}
	}

	if backup, changed, err := settings.NormalizeLoadOrder(paths.SettingsPath(), cat); err == nil && changed {
		fmt.Printf("Updated package load order in %s.%s\n", paths.SettingsPath(), backupSuffix(backup))
	}

	hasCompound := false
	for _, id := range presentIDs {
		if id == "compound" {
			hasCompound = true
			break
		}
	}
	if hasCompound {
		fmt.Println("Step 1/2: refresh Compound Engineering")
		force := map[string]struct{}{"compound": {}}
		code := RunInstall(cat, Flags{Local: flags.Local, Yes: true, ForceIDs: force}, map[string]struct{}{"compound": {}})
		if code != 0 {
			return code
		}
		fmt.Println("\nStep 2/2: pi update")
	} else {
		fmt.Println("pi update")
	}
	return pi.Update(flags.Local)
}

func backupSuffix(backup string) string {
	if backup == "" {
		return ""
	}
	return " Backup: " + backup
}

// RunRemove removes packages by id or raw source.
func RunRemove(cat *catalog.Catalog, flags Flags, targets []string) int {
	if len(targets) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: my-pi-package remove <id|source> [...]")
		return 2
	}
	paths := settings.Paths{Local: flags.Local}
	doc, _, _ := settings.Read(paths.SettingsPath())
	sources := settings.InstalledSources(doc)
	exitCode := 0

	for _, target := range targets {
		pkg := cat.ByID(target)
		if pkg != nil && pkg.Type == catalog.TypeCompound {
			if code := removeCompound(paths); code != 0 {
				fmt.Fprintf(os.Stderr, "Failed to remove %s\n", target)
				exitCode = 1
			}
			continue
		}
		if pkg != nil {
			var toRemove []string
			if _, ok := sources[pkg.Source()]; ok {
				toRemove = append(toRemove, pkg.Source())
			}
			toRemove = append(toRemove, findLegacySources(*pkg, sources)...)
			if len(toRemove) == 0 {
				toRemove = []string{pkg.Source()}
			}
			seen := map[string]struct{}{}
			for _, src := range toRemove {
				if _, ok := seen[src]; ok {
					continue
				}
				seen[src] = struct{}{}
				if code := pi.RemovePackage(src, flags.Local); code != 0 {
					fmt.Fprintf(os.Stderr, "Failed to remove %s\n", target)
					exitCode = 1
					break
				}
			}
			continue
		}
		// raw source
		if target == "npm:@every-env/compound-plugin" || strings.HasPrefix(target, "npm:@every-env/compound-plugin@") {
			if code := removeCompound(paths); code != 0 {
				exitCode = 1
			}
			continue
		}
		if code := pi.RemovePackage(target, flags.Local); code != 0 {
			fmt.Fprintf(os.Stderr, "Failed to remove %s\n", target)
			exitCode = 1
		}
	}
	return exitCode
}

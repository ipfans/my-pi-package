package install

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/ipfans/my-pi-package/catalog"
	"github.com/ipfans/my-pi-package/pi"
	"github.com/ipfans/my-pi-package/settings"
)

// RunDoctor checks environment health.
func RunDoctor(cat *catalog.Catalog, local bool) int {
	problems, warnings := 0, 0
	pass := func(msg string) { fmt.Printf("  ✓ %s\n", msg) }
	warn := func(msg string, fatal bool) {
		fmt.Printf("  ! %s\n", msg)
		if fatal {
			problems++
		} else {
			warnings++
		}
	}
	fail := func(msg string) {
		fmt.Printf("  ✗ %s\n", msg)
		problems++
	}

	fmt.Println("\nEnvironment")
	major, _ := strconv.Atoi(strings.Split(runtime.Version()[2:], ".")[0])
	// runtime.Version is like "go1.26.5" — better parse GOVERSION from go
	// Use compiler version from runtime
	if ver := runtime.Version(); strings.HasPrefix(ver, "go") {
		pass("Go runtime " + ver + " (my-pi-package binary)")
	}
	_ = major

	// Node check via command
	if out, code := pi.RunOutput("node", []string{"--version"}); code == 0 {
		pass("Node " + strings.TrimSpace(out))
	} else {
		fail("node is not on PATH — required for npm installs")
	}
	if pi.HasCommand("npm") {
		pass("npm is on PATH")
	} else {
		fail("npm is not on PATH — my-pi-package can't install Pi for you")
	}
	if pi.HasCommand("git") {
		pass("git is on PATH")
	} else {
		warn("git is not on PATH — required by git-based catalog packages", true)
	}
	if pi.HasCommand("bun") {
		pass("bun is on PATH — required for official Compound Engineering from Every")
	} else {
		warn("bun is not on PATH — official Compound Engineering from Every will be skipped by my-pi-package", false)
	}

	fmt.Println("\nPi")
	if pi.HasCommand("pi") {
		pass("`pi` is on PATH")
		if v := pi.Version(); v != "" {
			pass("pi --version: " + v)
		} else {
			warn("Could not read `pi --version` output", true)
		}
	} else {
		fail("`pi` is not on PATH — run `my-pi-package` to install it")
	}

	fmt.Println("\nSettings")
	paths := settings.Paths{Local: local}
	path := paths.SettingsPath()
	doc, exists, err := settings.Read(path)
	if !exists {
		warn(path+" does not exist yet (Pi has not been run)", true)
	} else if err != nil {
		fail(fmt.Sprintf("%s is not valid JSON — %v", path, err))
	} else {
		pass(path + " is readable")
		if checked, ok := settings.LoadOrderOK(doc, cat); checked && ok {
			pass("pi-extension-settings loads before pi-powerbar")
		} else if checked {
			warn("pi-extension-settings loads after pi-powerbar — run `my-pi-package --yes` to repair package load order", true)
		}
	}

	fmt.Println("\nCatalog package health")
	st := readCompoundState(paths)
	switch st.mode {
	case compoundInvalidManifest:
		fail("Compound Engineering manifest is invalid — " + st.err)
	case compoundInvalidLegacy:
		fail("Compound Engineering legacy state is invalid — " + st.err)
	case compoundManifest:
		pass("Compound Engineering manifest found at " + st.manifestPath)
		root := paths.InstallRoot()
		if _, err := os.Stat(filepath.Join(root, "skills")); err == nil {
			pass("Compound Engineering skills directory exists")
		} else {
			fail("Compound Engineering skills directory is missing")
		}
		if _, err := os.Stat(filepath.Join(root, "agents")); err == nil {
			pass("Compound Engineering agents directory exists")
		} else {
			fail("Compound Engineering agents directory is missing")
		}
		sources := settings.InstalledSources(doc)
		if sub := cat.ByID("subagents"); sub != nil {
			if _, ok := sources[sub.Source()]; ok {
				pass("pi-subagents installed")
			} else {
				fail("pi-subagents is missing — Compound Engineering requires it")
			}
		}
		if ask := cat.ByID("pi-ask-user"); ask != nil {
			if _, ok := sources[ask.Source()]; ok {
				pass("pi-ask-user installed")
			} else {
				fail("pi-ask-user is missing — Compound Engineering requires it")
			}
		}
	case compoundLegacy:
		warn("Legacy Compound Engineering marker found — run `my-pi-package update` to migrate to CE 3", true)
	default:
		fmt.Println("  · Compound Engineering not installed")
	}

	fmt.Println("\nAuth")
	auth := detectAuth(paths)
	for _, e := range auth.envProviders {
		pass("env var " + e)
	}
	if len(auth.fileProviders) > 0 {
		pass(auth.path + " → " + strings.Join(auth.fileProviders, ", "))
	}
	if !auth.authed {
		warn("No credentials detected — run `pi` then `/login`, or export a provider API key", false)
	}

	fmt.Println()
	if problems == 0 && warnings == 0 {
		fmt.Println("All checks passed.")
		return 0
	}
	if problems == 0 {
		fmt.Printf("%d warning(s) found.\n", warnings)
		return 0
	}
	fmt.Printf("%d problem(s) found", problems)
	if warnings > 0 {
		fmt.Printf(", %d warning(s)", warnings)
	}
	fmt.Println(".")
	return 1
}

package install

import (
	"fmt"

	"github.com/ipfans/my-pi-package/catalog"
	"github.com/ipfans/my-pi-package/settings"
)

// RunStatus prints installed / legacy / missing / other packages.
func RunStatus(cat *catalog.Catalog, local bool) int {
	paths := settings.Paths{Local: local}
	path := paths.SettingsPath()
	fmt.Printf("Settings file: %s\n", path)

	doc, exists, err := settings.Read(path)
	if !exists {
		fmt.Println("  (not found — Pi has not written settings yet)")
	} else if err != nil {
		fmt.Printf("  could not parse: %v\n", err)
		return 1
	}

	sources := settings.InstalledSources(doc)
	catalogSources := map[string]struct{}{}
	for _, p := range cat.Packages {
		catalogSources[p.Source()] = struct{}{}
		for _, leg := range p.LegacySources {
			catalogSources[leg] = struct{}{}
		}
	}

	var installed, legacy, missing []catalog.Package
	for _, pkg := range cat.Packages {
		st := packageStatus(pkg, sources, paths)
		switch {
		case st.installed:
			installed = append(installed, pkg)
		case st.legacy:
			legacy = append(legacy, pkg)
		default:
			missing = append(missing, pkg)
		}
	}

	var others []string
	for src := range sources {
		if _, ok := catalogSources[src]; !ok {
			others = append(others, src)
		}
	}

	fmt.Printf("\nInstalled from my-pi-package catalog (%d/%d):\n", len(installed), len(cat.Packages))
	if len(installed) == 0 {
		fmt.Println("  none")
	}
	for _, pkg := range installed {
		fmt.Printf("  ✓ [%s] %-20s %s\n", pkg.Category, pkg.ID, pkg.Source())
	}

	fmt.Printf("\nInstalled with legacy catalog sources (%d):\n", len(legacy))
	if len(legacy) == 0 {
		fmt.Println("  none")
	}
	for _, pkg := range legacy {
		detail := ""
		if pkg.Type == catalog.TypeCompound {
			detail = "legacy my-pi-package state — run `my-pi-package update` to migrate to CE 3"
		} else {
			detail = joinSources(findLegacySources(pkg, sources))
		}
		fmt.Printf("  ! [%s] %-20s %s\n", pkg.Category, pkg.ID, detail)
	}

	fmt.Printf("\nMissing from my-pi-package catalog (%d):\n", len(missing))
	if len(missing) == 0 {
		fmt.Println("  none — full catalog is installed")
	}
	for _, pkg := range missing {
		fmt.Printf("  · [%s] %-20s %s\n", pkg.Category, pkg.ID, pkg.Source())
	}

	fmt.Printf("\nOther Pi packages outside the my-pi-package catalog (%d):\n", len(others))
	if len(others) == 0 {
		fmt.Println("  none")
	}
	for _, src := range others {
		fmt.Printf("  · %s\n", src)
	}
	return 0
}

func joinSources(ss []string) string {
	if len(ss) == 0 {
		return ""
	}
	out := ss[0]
	for i := 1; i < len(ss); i++ {
		out += ", " + ss[i]
	}
	return out
}

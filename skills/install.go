package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

// InstallResult summarizes a skills install run.
type InstallResult struct {
	Installed int
	Replaced  int
	Failed    int
}

// Install copies skills from selected plugins into targetDir.
// Existing skill directories are always replaced.
// pluginOrder is the selected plugin names; installation follows this order
// (later plugins win on name collisions).
func Install(targetDir string, plugins []PluginSkills, pluginOrder []string) (InstallResult, error) {
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return InstallResult{}, fmt.Errorf("creating skills dir %s: %w", targetDir, err)
	}

	byName := make(map[string]PluginSkills, len(plugins))
	for _, p := range plugins {
		byName[p.Name] = p
	}

	var res InstallResult
	for _, name := range pluginOrder {
		p, ok := byName[name]
		if !ok {
			return res, fmt.Errorf("unknown plugin %q", name)
		}
		for _, skillName := range p.SkillNames() {
			src := p.Skills[skillName]
			dest := filepath.Join(targetDir, skillName)
			replaced := false
			if st, err := os.Stat(dest); err == nil && st.IsDir() {
				if err := os.RemoveAll(dest); err != nil {
					res.Failed++
					return res, fmt.Errorf("removing existing skill %s: %w", skillName, err)
				}
				replaced = true
			} else if err != nil && !os.IsNotExist(err) {
				res.Failed++
				return res, fmt.Errorf("stat %s: %w", dest, err)
			}
			if err := copyDir(src, dest); err != nil {
				res.Failed++
				return res, fmt.Errorf("copying skill %s from plugin %s: %w", skillName, p.Name, err)
			}
			if replaced {
				res.Replaced++
				fmt.Printf("  ~ %-24s (%s, replaced)\n", skillName, p.Name)
			} else {
				res.Installed++
				fmt.Printf("  + %-24s (%s)\n", skillName, p.Name)
			}
		}
	}
	return res, nil
}

// FilterPlugins returns plugins whose names are in selected (preserving plugins order).
func FilterPlugins(plugins []PluginSkills, selected []string) []PluginSkills {
	want := make(map[string]struct{}, len(selected))
	for _, s := range selected {
		want[s] = struct{}{}
	}
	var out []PluginSkills
	for _, p := range plugins {
		if _, ok := want[p.Name]; ok {
			out = append(out, p)
		}
	}
	return out
}

// PluginNames returns plugin names in discovery order.
func PluginNames(plugins []PluginSkills) []string {
	names := make([]string, len(plugins))
	for i, p := range plugins {
		names[i] = p.Name
	}
	return names
}

// ValidatePluginSelection ensures every name exists in plugins.
func ValidatePluginSelection(plugins []PluginSkills, names []string) error {
	have := make(map[string]struct{}, len(plugins))
	for _, p := range plugins {
		have[p.Name] = struct{}{}
	}
	for _, n := range names {
		if _, ok := have[n]; !ok {
			return fmt.Errorf("plugin %q not found in marketplace (or has no skills/)", n)
		}
	}
	if len(names) == 0 {
		return fmt.Errorf("select at least one plugin")
	}
	return nil
}

// SelectByOnly filters to --only list, preserving marketplace order.
func SelectByOnly(plugins []PluginSkills, only []string) ([]string, error) {
	if len(only) == 0 {
		return nil, fmt.Errorf("--only requires at least one name")
	}
	// de-dupe while preserving only order for validation messages
	seen := make(map[string]struct{})
	var ordered []string
	for _, n := range only {
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		ordered = append(ordered, n)
	}
	if err := ValidatePluginSelection(plugins, ordered); err != nil {
		return nil, err
	}
	// return in marketplace discovery order
	var out []string
	for _, p := range plugins {
		if slices.Contains(ordered, p.Name) {
			out = append(out, p.Name)
		}
	}
	return out, nil
}

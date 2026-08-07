package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// ListInstalled returns skill directory names under targetDir.
func ListInstalled(targetDir string) ([]string, error) {
	entries, err := os.ReadDir(targetDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading skills dir %s: %w", targetDir, err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		names = append(names, name)
	}
	slices.Sort(names)
	return names, nil
}

// Remove deletes the named skill directories under targetDir.
func Remove(targetDir string, names []string) (int, error) {
	if len(names) == 0 {
		return 0, fmt.Errorf("select at least one skill to remove")
	}
	removed := 0
	for _, name := range names {
		if name == "" || strings.Contains(name, string(os.PathSeparator)) || name == "." || name == ".." {
			return removed, fmt.Errorf("invalid skill name %q", name)
		}
		path := filepath.Join(targetDir, name)
		st, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				return removed, fmt.Errorf("skill %q is not installed", name)
			}
			return removed, err
		}
		if !st.IsDir() {
			return removed, fmt.Errorf("%q is not a skill directory", name)
		}
		if err := os.RemoveAll(path); err != nil {
			return removed, fmt.Errorf("removing %s: %w", name, err)
		}
		fmt.Printf("  - %s\n", name)
		removed++
	}
	return removed, nil
}

// ValidateSkillSelection checks that every name is in installed.
func ValidateSkillSelection(installed, names []string) error {
	if len(names) == 0 {
		return fmt.Errorf("select at least one skill")
	}
	have := make(map[string]struct{}, len(installed))
	for _, n := range installed {
		have[n] = struct{}{}
	}
	for _, n := range names {
		if _, ok := have[n]; !ok {
			return fmt.Errorf("skill %q is not installed", n)
		}
	}
	return nil
}

// FilterInstalled returns names that appear in installed, in installed order.
func FilterInstalled(installed, names []string) []string {
	var out []string
	for _, n := range installed {
		if slices.Contains(names, n) {
			out = append(out, n)
		}
	}
	return out
}

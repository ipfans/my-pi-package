package skills

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Manifest modes returned by Discover.
const (
	ModePlugin      = "plugin"
	ModeMarketplace = "marketplace"
)

// PluginManifest is the minimal .claude-plugin/plugin.json shape we need.
type PluginManifest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Skills      []string `json:"skills"` // relative dirs, e.g. "./skills/engineering/ask-matt"
}

// DiscoverResult is the unified discovery output for install wiring.
type DiscoverResult struct {
	Mode        string // ModePlugin or ModeMarketplace
	ManifestRel string
	Name        string
	// Plugin mode: one entry with all listed skills.
	// Marketplace mode: one entry per marketplace plugin that has skills.
	Plugins []PluginSkills
}

const pluginManifestRel = ".claude-plugin/plugin.json"

// Discover loads skills from a repo root.
// Priority: .claude-plugin/plugin.json, then marketplace.json paths.
func Discover(root string) (*DiscoverResult, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolving root: %w", err)
	}

	if pm, rel, err := loadPluginManifest(root); err == nil {
		ps, err := pluginSkillsFromManifest(root, pm)
		if err != nil {
			return nil, err
		}
		name := pm.Name
		if name == "" {
			name = "(unnamed)"
		}
		return &DiscoverResult{
			Mode:        ModePlugin,
			ManifestRel: rel,
			Name:        name,
			Plugins:     []PluginSkills{ps},
		}, nil
	} else if !os.IsNotExist(err) {
		// parse / validation errors from an existing plugin.json should surface
		return nil, err
	}

	m, rel, err := LoadMarketplace(root)
	if err != nil {
		return nil, err
	}
	plugins, err := DiscoverPluginsWithSkills(root, m)
	if err != nil {
		return nil, err
	}
	name := m.Name
	if name == "" {
		name = "(unnamed)"
	}
	return &DiscoverResult{
		Mode:        ModeMarketplace,
		ManifestRel: rel,
		Name:        name,
		Plugins:     plugins,
	}, nil
}

// loadPluginManifest reads .claude-plugin/plugin.json.
// Returns os.ErrNotExist when the file is absent so Discover can fall through.
func loadPluginManifest(root string) (*PluginManifest, string, error) {
	path := filepath.Join(root, filepath.FromSlash(pluginManifestRel))
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", err
		}
		return nil, "", fmt.Errorf("reading %s: %w", path, err)
	}
	var pm PluginManifest
	if err := json.Unmarshal(data, &pm); err != nil {
		return nil, "", fmt.Errorf("parsing %s: %w", path, err)
	}
	if len(pm.Skills) == 0 {
		return nil, "", fmt.Errorf("%s: no skills listed", path)
	}
	return &pm, pluginManifestRel, nil
}

// pluginSkillsFromManifest resolves each skills[] path under root into PluginSkills.
func pluginSkillsFromManifest(root string, pm *PluginManifest) (PluginSkills, error) {
	skills := make(map[string]string, len(pm.Skills))
	for i, raw := range pm.Skills {
		rel, err := cleanRel(raw)
		if err != nil {
			return PluginSkills{}, fmt.Errorf("skills[%d] %q: %w", i, raw, err)
		}
		dir, err := safeJoin(root, rel)
		if err != nil {
			return PluginSkills{}, fmt.Errorf("skills[%d] %q: %w", i, raw, err)
		}
		st, err := os.Stat(dir)
		if err != nil {
			if os.IsNotExist(err) {
				return PluginSkills{}, fmt.Errorf("skills[%d] %q: directory not found", i, raw)
			}
			return PluginSkills{}, fmt.Errorf("skills[%d] %q: %w", i, raw, err)
		}
		if !st.IsDir() {
			return PluginSkills{}, fmt.Errorf("skills[%d] %q: not a directory", i, raw)
		}
		name := filepath.Base(dir)
		if name == "" || name == "." || name == string(os.PathSeparator) {
			return PluginSkills{}, fmt.Errorf("skills[%d] %q: empty skill name", i, raw)
		}
		if prev, ok := skills[name]; ok {
			return PluginSkills{}, fmt.Errorf("skills[%d] %q: duplicate skill name %q (also %s)",
				i, raw, name, prev)
		}
		skills[name] = dir
	}
	pluginName := pm.Name
	if pluginName == "" {
		pluginName = "plugin"
	}
	return PluginSkills{
		Name:        pluginName,
		Description: pm.Description,
		Skills:      skills,
	}, nil
}

// FilterSkills returns a copy of p with only the named skills (order from SkillNames of result).
func FilterSkills(p PluginSkills, names []string) (PluginSkills, error) {
	if len(names) == 0 {
		return PluginSkills{}, fmt.Errorf("select at least one skill")
	}
	seen := make(map[string]struct{}, len(names))
	var ordered []string
	for _, n := range names {
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		ordered = append(ordered, n)
	}
	out := make(map[string]string, len(ordered))
	for _, n := range ordered {
		src, ok := p.Skills[n]
		if !ok {
			return PluginSkills{}, fmt.Errorf("skill %q not found in plugin %q", n, p.Name)
		}
		out[n] = src
	}
	return PluginSkills{
		Name:        p.Name,
		Description: p.Description,
		Skills:      out,
	}, nil
}

// SelectSkillsByOnly resolves --only skill names against a plugin's skill map.
// Returns names in stable SkillNames order (sorted), after validation.
func SelectSkillsByOnly(p PluginSkills, only []string) ([]string, error) {
	if len(only) == 0 {
		return nil, fmt.Errorf("--only requires at least one name")
	}
	filtered, err := FilterSkills(p, only)
	if err != nil {
		return nil, err
	}
	return filtered.SkillNames(), nil
}


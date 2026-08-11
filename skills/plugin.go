package skills

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	Skills      PathList `json:"skills"` // string or []string relative paths
	// Optional component fields — only used for strict:false conflict detection.
	Commands     PathList        `json:"commands"`
	Agents       PathList        `json:"agents"`
	Hooks        json.RawMessage `json:"hooks"`
	MCPServers   json.RawMessage `json:"mcpServers"`
	OutputStyles PathList        `json:"outputStyles"`
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

// Discover loads skills from a repo root using Claude Code semantics.
//
// Priority:
//  1. marketplace.json (Claude distribution unit) when present
//  2. otherwise single-plugin discovery (plugin.json and/or default layout)
func Discover(root string) (*DiscoverResult, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolving root: %w", err)
	}

	if m, rel, err := LoadMarketplace(root); err == nil {
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
	} else if !isNoMarketplace(err) {
		return nil, err
	}

	ps, name, rel, err := discoverSinglePlugin(root)
	if err != nil {
		return nil, err
	}
	return &DiscoverResult{
		Mode:        ModePlugin,
		ManifestRel: rel,
		Name:        name,
		Plugins:     []PluginSkills{ps},
	}, nil
}

func isNoMarketplace(err error) bool {
	return err != nil && strings.Contains(err.Error(), "no marketplace.json found")
}

// discoverSinglePlugin resolves a plugin root (with optional plugin.json).
func discoverSinglePlugin(root string) (PluginSkills, string, string, error) {
	pm, ok, err := tryLoadPluginManifest(root)
	if err != nil {
		return PluginSkills{}, "", "", err
	}
	var skillsField PathList
	pluginName := filepath.Base(root)
	desc := ""
	manifestLabel := "(auto)"
	if ok && pm != nil {
		skillsField = pm.Skills
		if pm.Name != "" {
			pluginName = pm.Name
		}
		desc = pm.Description
		manifestLabel = pluginManifestRel
	}

	skills, err := resolveSkills(root, skillResolutionOptions{
		SkillsField:     skillsField,
		MarketplaceRoot: false,
		DefaultName:     pluginName,
	})
	if err != nil {
		return PluginSkills{}, "", "", err
	}
	if len(skills) == 0 {
		return PluginSkills{}, "", "", fmt.Errorf("no installable skills found under %s", root)
	}
	return PluginSkills{
		Name:        pluginName,
		Description: desc,
		Skills:      skills,
	}, pluginName, manifestLabel, nil
}

// tryLoadPluginManifest reads .claude-plugin/plugin.json when present.
// ok is false when the file is absent (not an error).
func tryLoadPluginManifest(root string) (*PluginManifest, bool, error) {
	path := filepath.Join(root, filepath.FromSlash(pluginManifestRel))
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("reading %s: %w", path, err)
	}
	var pm PluginManifest
	if err := json.Unmarshal(data, &pm); err != nil {
		return nil, false, fmt.Errorf("parsing %s: %w", path, err)
	}
	return &pm, true, nil
}

// loadPluginManifest reads plugin.json requiring a non-empty skills field.
// Kept for tests that assert explicit path validation; prefer tryLoad + resolveSkills.
func loadPluginManifest(root string) (*PluginManifest, string, error) {
	pm, ok, err := tryLoadPluginManifest(root)
	if err != nil {
		return nil, "", err
	}
	if !ok {
		return nil, "", os.ErrNotExist
	}
	if len(pm.Skills) == 0 {
		return nil, "", fmt.Errorf("%s: no skills listed", filepath.Join(root, pluginManifestRel))
	}
	return pm, pluginManifestRel, nil
}

// pluginSkillsFromManifest resolves skills[] paths under root into PluginSkills.
// Used by tests and as a thin wrapper around resolveSkills for explicit path lists.
func pluginSkillsFromManifest(root string, pm *PluginManifest) (PluginSkills, error) {
	if pm == nil {
		return PluginSkills{}, fmt.Errorf("nil plugin manifest")
	}
	// Explicit path list without default skills/ merge when only validating listed paths:
	// Claude additive rules still apply (skills/ + field). For pure path validation
	// of the listed entries (legacy tests), expand field paths and require them.
	pluginName := pm.Name
	if pluginName == "" {
		pluginName = "plugin"
	}
	skills, err := resolveSkills(root, skillResolutionOptions{
		SkillsField:     pm.Skills,
		MarketplaceRoot: false,
		DefaultName:     pluginName,
	})
	if err != nil {
		return PluginSkills{}, err
	}
	// If skills field was set, ensure each listed path existed (stricter for plugin.json).
	if len(pm.Skills) > 0 {
		_, any, err := expandSkillsField(root, pm.Skills, pluginName)
		if err != nil {
			return PluginSkills{}, err
		}
		if !any {
			return PluginSkills{}, fmt.Errorf("skills[0] %q: directory not found", pm.Skills[0])
		}
	}
	if len(skills) == 0 {
		return PluginSkills{}, fmt.Errorf("no installable skills in plugin %q", pluginName)
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

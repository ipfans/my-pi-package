// Package skills installs and removes Pi agent skills from Claude/Codex-style marketplaces.
package skills

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Marketplace is the minimal marketplace.json shape we need.
type Marketplace struct {
	Name    string   `json:"name"`
	Plugins []Plugin `json:"plugins"`
}

// Plugin is one marketplace plugin entry.
type Plugin struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Source      json.RawMessage `json:"source"`
}

// PluginSkills is a plugin that has one or more skill directories.
type PluginSkills struct {
	Name        string
	Description string
	// Skills maps skill name → absolute path of the skill directory.
	Skills map[string]string
}

// SkillNames returns skill basenames in stable sorted order.
func (p PluginSkills) SkillNames() []string {
	names := make([]string, 0, len(p.Skills))
	for name := range p.Skills {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// marketplace paths relative to repo root, first found wins.
var marketplaceRelPaths = []string{
	".claude-plugin/marketplace.json",
	".agents/plugins/marketplace.json",
}

// LoadMarketplace reads marketplace.json from a cloned or local marketplace root.
func LoadMarketplace(root string) (*Marketplace, string, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, "", fmt.Errorf("resolving marketplace root: %w", err)
	}
	for _, rel := range marketplaceRelPaths {
		path := filepath.Join(root, filepath.FromSlash(rel))
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, "", fmt.Errorf("reading %s: %w", path, err)
		}
		var m Marketplace
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, "", fmt.Errorf("parsing %s: %w", path, err)
		}
		if len(m.Plugins) == 0 {
			return nil, "", fmt.Errorf("%s: no plugins listed", path)
		}
		return &m, rel, nil
	}
	return nil, "", fmt.Errorf("no marketplace.json found under %s (tried %s)",
		root, strings.Join(marketplaceRelPaths, ", "))
}

// DiscoverPluginsWithSkills resolves each plugin source and lists skill directories.
// Plugins without a skills/ directory are omitted.
func DiscoverPluginsWithSkills(root string, m *Marketplace) ([]PluginSkills, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolving root: %w", err)
	}
	var out []PluginSkills
	for _, p := range m.Plugins {
		if p.Name == "" {
			return nil, fmt.Errorf("marketplace plugin missing name")
		}
		rel, err := resolvePluginSource(p.Source)
		if err != nil {
			return nil, fmt.Errorf("plugin %q: %w", p.Name, err)
		}
		pluginDir, err := safeJoin(root, rel)
		if err != nil {
			return nil, fmt.Errorf("plugin %q: %w", p.Name, err)
		}
		skillsDir := filepath.Join(pluginDir, "skills")
		st, err := os.Stat(skillsDir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("plugin %q: stat skills: %w", p.Name, err)
		}
		if !st.IsDir() {
			continue
		}
		skills, err := listSkillDirs(skillsDir)
		if err != nil {
			return nil, fmt.Errorf("plugin %q: %w", p.Name, err)
		}
		if len(skills) == 0 {
			continue
		}
		out = append(out, PluginSkills{
			Name:        p.Name,
			Description: p.Description,
			Skills:      skills,
		})
	}
	return out, nil
}

// resolvePluginSource turns marketplace "source" (string or object) into a relative path.
func resolvePluginSource(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", fmt.Errorf("missing source")
	}
	// string form: "./plugins/dev-flow"
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return cleanRel(asString)
	}
	// object form
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return "", fmt.Errorf("invalid source: %w", err)
	}
	// prefer path, then url when local-relative
	if path, ok := stringField(obj, "path"); ok {
		return cleanRel(path)
	}
	if url, ok := stringField(obj, "url"); ok {
		if isRemoteURL(url) {
			return "", fmt.Errorf("remote source url %q is not supported (plugin must live in the marketplace tree)", url)
		}
		return cleanRel(url)
	}
	return "", fmt.Errorf("source object needs path or relative url")
}

func stringField(m map[string]any, key string) (string, bool) {
	v, ok := m[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	if !ok || s == "" {
		return "", false
	}
	return s, true
}

func isRemoteURL(s string) bool {
	lower := strings.ToLower(s)
	return strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(lower, "git@") ||
		strings.HasPrefix(lower, "ssh://") ||
		strings.HasPrefix(lower, "git://")
}

func cleanRel(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", fmt.Errorf("empty path")
	}
	// filepath.Clean keeps "./" as "." for current dir
	cleaned := filepath.Clean(filepath.FromSlash(p))
	if cleaned == "." {
		return ".", nil
	}
	if filepath.IsAbs(cleaned) {
		return "", fmt.Errorf("absolute plugin path not allowed: %s", p)
	}
	return cleaned, nil
}

// safeJoin joins root and rel and ensures the result stays under root.
func safeJoin(root, rel string) (string, error) {
	if rel == "." {
		return root, nil
	}
	joined := filepath.Join(root, rel)
	// resolve symlinks only for root to compare prefixes; joined may not exist yet
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	absJoined, err := filepath.Abs(joined)
	if err != nil {
		return "", err
	}
	// ensure absJoined is absRoot or a child
	sep := string(os.PathSeparator)
	if absJoined != absRoot && !strings.HasPrefix(absJoined, absRoot+sep) {
		return "", fmt.Errorf("plugin path escapes marketplace root: %s", rel)
	}
	return absJoined, nil
}

func listSkillDirs(skillsDir string) (map[string]string, error) {
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return nil, fmt.Errorf("reading skills dir: %w", err)
	}
	out := make(map[string]string)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		out[name] = filepath.Join(skillsDir, name)
	}
	return out, nil
}

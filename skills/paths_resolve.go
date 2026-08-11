package skills

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PathList unmarshals a Claude component path field that may be a string or []string.
type PathList []string

func (p *PathList) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		*p = nil
		return nil
	}
	var one string
	if err := json.Unmarshal(data, &one); err == nil {
		if one == "" {
			*p = nil
			return nil
		}
		*p = PathList{one}
		return nil
	}
	var many []string
	if err := json.Unmarshal(data, &many); err != nil {
		return fmt.Errorf("skills path must be string or array of strings: %w", err)
	}
	*p = PathList(many)
	return nil
}

// skillResolutionOptions controls Claude skill path resolution for a plugin root.
type skillResolutionOptions struct {
	// Paths from the skills field (plugin.json or marketplace entry).
	SkillsField PathList
	// When true (marketplace entry whose source is marketplace root), a non-empty
	// skills field replaces the default skills/ scan instead of adding to it.
	// Listing ./skills/ or the plugin root still expands to a full container scan.
	MarketplaceRoot bool
	// Fallback skill name when a root-level SKILL.md has no frontmatter name
	// (typically the plugin name).
	DefaultName string
}

// resolveSkills applies Claude Code skill discovery rules under pluginDir.
//
// Rules (code.claude.com plugins / marketplaces docs):
//   - Default: scan pluginDir/skills/ for <name>/SKILL.md children
//   - skills field (string|array): paths relative to plugin root (./… or ".")
//   - Each path may be a skill dir (contains SKILL.md) or a container of skill dirs
//   - skills paths ADD to default skills/ scan, except marketplace-root entries
//     where listed paths are the complete set (unless none exist → default scan)
//   - No skills/ and no skills field + root SKILL.md → single-skill plugin
func resolveSkills(pluginDir string, opts skillResolutionOptions) (map[string]string, error) {
	pluginDir, err := filepath.Abs(pluginDir)
	if err != nil {
		return nil, err
	}

	fromField, anyPathExisted, err := expandSkillsField(pluginDir, opts.SkillsField, opts.DefaultName)
	if err != nil {
		return nil, err
	}

	var out map[string]string
	switch {
	case len(opts.SkillsField) > 0 && opts.MarketplaceRoot:
		// Marketplace-root: listed paths are exclusive when at least one exists.
		if anyPathExisted {
			out = fromField
		} else {
			// None of the listed paths exist → fall back to default scan.
			out, err = scanSkillsContainer(filepath.Join(pluginDir, "skills"))
			if err != nil {
				return nil, err
			}
		}
	case len(opts.SkillsField) > 0:
		// Additive: default skills/ + listed paths.
		out, err = scanSkillsContainer(filepath.Join(pluginDir, "skills"))
		if err != nil {
			return nil, err
		}
		out = mergeSkills(out, fromField)
	default:
		out, err = scanSkillsContainer(filepath.Join(pluginDir, "skills"))
		if err != nil {
			return nil, err
		}
	}

	// Single-skill plugin: root SKILL.md when nothing else resolved.
	if len(out) == 0 {
		if isSkillDir(pluginDir) {
			name := skillNameForDir(pluginDir, opts.DefaultName)
			out = map[string]string{name: pluginDir}
		}
	}
	return out, nil
}

// expandSkillsField resolves each skills-field path under pluginDir.
// anyPathExisted is true if at least one path existed on disk (even if empty of skills).
func expandSkillsField(pluginDir string, paths PathList, defaultName string) (map[string]string, bool, error) {
	out := make(map[string]string)
	any := false
	for i, raw := range paths {
		rel, err := cleanSkillPath(raw)
		if err != nil {
			return nil, false, fmt.Errorf("skills[%d] %q: %w", i, raw, err)
		}
		abs, err := safeJoin(pluginDir, rel)
		if err != nil {
			return nil, false, fmt.Errorf("skills[%d] %q: %w", i, raw, err)
		}
		st, err := os.Stat(abs)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, false, fmt.Errorf("skills[%d] %q: %w", i, raw, err)
		}
		any = true
		if !st.IsDir() {
			return nil, false, fmt.Errorf("skills[%d] %q: not a directory", i, raw)
		}
		part, err := expandSkillPath(abs, defaultName)
		if err != nil {
			return nil, false, fmt.Errorf("skills[%d] %q: %w", i, raw, err)
		}
		out = mergeSkills(out, part)
	}
	return out, any, nil
}

// expandSkillPath turns one skills-field path into skill name → dir.
// If the directory itself has SKILL.md it is one skill; otherwise scan children.
func expandSkillPath(dir, defaultName string) (map[string]string, error) {
	if isSkillDir(dir) {
		name := skillNameForDir(dir, defaultName)
		return map[string]string{name: dir}, nil
	}
	return scanSkillsContainer(dir)
}

// scanSkillsContainer lists immediate child directories that contain SKILL.md.
// Missing container dir yields an empty map (not an error).
func scanSkillsContainer(dir string) (map[string]string, error) {
	st, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	if !st.IsDir() {
		return map[string]string{}, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading skills dir: %w", err)
	}
	out := make(map[string]string)
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		child := filepath.Join(dir, e.Name())
		if !isSkillDir(child) {
			continue
		}
		name := skillNameForDir(child, e.Name())
		out[name] = child
	}
	return out, nil
}

func isSkillDir(dir string) bool {
	st, err := os.Stat(filepath.Join(dir, "SKILL.md"))
	return err == nil && !st.IsDir()
}

// skillNameForDir prefers YAML frontmatter `name:` in SKILL.md, else basename,
// else defaultName (for plugin-root single skills).
func skillNameForDir(dir, defaultName string) string {
	if n := frontmatterName(filepath.Join(dir, "SKILL.md")); n != "" {
		return n
	}
	base := filepath.Base(dir)
	if base != "" && base != "." && base != string(os.PathSeparator) {
		return base
	}
	if defaultName != "" {
		return defaultName
	}
	return "skill"
}

func frontmatterName(skillMD string) string {
	f, err := os.Open(skillMD)
	if err != nil {
		return ""
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	// Cap line size for safety
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	if !sc.Scan() {
		return ""
	}
	if strings.TrimSpace(sc.Text()) != "---" {
		return ""
	}
	for sc.Scan() {
		line := sc.Text()
		if strings.TrimSpace(line) == "---" {
			break
		}
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		if strings.TrimSpace(key) != "name" {
			continue
		}
		val = strings.TrimSpace(val)
		val = strings.Trim(val, `"'`)
		return val
	}
	return ""
}

func mergeSkills(dst, src map[string]string) map[string]string {
	if dst == nil {
		dst = make(map[string]string, len(src))
	}
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// cleanSkillPath is like cleanRel but also accepts "." for plugin root.
func cleanSkillPath(p string) (string, error) {
	return cleanRel(p)
}

// componentFieldsDeclared reports whether a plugin.json declares component paths
// (used for strict:false conflict detection).
func componentFieldsDeclared(pm *PluginManifest) bool {
	if pm == nil {
		return false
	}
	if len(pm.Skills) > 0 || len(pm.Commands) > 0 || len(pm.Agents) > 0 || len(pm.OutputStyles) > 0 {
		return true
	}
	// hooks / mcpServers may be object or path string — any non-null JSON counts.
	if len(pm.Hooks) > 0 && string(pm.Hooks) != "null" {
		return true
	}
	if len(pm.MCPServers) > 0 && string(pm.MCPServers) != "null" {
		return true
	}
	return false
}

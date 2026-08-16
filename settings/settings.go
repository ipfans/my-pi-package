// Package settings reads and writes Pi agent settings.json.
package settings

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/ipfans/my-pi-package/catalog"
)

// Paths resolves global vs local settings locations.
type Paths struct {
	Local bool
	Cwd   string
	Home  string
	// AgentDir overrides PI_CODING_AGENT_DIR resolution when non-empty (tests).
	AgentDir string
}

// AgentConfigDir returns the Pi agent config root.
func (p Paths) AgentConfigDir() string {
	if p.AgentDir != "" {
		return p.AgentDir
	}
	configured := os.Getenv("PI_CODING_AGENT_DIR")
	home := p.Home
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	if configured == "" {
		return filepath.Join(home, ".pi", "agent")
	}
	if configured == "~" {
		return home
	}
	if strings.HasPrefix(configured, "~/") {
		return filepath.Join(home, configured[2:])
	}
	return configured
}

// SettingsPath is the settings.json path for the current scope.
func (p Paths) SettingsPath() string {
	if p.Local {
		cwd := p.Cwd
		if cwd == "" {
			cwd, _ = os.Getwd()
		}
		return filepath.Join(cwd, ".pi", "settings.json")
	}
	return filepath.Join(p.AgentConfigDir(), "settings.json")
}

// InstallRoot is the compound / agent root for the scope.
func (p Paths) InstallRoot() string {
	if p.Local {
		cwd := p.Cwd
		if cwd == "" {
			cwd, _ = os.Getwd()
		}
		return filepath.Join(cwd, ".pi")
	}
	return p.AgentConfigDir()
}

// AuthPath is auth.json under the agent config dir (always global agent dir).
func (p Paths) AuthPath() string {
	return filepath.Join(p.AgentConfigDir(), "auth.json")
}

// Document is the loosely-typed settings JSON object.
type Document map[string]any

// Read loads settings. Missing file yields empty document, exists=false.
func Read(path string) (doc Document, exists bool, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Document{}, false, nil
		}
		return nil, false, err
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, true, fmt.Errorf("invalid JSON in %s: %w", path, err)
	}
	if doc == nil {
		doc = Document{}
	}
	return doc, true, nil
}

// Write mutates settings via fn; writes only if fn returns true.
// Existing files are copied to a timestamped .bak first.
func Write(path string, mutate func(Document) bool) (backup string, changed bool, err error) {
	doc, exists, err := Read(path)
	if err != nil {
		return "", false, err
	}
	if doc == nil {
		doc = Document{}
	}
	if !mutate(doc) {
		return "", false, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", false, err
	}
	if exists {
		backup = path + ".my-pi-package." + time.Now().UTC().Format("20060102T150405Z") + ".bak"
		if err := copyFile(path, backup); err != nil {
			return "", false, fmt.Errorf("backing up settings: %w", err)
		}
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return backup, false, err
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return backup, false, err
	}
	return backup, true, nil
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

// PackageEntrySource extracts the source string from a packages[] entry.
func PackageEntrySource(entry any) string {
	switch v := entry.(type) {
	case string:
		return v
	case map[string]any:
		if s, ok := v["source"].(string); ok {
			return s
		}
	}
	return ""
}

// InstalledSources returns the set of package sources in settings.
func InstalledSources(doc Document) map[string]struct{} {
	out := make(map[string]struct{})
	raw, _ := doc["packages"].([]any)
	for _, entry := range raw {
		if s := PackageEntrySource(entry); s != "" {
			out[s] = struct{}{}
		}
	}
	return out
}

var subagentBuiltinModels = []string{
	"context-builder", "planner", "researcher", "reviewer", "scout", "worker",
}

// WriteSubagentOverrides blanks hardcoded models on builtin sub-agents.
func WriteSubagentOverrides(path string) (backup string, err error) {
	backup, _, err = Write(path, func(doc Document) bool {
		sub, _ := doc["subagents"].(map[string]any)
		if sub == nil {
			sub = map[string]any{}
		}
		overrides, _ := sub["agentOverrides"].(map[string]any)
		if overrides == nil {
			overrides = map[string]any{}
		}
		for _, name := range subagentBuiltinModels {
			overrides[name] = map[string]any{"model": ""}
		}
		sub["agentOverrides"] = overrides
		doc["subagents"] = sub
		return true
	})
	return backup, err
}

// NormalizeLoadOrder moves load_first sources to the front of packages[].
func NormalizeLoadOrder(path string, cat *catalog.Catalog) (backup string, changed bool, err error) {
	priority := cat.LoadFirstSources()
	if len(priority) == 0 {
		return "", false, nil
	}
	prioIndex := make(map[string]int, len(priority))
	for i, s := range priority {
		prioIndex[s] = i
	}
	return Write(path, func(doc Document) bool {
		raw, ok := doc["packages"].([]any)
		if !ok || len(raw) == 0 {
			return false
		}
		type scored struct {
			entry any
			prio  int
			orig  int
		}
		var first, rest []scored
		for i, entry := range raw {
			src := PackageEntrySource(entry)
			if idx, ok := prioIndex[src]; ok {
				first = append(first, scored{entry, idx, i})
			} else {
				rest = append(rest, scored{entry, -1, i})
			}
		}
		if len(first) == 0 {
			return false
		}
		// stable sort by priority then original index
		for i := 0; i < len(first); i++ {
			for j := i + 1; j < len(first); j++ {
				if first[j].prio < first[i].prio || (first[j].prio == first[i].prio && first[j].orig < first[i].orig) {
					first[i], first[j] = first[j], first[i]
				}
			}
		}
		ordered := make([]any, 0, len(raw))
		for _, s := range first {
			ordered = append(ordered, s.entry)
		}
		for _, s := range rest {
			ordered = append(ordered, s.entry)
		}
		if reflect.DeepEqual(ordered, raw) {
			return false
		}
		doc["packages"] = ordered
		return true
	})
}

// LoadOrderOK reports whether extension-settings appears before powerbar when both present.
func LoadOrderOK(doc Document, cat *catalog.Catalog) (checked, ok bool) {
	ext := cat.ByID("extension-settings")
	pwr := cat.ByID("powerbar")
	if ext == nil || pwr == nil {
		return false, false
	}
	raw, _ := doc["packages"].([]any)
	extIdx, pwrIdx := -1, -1
	for i, entry := range raw {
		src := PackageEntrySource(entry)
		if src == ext.Source() {
			extIdx = i
		}
		if src == pwr.Source() {
			pwrIdx = i
		}
	}
	if extIdx < 0 || pwrIdx < 0 {
		return false, false
	}
	return true, extIdx < pwrIdx
}

package install

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ipfans/my-pi-package/catalog"
	"github.com/ipfans/my-pi-package/pi"
	"github.com/ipfans/my-pi-package/settings"
)

const (
	compoundManifestRel = "compound-engineering/install-manifest.json"
	compoundLegacyRel   = ".my-pi-package/compound-engineering.json"
)

type compoundMode string

const (
	compoundNone            compoundMode = "none"
	compoundManifest        compoundMode = "manifest"
	compoundLegacy          compoundMode = "legacy"
	compoundInvalidManifest compoundMode = "invalid-manifest"
	compoundInvalidLegacy   compoundMode = "invalid-legacy"
)

type compoundState struct {
	mode         compoundMode
	manifestPath string
	legacyPath   string
	manifest     map[string]any
	legacy       map[string]any
	err          string
}

func readCompoundState(paths settings.Paths) compoundState {
	root := paths.InstallRoot()
	manifestPath := filepath.Join(root, filepath.FromSlash(compoundManifestRel))
	legacyPath := filepath.Join(root, filepath.FromSlash(compoundLegacyRel))

	st := compoundState{manifestPath: manifestPath, legacyPath: legacyPath}

	if data, err := os.ReadFile(manifestPath); err == nil {
		var m map[string]any
		if err := json.Unmarshal(data, &m); err != nil {
			st.mode = compoundInvalidManifest
			st.err = err.Error()
			return st
		}
		st.mode = compoundManifest
		st.manifest = m
		return st
	} else if !os.IsNotExist(err) {
		st.mode = compoundInvalidManifest
		st.err = err.Error()
		return st
	}

	if data, err := os.ReadFile(legacyPath); err == nil {
		var m map[string]any
		if err := json.Unmarshal(data, &m); err != nil {
			st.mode = compoundInvalidLegacy
			st.err = err.Error()
			return st
		}
		st.mode = compoundLegacy
		st.legacy = m
		return st
	}

	st.mode = compoundNone
	return st
}

func isCompoundInstalled(paths settings.Paths) bool {
	m := readCompoundState(paths).mode
	return m == compoundManifest || m == compoundLegacy
}

func compoundNeedsMigration(paths settings.Paths) bool {
	return readCompoundState(paths).mode == compoundLegacy
}

func installCompound(pkg catalog.Package, paths settings.Paths) (status string, reason string, code int) {
	if !pi.HasCommand("bun") {
		return "skipped", "bun is not on PATH — official Compound Engineering from Every will be skipped.", 0
	}
	root, err := filepath.Abs(paths.InstallRoot())
	if err != nil {
		return "failed", err.Error(), 1
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "failed", err.Error(), 1
	}

	spec := pkg.BunxSpec()
	cleanupArgs := []string{spec, "cleanup", "--target", "pi", "--pi-home", root}
	fmt.Printf("\n→ bunx %s\n", strings.Join(cleanupArgs, " "))
	if code := pi.Run("bunx", cleanupArgs, nil); code != 0 {
		return "failed", "upstream cleanup failed", code
	}

	plugin := pkg.PluginName
	if plugin == "" {
		plugin = "compound-engineering"
	}
	installArgs := []string{spec, "install", plugin, "--to", "pi", "--pi-home", root}
	fmt.Printf("\n→ bunx %s\n", strings.Join(installArgs, " "))
	if code := pi.Run("bunx", installArgs, nil); code != 0 {
		return "failed", "upstream install failed", code
	}

	st := readCompoundState(paths)
	if st.mode != compoundManifest {
		return "failed", fmt.Sprintf("missing %s", compoundManifestRel), 1
	}
	clearCompoundLegacy(paths)
	pruneEmptyManagedDirs(paths)
	return "installed", "", 0
}

func removeCompound(paths settings.Paths) int {
	st := readCompoundState(paths)
	root := paths.InstallRoot()
	switch st.mode {
	case compoundNone:
		return 0
	case compoundInvalidManifest:
		fmt.Fprintf(os.Stderr, "Compound Engineering manifest is invalid: %s\n", st.err)
		return 1
	case compoundInvalidLegacy:
		fmt.Fprintf(os.Stderr, "Compound Engineering legacy state is invalid: %s\n", st.err)
		return 1
	case compoundManifest:
		pathsToRemove := compoundManifestManagedPaths(st.manifest)
		if len(pathsToRemove) == 0 {
			fmt.Fprintf(os.Stderr, "Compound Engineering manifest does not list managed paths: %s\n", st.manifestPath)
			return 1
		}
		pathsToRemove = append(pathsToRemove, compoundManifestRel, "compound-engineering/legacy-backup")
		removeManagedPaths(root, pathsToRemove)
	case compoundLegacy:
		removeManagedPaths(root, legacyCreatedPaths(st.legacy))
		restoreLegacyModified(root, st.legacy)
		removeManagedPaths(root, []string{"compound-engineering/legacy-backup"})
	}
	clearCompoundLegacy(paths)
	pruneEmptyManagedDirs(paths)
	return 0
}

func clearCompoundLegacy(paths settings.Paths) {
	p := filepath.Join(paths.InstallRoot(), filepath.FromSlash(compoundLegacyRel))
	_ = os.Remove(p)
	parent := filepath.Dir(p)
	if entries, err := os.ReadDir(parent); err == nil && len(entries) == 0 {
		_ = os.Remove(parent)
	}
}

func pruneEmptyManagedDirs(paths settings.Paths) {
	root := paths.InstallRoot()
	for _, dir := range []string{"prompts", "skills", "extensions", "agents", "compound-engineering", ".my-pi-package"} {
		pruneEmpty(filepath.Join(root, dir))
	}
}

func pruneEmpty(path string) {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return
	}
	for _, e := range entries {
		pruneEmpty(filepath.Join(path, e.Name()))
	}
	entries, err = os.ReadDir(path)
	if err == nil && len(entries) == 0 {
		_ = os.Remove(path)
	}
}

func normalizeRel(path string) string {
	return strings.TrimPrefix(strings.ReplaceAll(path, "\\", "/"), "./")
}

func coerceManagedPath(entry any) string {
	switch v := entry.(type) {
	case string:
		return normalizeRel(v)
	case map[string]any:
		for _, key := range []string{"path", "relativePath", "file"} {
			if s, ok := v[key].(string); ok {
				return normalizeRel(s)
			}
		}
	}
	return ""
}

func compoundManifestManagedPaths(manifest map[string]any) []string {
	seen := map[string]struct{}{"AGENTS.md": {}}
	add := func(entry any, baseDir string) {
		path := coerceManagedPath(entry)
		if path != "" {
			if baseDir != "" && !strings.Contains(path, "/") {
				path = normalizeRel(filepath.ToSlash(filepath.Join(baseDir, path)))
			}
			seen[path] = struct{}{}
			return
		}
		if s, ok := entry.(string); ok && baseDir != "" {
			seen[normalizeRel(filepath.ToSlash(filepath.Join(baseDir, s)))] = struct{}{}
		}
	}
	for _, source := range []map[string]any{manifest, asMap(manifest["installManifest"])} {
		if source == nil {
			continue
		}
		for _, key := range []string{"files", "managedFiles", "paths", "createdFiles", "artifacts"} {
			arr, _ := source[key].([]any)
			for _, e := range arr {
				add(e, "")
			}
		}
		for _, e := range asSlice(source["skills"]) {
			add(e, "skills")
		}
		for _, e := range asSlice(source["prompts"]) {
			add(e, "prompts")
		}
		for _, e := range asSlice(source["extensions"]) {
			add(e, "extensions")
		}
		for _, e := range asSlice(source["agents"]) {
			add(e, "agents")
		}
	}
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	// longest first for safe delete
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if len(out[j]) > len(out[i]) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

func legacyCreatedPaths(state map[string]any) []string {
	var out []string
	for _, e := range asSlice(state["createdFiles"]) {
		if s, ok := e.(string); ok && s != "" {
			out = append(out, normalizeRel(s))
		}
	}
	return out
}

func restoreLegacyModified(root string, state map[string]any) {
	for _, e := range asSlice(state["modifiedFiles"]) {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		path, _ := m["path"].(string)
		b64, _ := m["previousContentBase64"].(string)
		if path == "" || b64 == "" {
			continue
		}
		// restore best-effort; ignore decode errors
		// use encoding/base64
		data, err := decodeB64(b64)
		if err != nil {
			continue
		}
		target := filepath.Join(root, filepath.FromSlash(normalizeRel(path)))
		_ = os.MkdirAll(filepath.Dir(target), 0o755)
		_ = os.WriteFile(target, data, 0o644)
	}
}

func removeManagedPaths(root string, rels []string) {
	// longest first
	for i := 0; i < len(rels); i++ {
		for j := i + 1; j < len(rels); j++ {
			if len(rels[j]) > len(rels[i]) {
				rels[i], rels[j] = rels[j], rels[i]
			}
		}
	}
	seen := map[string]struct{}{}
	for _, rel := range rels {
		rel = normalizeRel(rel)
		if rel == "" {
			continue
		}
		if _, ok := seen[rel]; ok {
			continue
		}
		seen[rel] = struct{}{}
		_ = os.RemoveAll(filepath.Join(root, filepath.FromSlash(rel)))
	}
}

func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func asSlice(v any) []any {
	s, _ := v.([]any)
	return s
}

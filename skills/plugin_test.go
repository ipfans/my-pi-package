package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func writePluginJSON(t *testing.T, root, body string) {
	t.Helper()
	dir := filepath.Join(root, ".claude-plugin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func makeSkillTree(t *testing.T, root string, rel string, files map[string]string) {
	t.Helper()
	dir := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDiscoverPluginJSON(t *testing.T) {
	root := t.TempDir()
	makeSkillTree(t, root, "skills/engineering/ask-matt", map[string]string{
		"SKILL.md":            "# ask-matt\n",
		"references/notes.md": "nested\n",
	})
	makeSkillTree(t, root, "skills/productivity/grill-me", map[string]string{
		"SKILL.md": "# grill\n",
	})
	writePluginJSON(t, root, `{
  "name": "mattpocock-skills",
  "description": "demo",
  "skills": [
    "./skills/engineering/ask-matt",
    "./skills/productivity/grill-me"
  ]
}`)

	disc, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if disc.Mode != ModePlugin {
		t.Fatalf("mode = %q", disc.Mode)
	}
	if disc.ManifestRel != pluginManifestRel {
		t.Fatalf("rel = %q", disc.ManifestRel)
	}
	if disc.Name != "mattpocock-skills" {
		t.Fatalf("name = %q", disc.Name)
	}
	if len(disc.Plugins) != 1 {
		t.Fatalf("plugins = %d", len(disc.Plugins))
	}
	p := disc.Plugins[0]
	if len(p.Skills) != 2 {
		t.Fatalf("skills = %d", len(p.Skills))
	}
	if _, ok := p.Skills["ask-matt"]; !ok {
		t.Fatal("missing ask-matt")
	}
	if _, ok := p.Skills["grill-me"]; !ok {
		t.Fatal("missing grill-me")
	}
	// paths must point at skill dirs
	if filepath.Base(p.Skills["ask-matt"]) != "ask-matt" {
		t.Fatalf("path = %q", p.Skills["ask-matt"])
	}
}

func TestDiscoverPluginJSONTakesPriorityOverMarketplace(t *testing.T) {
	root := t.TempDir()
	makeSkillTree(t, root, "skills/from-plugin/alpha", map[string]string{"SKILL.md": "p\n"})
	writePluginJSON(t, root, `{
  "name": "from-plugin",
  "skills": ["./skills/from-plugin/alpha"]
}`)

	// marketplace that would yield different skills if used
	pluginDir := filepath.Join(root, "plugins", "demo")
	skill := filepath.Join(pluginDir, "skills", "market-skill")
	if err := os.MkdirAll(skill, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("m\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := `{
  "name": "test-market",
  "plugins": [{"name": "demo", "source": "./plugins/demo"}]
}`
	if err := os.WriteFile(filepath.Join(root, ".claude-plugin", "marketplace.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	disc, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if disc.Mode != ModePlugin {
		t.Fatalf("want plugin mode, got %q", disc.Mode)
	}
	if _, ok := disc.Plugins[0].Skills["alpha"]; !ok {
		t.Fatalf("expected plugin skill, got %+v", disc.Plugins[0].Skills)
	}
	if _, ok := disc.Plugins[0].Skills["market-skill"]; ok {
		t.Fatal("must not load marketplace skills when plugin.json exists")
	}
}

func TestDiscoverFallsBackToMarketplace(t *testing.T) {
	root := t.TempDir()
	pluginDir := filepath.Join(root, "plugins", "demo")
	skill := filepath.Join(pluginDir, "skills", "s1")
	if err := os.MkdirAll(skill, 0o755); err != nil {
		t.Fatal(err)
	}
	mPath := filepath.Join(root, ".claude-plugin")
	if err := os.MkdirAll(mPath, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{
  "name": "test-market",
  "plugins": [{"name": "demo", "source": "./plugins/demo"}]
}`
	if err := os.WriteFile(filepath.Join(mPath, "marketplace.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	disc, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if disc.Mode != ModeMarketplace {
		t.Fatalf("mode = %q", disc.Mode)
	}
	if len(disc.Plugins) != 1 || disc.Plugins[0].Name != "demo" {
		t.Fatalf("plugins = %+v", disc.Plugins)
	}
}

func TestPluginManifestErrors(t *testing.T) {
	t.Run("escape", func(t *testing.T) {
		root := t.TempDir()
		writePluginJSON(t, root, `{
  "name": "x",
  "skills": ["../outside"]
}`)
		_, err := Discover(root)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("missing dir", func(t *testing.T) {
		root := t.TempDir()
		writePluginJSON(t, root, `{
  "name": "x",
  "skills": ["./skills/nope"]
}`)
		_, err := Discover(root)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("duplicate basename", func(t *testing.T) {
		root := t.TempDir()
		makeSkillTree(t, root, "skills/a/same", map[string]string{"SKILL.md": "1\n"})
		makeSkillTree(t, root, "skills/b/same", map[string]string{"SKILL.md": "2\n"})
		writePluginJSON(t, root, `{
  "name": "x",
  "skills": ["./skills/a/same", "./skills/b/same"]
}`)
		_, err := Discover(root)
		if err == nil {
			t.Fatal("expected duplicate error")
		}
	})

	t.Run("empty skills", func(t *testing.T) {
		root := t.TempDir()
		writePluginJSON(t, root, `{"name":"x","skills":[]}`)
		_, err := Discover(root)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("not a directory", func(t *testing.T) {
		root := t.TempDir()
		path := filepath.Join(root, "skills", "file")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		writePluginJSON(t, root, `{
  "name": "x",
  "skills": ["./skills/file"]
}`)
		_, err := Discover(root)
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestFilterAndSelectSkills(t *testing.T) {
	p := PluginSkills{
		Name: "demo",
		Skills: map[string]string{
			"alpha": "/tmp/alpha",
			"beta":  "/tmp/beta",
			"gamma": "/tmp/gamma",
		},
	}

	filtered, err := FilterSkills(p, []string{"beta", "alpha", "beta"})
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered.Skills) != 2 {
		t.Fatalf("len = %d", len(filtered.Skills))
	}
	if _, ok := filtered.Skills["gamma"]; ok {
		t.Fatal("gamma should be filtered out")
	}

	names, err := SelectSkillsByOnly(p, []string{"gamma", "alpha"})
	if err != nil {
		t.Fatal(err)
	}
	// SkillNames is sorted
	if len(names) != 2 || names[0] != "alpha" || names[1] != "gamma" {
		t.Fatalf("names = %v", names)
	}

	if _, err := FilterSkills(p, []string{"missing"}); err == nil {
		t.Fatal("expected missing skill error")
	}
	if _, err := SelectSkillsByOnly(p, nil); err == nil {
		t.Fatal("expected empty only error")
	}
}

func TestInstallFromPluginJSONPaths(t *testing.T) {
	root := t.TempDir()
	makeSkillTree(t, root, "skills/engineering/ask-matt", map[string]string{
		"SKILL.md":            "# ask\n",
		"references/notes.md": "nested body\n",
	})
	makeSkillTree(t, root, "skills/productivity/grill-me", map[string]string{
		"SKILL.md": "# grill\n",
	})
	writePluginJSON(t, root, `{
  "name": "demo-plugin",
  "skills": [
    "./skills/engineering/ask-matt",
    "./skills/productivity/grill-me"
  ]
}`)

	disc, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	filtered, err := FilterSkills(disc.Plugins[0], []string{"ask-matt"})
	if err != nil {
		t.Fatal(err)
	}

	target := filepath.Join(t.TempDir(), "skills")
	res, err := Install(target, []PluginSkills{filtered}, []string{filtered.Name})
	if err != nil {
		t.Fatal(err)
	}
	if res.Installed != 1 || res.Failed != 0 {
		t.Fatalf("res = %+v", res)
	}

	// whole folder including nested file
	body, err := os.ReadFile(filepath.Join(target, "ask-matt", "references", "notes.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "nested body\n" {
		t.Fatalf("body = %q", body)
	}
	if _, err := os.Stat(filepath.Join(target, "grill-me")); !os.IsNotExist(err) {
		t.Fatal("grill-me should not be installed")
	}
}

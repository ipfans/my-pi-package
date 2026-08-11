package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func writeMarketplace(t *testing.T, root, body string) {
	t.Helper()
	dir := filepath.Join(root, ".claude-plugin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "marketplace.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeNestedPluginJSON(t *testing.T, pluginDir, body string) {
	t.Helper()
	dir := filepath.Join(pluginDir, ".claude-plugin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestBetterAuthMarketplaceLayout(t *testing.T) {
	// Mirrors https://github.com/better-auth/skills:
	// marketplace source ./ with strict:false and explicit skill paths;
	// skills live as peer dirs with SKILL.md, not under skills/.
	root := t.TempDir()
	makeSkillTree(t, root, "better-auth/create-auth", map[string]string{
		"SKILL.md": "---\nname: create-auth\n---\n# create\n",
	})
	makeSkillTree(t, root, "better-auth/best-practices", map[string]string{
		"SKILL.md": "---\nname: better-auth-best-practices\n---\n# best\n",
	})
	makeSkillTree(t, root, "better-auth/twoFactor", map[string]string{
		"SKILL.md": "---\nname: two-factor\n---\n# 2fa\n",
	})
	makeSkillTree(t, root, "security", map[string]string{
		"SKILL.md": "---\nname: better-auth-security-best-practices\n---\n# sec\n",
	})
	writeNestedPluginJSON(t, filepath.Join(root, "better-auth"), `{
  "name": "better-auth",
  "description": "nested plugin without component fields"
}`)
	writeMarketplace(t, root, `{
  "name": "better-auth-agent-skills",
  "plugins": [{
    "name": "auth-skills",
    "description": "Create and update auth layer",
    "source": "./",
    "strict": false,
    "skills": [
      "./better-auth/create-auth",
      "./better-auth/best-practices"
    ]
  }]
}`)

	disc, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if disc.Mode != ModeMarketplace {
		t.Fatalf("mode = %q", disc.Mode)
	}
	if len(disc.Plugins) != 1 {
		t.Fatalf("plugins = %d, want 1", len(disc.Plugins))
	}
	p := disc.Plugins[0]
	if p.Name != "auth-skills" {
		t.Fatalf("name = %q", p.Name)
	}
	// frontmatter names win
	if len(p.Skills) != 2 {
		t.Fatalf("skills = %v (want 2 exclusive paths)", p.SkillNames())
	}
	if _, ok := p.Skills["create-auth"]; !ok {
		t.Fatalf("missing create-auth: %v", p.SkillNames())
	}
	if _, ok := p.Skills["better-auth-best-practices"]; !ok {
		t.Fatalf("missing better-auth-best-practices: %v", p.SkillNames())
	}
	// Not in marketplace skills list
	for _, banned := range []string{"two-factor", "twoFactor", "better-auth-security-best-practices", "security"} {
		if _, ok := p.Skills[banned]; ok {
			t.Fatalf("should not include %q", banned)
		}
	}
}

func TestMarketplaceRootSkillsExclusive(t *testing.T) {
	root := t.TempDir()
	makeSkillTree(t, root, "skills/keep-me", map[string]string{"SKILL.md": "# k\n"})
	makeSkillTree(t, root, "skills/skip-me", map[string]string{"SKILL.md": "# s\n"})
	writeMarketplace(t, root, `{
  "name": "m",
  "plugins": [{
    "name": "p",
    "source": "./",
    "skills": ["./skills/keep-me"]
  }]
}`)
	plugins, err := DiscoverPluginsWithSkills(root, mustLoadMarket(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if len(plugins) != 1 || len(plugins[0].Skills) != 1 {
		t.Fatalf("got %+v", plugins)
	}
	if _, ok := plugins[0].Skills["keep-me"]; !ok {
		t.Fatalf("skills = %v", plugins[0].SkillNames())
	}
}

func TestMarketplaceSkillsAdditiveForNonRootSource(t *testing.T) {
	root := t.TempDir()
	plugin := filepath.Join(root, "plugins", "demo")
	makeSkillTree(t, plugin, "skills/default-skill", map[string]string{"SKILL.md": "# d\n"})
	makeSkillTree(t, plugin, "extra/extra-skill", map[string]string{"SKILL.md": "# e\n"})
	writeMarketplace(t, root, `{
  "name": "m",
  "plugins": [{
    "name": "demo",
    "source": "./plugins/demo",
    "skills": ["./extra/extra-skill"]
  }]
}`)
	plugins, err := DiscoverPluginsWithSkills(root, mustLoadMarket(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if len(plugins) != 1 {
		t.Fatalf("plugins = %d", len(plugins))
	}
	// default skills/ + listed path
	if len(plugins[0].Skills) != 2 {
		t.Fatalf("skills = %v", plugins[0].SkillNames())
	}
}

func TestMarketplaceSkillsFieldAsString(t *testing.T) {
	root := t.TempDir()
	plugin := filepath.Join(root, "plugins", "demo")
	makeSkillTree(t, plugin, "custom/a", map[string]string{"SKILL.md": "# a\n"})
	makeSkillTree(t, plugin, "custom/b", map[string]string{"SKILL.md": "# b\n"})
	// string form = container path; non-root → additive with empty skills/
	writeMarketplace(t, root, `{
  "name": "m",
  "plugins": [{
    "name": "demo",
    "source": "./plugins/demo",
    "skills": "./custom/"
  }]
}`)
	plugins, err := DiscoverPluginsWithSkills(root, mustLoadMarket(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if len(plugins[0].Skills) != 2 {
		t.Fatalf("skills = %v", plugins[0].SkillNames())
	}
}

func TestMarketplaceMissingSkillsFieldFallsBackToDefaultScan(t *testing.T) {
	root := t.TempDir()
	// listed paths missing → default skills/ scan
	makeSkillTree(t, root, "skills/from-default", map[string]string{"SKILL.md": "# d\n"})
	writeMarketplace(t, root, `{
  "name": "m",
  "plugins": [{
    "name": "p",
    "source": "./",
    "skills": ["./does-not-exist/skill"]
  }]
}`)
	plugins, err := DiscoverPluginsWithSkills(root, mustLoadMarket(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if len(plugins) != 1 || len(plugins[0].Skills) != 1 {
		t.Fatalf("got %+v", plugins)
	}
	if _, ok := plugins[0].Skills["from-default"]; !ok {
		t.Fatalf("skills = %v", plugins[0].SkillNames())
	}
}

func TestStrictFalseConflictsWithPluginComponents(t *testing.T) {
	root := t.TempDir()
	plugin := filepath.Join(root, "plugins", "demo")
	makeSkillTree(t, plugin, "skills/a", map[string]string{"SKILL.md": "# a\n"})
	writeNestedPluginJSON(t, plugin, `{
  "name": "demo",
  "skills": ["./skills/a"]
}`)
	strictFalse := false
	_ = strictFalse
	writeMarketplace(t, root, `{
  "name": "m",
  "plugins": [{
    "name": "demo",
    "source": "./plugins/demo",
    "strict": false,
    "skills": ["./skills/a"]
  }]
}`)
	_, err := DiscoverPluginsWithSkills(root, mustLoadMarket(t, root))
	if err == nil {
		t.Fatal("expected strict:false conflict error")
	}
}

func TestStrictTrueMergesPluginAndMarketplace(t *testing.T) {
	root := t.TempDir()
	plugin := filepath.Join(root, "plugins", "demo")
	makeSkillTree(t, plugin, "skills/from-default", map[string]string{"SKILL.md": "# d\n"})
	makeSkillTree(t, plugin, "extra/from-market", map[string]string{"SKILL.md": "# m\n"})
	writeNestedPluginJSON(t, plugin, `{"name":"demo"}`)
	writeMarketplace(t, root, `{
  "name": "m",
  "plugins": [{
    "name": "demo",
    "source": "./plugins/demo",
    "skills": ["./extra/from-market"]
  }]
}`)
	plugins, err := DiscoverPluginsWithSkills(root, mustLoadMarket(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if len(plugins[0].Skills) != 2 {
		t.Fatalf("skills = %v", plugins[0].SkillNames())
	}
}

func TestRootLevelSKILLMDSingleSkillPlugin(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte("---\nname: root-skill\n---\n# hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writePluginJSON(t, root, `{"name":"solo"}`)

	disc, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if disc.Mode != ModePlugin {
		t.Fatalf("mode = %q", disc.Mode)
	}
	if len(disc.Plugins[0].Skills) != 1 {
		t.Fatalf("skills = %v", disc.Plugins[0].SkillNames())
	}
	if _, ok := disc.Plugins[0].Skills["root-skill"]; !ok {
		t.Fatalf("want frontmatter name, got %v", disc.Plugins[0].SkillNames())
	}
}

func TestDefaultSkillsDirWithoutManifest(t *testing.T) {
	root := t.TempDir()
	makeSkillTree(t, root, "skills/alpha", map[string]string{"SKILL.md": "# a\n"})
	makeSkillTree(t, root, "skills/beta", map[string]string{"SKILL.md": "# b\n"})

	disc, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if disc.Mode != ModePlugin {
		t.Fatalf("mode = %q", disc.Mode)
	}
	if len(disc.Plugins[0].Skills) != 2 {
		t.Fatalf("skills = %v", disc.Plugins[0].SkillNames())
	}
}

func TestMarketplaceTakesPriorityOverRootPluginJSON(t *testing.T) {
	root := t.TempDir()
	makeSkillTree(t, root, "skills/from-plugin/alpha", map[string]string{"SKILL.md": "p\n"})
	writePluginJSON(t, root, `{
  "name": "from-plugin",
  "skills": ["./skills/from-plugin/alpha"]
}`)
	pluginDir := filepath.Join(root, "plugins", "demo")
	skill := filepath.Join(pluginDir, "skills", "market-skill")
	if err := os.MkdirAll(skill, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("m\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Append marketplace alongside existing plugin.json dir
	if err := os.WriteFile(filepath.Join(root, ".claude-plugin", "marketplace.json"), []byte(`{
  "name": "test-market",
  "plugins": [{"name": "demo", "source": "./plugins/demo"}]
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	disc, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if disc.Mode != ModeMarketplace {
		t.Fatalf("want marketplace mode, got %q", disc.Mode)
	}
	if _, ok := disc.Plugins[0].Skills["market-skill"]; !ok {
		t.Fatalf("expected marketplace skill, got %+v", disc.Plugins[0].Skills)
	}
}

func TestScanRequiresSKILLMD(t *testing.T) {
	root := t.TempDir()
	// Directory without SKILL.md must be ignored
	if err := os.MkdirAll(filepath.Join(root, "skills", "empty-dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	makeSkillTree(t, root, "skills/real", map[string]string{"SKILL.md": "# r\n"})
	writeMarketplace(t, root, `{
  "name": "m",
  "plugins": [{"name": "p", "source": "./"}]
}`)
	plugins, err := DiscoverPluginsWithSkills(root, mustLoadMarket(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if len(plugins) != 1 || len(plugins[0].Skills) != 1 {
		t.Fatalf("got %+v", plugins)
	}
	if _, ok := plugins[0].Skills["real"]; !ok {
		t.Fatalf("skills = %v", plugins[0].SkillNames())
	}
}

func mustLoadMarket(t *testing.T, root string) *Marketplace {
	t.Helper()
	m, _, err := LoadMarketplace(root)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

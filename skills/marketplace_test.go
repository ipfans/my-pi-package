package skills

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestResolvePluginSource(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{name: "string path", raw: `"./plugins/dev-flow"`, want: "plugins/dev-flow"},
		{name: "string root", raw: `"./"`, want: "."},
		{name: "object local path", raw: `{"source":"local","path":"./plugins/mattpocock"}`, want: "plugins/mattpocock"},
		{name: "object url relative", raw: `{"source":"url","url":"./"}`, want: "."},
		{name: "object path only", raw: `{"path":"./plugins/x"}`, want: "plugins/x"},
		{name: "remote url rejected", raw: `{"source":"url","url":"https://example.com/p.git"}`, wantErr: true},
		{name: "absolute rejected", raw: `"/etc/passwd"`, wantErr: true},
		{name: "empty", raw: `""`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolvePluginSource(json.RawMessage(tt.raw))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			// filepath.FromSlash may differ on Windows; compare cleaned
			if filepath.ToSlash(got) != filepath.ToSlash(tt.want) {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSafeJoinEscape(t *testing.T) {
	root := t.TempDir()
	_, err := safeJoin(root, filepath.Join("..", "outside"))
	if err == nil {
		t.Fatal("expected escape error")
	}
}

func TestLoadAndDiscover(t *testing.T) {
	root := t.TempDir()
	// marketplace
	pluginDir := filepath.Join(root, "plugins", "demo")
	skillA := filepath.Join(pluginDir, "skills", "skill-a")
	skillB := filepath.Join(pluginDir, "skills", "skill-b")
	if err := os.MkdirAll(skillA, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(skillB, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillA, "SKILL.md"), []byte("# a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillB, "SKILL.md"), []byte("# b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// plugin without skills
	noSkills := filepath.Join(root, "plugins", "empty")
	if err := os.MkdirAll(noSkills, 0o755); err != nil {
		t.Fatal(err)
	}

	manifest := `{
  "name": "test-market",
  "plugins": [
    {"name": "demo", "description": "Demo plugin", "source": "./plugins/demo"},
    {"name": "empty", "description": "No skills", "source": "./plugins/empty"},
    {"name": "object-src", "source": {"source": "local", "path": "./plugins/demo"}}
  ]
}`
	mPath := filepath.Join(root, ".claude-plugin")
	if err := os.MkdirAll(mPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mPath, "marketplace.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	m, rel, err := LoadMarketplace(root)
	if err != nil {
		t.Fatalf("LoadMarketplace: %v", err)
	}
	if rel != ".claude-plugin/marketplace.json" {
		t.Fatalf("rel = %q", rel)
	}
	if m.Name != "test-market" {
		t.Fatalf("name = %q", m.Name)
	}

	plugins, err := DiscoverPluginsWithSkills(root, m)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	// empty omitted; demo + object-src both have skills
	if len(plugins) != 2 {
		t.Fatalf("got %d plugins, want 2", len(plugins))
	}
	if plugins[0].Name != "demo" {
		t.Fatalf("first = %q", plugins[0].Name)
	}
	if len(plugins[0].Skills) != 2 {
		t.Fatalf("demo skills = %d", len(plugins[0].Skills))
	}
}

func TestLoadAgentsMarketplace(t *testing.T) {
	root := t.TempDir()
	pluginDir := filepath.Join(root, "plugins", "p1")
	skill := filepath.Join(pluginDir, "skills", "s1")
	if err := os.MkdirAll(skill, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("# s1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := `{
  "name": "agents-style",
  "plugins": [
    {"name": "p1", "source": {"source": "local", "path": "./plugins/p1"}}
  ]
}`
	dir := filepath.Join(root, ".agents", "plugins")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "marketplace.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	m, rel, err := LoadMarketplace(root)
	if err != nil {
		t.Fatal(err)
	}
	if rel != ".agents/plugins/marketplace.json" {
		t.Fatalf("rel = %q", rel)
	}
	plugins, err := DiscoverPluginsWithSkills(root, m)
	if err != nil {
		t.Fatal(err)
	}
	if len(plugins) != 1 || plugins[0].Name != "p1" {
		t.Fatalf("plugins = %+v", plugins)
	}
}

func TestCloneURL(t *testing.T) {
	tests := []struct {
		in, want string
		wantErr  bool
	}{
		{"ipfans/dev-plugins", "https://github.com/ipfans/dev-plugins.git", false},
		{"https://github.com/a/b.git", "https://github.com/a/b.git", false},
		{"github.com/a/b", "https://github.com/a/b", false},
		{"not a source", "", true},
	}
	for _, tt := range tests {
		got, err := cloneURL(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Fatalf("%q: expected error", tt.in)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%q: %v", tt.in, err)
		}
		if got != tt.want {
			t.Fatalf("%q: got %q want %q", tt.in, got, tt.want)
		}
	}
}

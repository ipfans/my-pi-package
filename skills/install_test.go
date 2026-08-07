package skills

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ipfans/my-pi-package/settings"
)

func TestInstallAndRemove(t *testing.T) {
	srcRoot := t.TempDir()
	target := filepath.Join(t.TempDir(), "skills")

	// two plugins with overlapping skill name "shared"
	makeSkill := func(plugin, skill, body string) string {
		dir := filepath.Join(srcRoot, plugin, "skills", skill)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		p := filepath.Join(dir, "SKILL.md")
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	makeSkill("p1", "alpha", "alpha-v1")
	makeSkill("p1", "shared", "from-p1")
	makeSkill("p2", "shared", "from-p2")
	makeSkill("p2", "beta", "beta")

	plugins := []PluginSkills{
		{
			Name: "p1",
			Skills: map[string]string{
				"alpha":  filepath.Join(srcRoot, "p1", "skills", "alpha"),
				"shared": filepath.Join(srcRoot, "p1", "skills", "shared"),
			},
		},
		{
			Name: "p2",
			Skills: map[string]string{
				"shared": filepath.Join(srcRoot, "p2", "skills", "shared"),
				"beta":   filepath.Join(srcRoot, "p2", "skills", "beta"),
			},
		},
	}

	// install only p1
	res, err := Install(target, plugins, []string{"p1"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Installed != 2 {
		t.Fatalf("installed=%d", res.Installed)
	}
	assertFile(t, filepath.Join(target, "shared", "SKILL.md"), "from-p1")

	// install p2 — overwrites shared
	res, err = Install(target, plugins, []string{"p2"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Replaced < 1 {
		t.Fatalf("expected replaced, got %+v", res)
	}
	assertFile(t, filepath.Join(target, "shared", "SKILL.md"), "from-p2")
	assertFile(t, filepath.Join(target, "alpha", "SKILL.md"), "alpha-v1") // still there

	installed, err := ListInstalled(target)
	if err != nil {
		t.Fatal(err)
	}
	if len(installed) != 3 {
		t.Fatalf("installed list = %v", installed)
	}

	n, err := Remove(target, []string{"alpha", "beta"})
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("removed=%d", n)
	}
	installed, err = ListInstalled(target)
	if err != nil {
		t.Fatal(err)
	}
	if len(installed) != 1 || installed[0] != "shared" {
		t.Fatalf("after remove: %v", installed)
	}
}

func TestDirPaths(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	global := Dir(settings.Paths{Home: home, AgentDir: filepath.Join(home, ".pi", "agent")})
	wantGlobal := filepath.Join(home, ".pi", "agent", "skills")
	if global != wantGlobal {
		t.Fatalf("global = %q want %q", global, wantGlobal)
	}
	local := Dir(settings.Paths{Local: true, Cwd: cwd})
	wantLocal := filepath.Join(cwd, ".pi", "skills")
	if local != wantLocal {
		t.Fatalf("local = %q want %q", local, wantLocal)
	}
}

func TestSelectByOnly(t *testing.T) {
	plugins := []PluginSkills{
		{Name: "a"},
		{Name: "b"},
		{Name: "c"},
	}
	got, err := SelectByOnly(plugins, []string{"c", "a"})
	if err != nil {
		t.Fatal(err)
	}
	// marketplace order
	if len(got) != 2 || got[0] != "a" || got[1] != "c" {
		t.Fatalf("got %v", got)
	}
	_, err = SelectByOnly(plugins, []string{"nope"})
	if err == nil {
		t.Fatal("expected error for unknown plugin")
	}
}

func assertFile(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != want {
		t.Fatalf("%s: got %q want %q", path, data, want)
	}
}

package catalog

import (
	"testing"
)

func TestLoadEmbedded(t *testing.T) {
	c, err := LoadEmbedded()
	if err != nil {
		t.Fatalf("LoadEmbedded: %v", err)
	}
	if len(c.Packages) < 25 {
		t.Fatalf("expected full catalog, got %d packages", len(c.Packages))
	}
	for _, id := range []string{
		"extension-settings",
		"subagents",
		"goal",
		"cache-optimizer",
		"xai-oauth",
		"cliproxyapi-provider",
	} {
		if c.ByID(id) == nil {
			t.Fatalf("missing package %q", id)
		}
	}
}

func TestSourceResolution(t *testing.T) {
	tests := []struct {
		name string
		pkg  Package
		want string
	}{
		{
			name: "npm latest",
			pkg:  Package{Type: TypeNPM, Package: "pi-subagents", Version: VersionLatest},
			want: "npm:pi-subagents",
		},
		{
			name: "npm pinned",
			pkg:  Package{Type: TypeNPM, Package: "pi-subagents", Version: "0.13.3"},
			want: "npm:pi-subagents@0.13.3",
		},
		{
			name: "npm scoped latest",
			pkg:  Package{Type: TypeNPM, Package: "@devkade/pi-plan", Version: VersionLatest},
			want: "npm:@devkade/pi-plan",
		},
		{
			name: "git latest",
			pkg:  Package{Type: TypeGit, Repo: "github.com/VandeeFeng/pi-memory-md", Version: VersionLatest},
			want: "git:github.com/VandeeFeng/pi-memory-md",
		},
		{
			name: "git pinned",
			pkg:  Package{Type: TypeGit, Repo: "github.com/tintinweb/pi-manage-todo-list", Version: "b75c449aa85ce328e9a8b632f62bf642aed40359"},
			want: "git:github.com/tintinweb/pi-manage-todo-list@b75c449aa85ce328e9a8b632f62bf642aed40359",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.pkg.Source(); got != tt.want {
				t.Fatalf("Source() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExpandDependsOn(t *testing.T) {
	c, err := LoadEmbedded()
	if err != nil {
		t.Fatal(err)
	}
	selected := map[string]struct{}{"powerbar": {}}
	expanded := c.ExpandDependsOn(selected)
	for _, id := range []string{"powerbar", "extension-settings"} {
		if _, ok := expanded[id]; !ok {
			t.Errorf("missing dependency %q", id)
		}
	}
}

func TestResolveSelectionOnly(t *testing.T) {
	c, err := LoadEmbedded()
	if err != nil {
		t.Fatal(err)
	}
	sel, err := c.ResolveSelection([]string{"core"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for id := range sel {
		p := c.ByID(id)
		if p.Category != "core" {
			t.Errorf("unexpected non-core package %q", id)
		}
	}
	if _, ok := sel["subagents"]; !ok {
		t.Error("expected subagents in core")
	}
}

func TestResolveSelectionBadSelector(t *testing.T) {
	c, err := LoadEmbedded()
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.ResolveSelection([]string{"nope"}, nil)
	if err == nil {
		t.Fatal("expected error for unknown selector")
	}
}

func TestCatalogMatchesUserPackages(t *testing.T) {
	c, err := LoadEmbedded()
	if err != nil {
		t.Fatal(err)
	}
	// Sources from the user's installed package list (order independent).
	want := []string{
		"npm:@juanibiapina/pi-extension-settings",
		"npm:pi-subagents",
		"npm:pi-ask-user",
		"npm:pi-mcp-adapter",
		"npm:pi-web-access",
		"npm:pi-memory-md",
		"npm:@devkade/pi-plan",
		"npm:pi-simplify",
		"npm:pi-add-dir",
		"npm:pi-prompt-template-model",
		"npm:pi-claude-cli",
		"npm:@plannotator/pi-extension",
		"npm:pi-slopchop",
		"npm:@juanibiapina/pi-powerbar",
		"npm:@tmustier/pi-usage-extension",
		"npm:@tmustier/pi-raw-paste",
		"npm:@tintinweb/pi-tasks",
		"npm:pi-btw",
		"npm:pi-interactive-shell",
		"npm:@narumitw/pi-statusline",
		"npm:pi-autoresearch",
		"npm:@tmustier/pi-ralph-wiggum",
		"npm:@victor-software-house/pi-curated-themes",
		"npm:pi-terminal-theme",
		"npm:@dietrichgebert/ponytail",
		"npm:@narumitw/pi-goal",
		"npm:pi-cache-optimizer",
		"npm:pi-xai-oauth",
		"npm:@router-for-me/pi-cliproxyapi-provider",
	}
	got := map[string]struct{}{}
	for _, p := range c.Packages {
		got[p.Source()] = struct{}{}
	}
	if len(c.Packages) != len(want) {
		t.Errorf("package count = %d, want %d", len(c.Packages), len(want))
	}
	for _, src := range want {
		if _, ok := got[src]; !ok {
			t.Errorf("missing source %q", src)
		}
	}
	for src := range got {
		found := false
		for _, w := range want {
			if w == src {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("unexpected source %q", src)
		}
	}
}

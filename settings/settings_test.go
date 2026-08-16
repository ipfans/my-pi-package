package settings

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ipfans/my-pi-package/catalog"
)

func TestNormalizeLoadOrderWithObjectEntryAlreadyOrdered(t *testing.T) {
	cat, err := catalog.LoadEmbedded()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "settings.json")
	data := []byte(`{"packages":["npm:@juanibiapina/pi-extension-settings",{"source":"npm:pi-claude-cli"}]}`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	_, changed, err := NormalizeLoadOrder(path, cat)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("already ordered packages should not be rewritten")
	}
}

package omarchyplugin

import (
	"encoding/json"
	"io/fs"
	"os"
	"strings"
	"testing"

	"github.com/Snakeblack/netmon-tray/internal/adapter/omarchy"
)

// A file added to the plugin must be added to the go:embed line too, or the
// installed copy would miss it.
func TestFilesHoldsEveryPluginFile(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || strings.HasSuffix(name, ".go") || strings.HasSuffix(name, ".test.js") {
			continue
		}
		if _, err := fs.Stat(Files, name); err != nil {
			t.Errorf("%s no está embebido: %v", name, err)
		}
	}
	for _, name := range []string{"embed.go", "embed_test.go", "Model.test.js"} {
		if _, err := fs.Stat(Files, name); err == nil {
			t.Errorf("%s no debería estar embebido", name)
		}
	}
}

func TestManifestIDIsTheInstalledFolder(t *testing.T) {
	data, err := fs.ReadFile(Files, "manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.ID != omarchy.PluginID {
		t.Errorf("id = %q, la carpeta instalada es %q", manifest.ID, omarchy.PluginID)
	}
}

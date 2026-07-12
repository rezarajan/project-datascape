package docsgen

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildSiteIncludesSourceAndGeneratedAPIReference(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.md"), []byte("# Test Documentation\n\n[Guide](guide.md)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "guide.md"), []byte("# Guide\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	files, err := BuildSite(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	content := map[string]string{}
	for _, file := range files {
		content[file.Path] = string(file.Content)
	}
	if !strings.Contains(content["index.html"], `href="guide.html"`) {
		t.Fatalf("Markdown link was not rewritten: %s", content["index.html"])
	}
	if !strings.Contains(content["reference/api.html"], "StorageClass") {
		t.Fatal("generated API reference is missing StorageClass")
	}
	if content["assets/site.css"] == "" || content["assets/site.js"] == "" {
		t.Fatal("site assets missing")
	}
}

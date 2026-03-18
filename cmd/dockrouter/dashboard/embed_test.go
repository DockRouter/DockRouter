package dashboard

import (
	"io/fs"
	"testing"
)

func TestEmbeddedFiles(t *testing.T) {
	// Verify all expected files are embedded
	expectedFiles := []string{
		"index.html",
		"app.js",
		"style.css",
	}

	for _, file := range expectedFiles {
		t.Run(file, func(t *testing.T) {
			content, err := Files.ReadFile(file)
			if err != nil {
				t.Errorf("failed to read %s: %v", file, err)
				return
			}
			if len(content) == 0 {
				t.Errorf("%s is empty", file)
			}
		})
	}
}

func TestIndexHTML(t *testing.T) {
	content, err := Files.ReadFile("index.html")
	if err != nil {
		t.Fatalf("failed to read index.html: %v", err)
	}

	// Verify it contains expected HTML elements
	if len(content) == 0 {
		t.Error("index.html is empty")
	}
}

func TestAppJS(t *testing.T) {
	content, err := Files.ReadFile("app.js")
	if err != nil {
		t.Fatalf("failed to read app.js: %v", err)
	}

	if len(content) == 0 {
		t.Error("app.js is empty")
	}
}

func TestStyleCSS(t *testing.T) {
	content, err := Files.ReadFile("style.css")
	if err != nil {
		t.Fatalf("failed to read style.css: %v", err)
	}

	if len(content) == 0 {
		t.Error("style.css is empty")
	}
}

func TestFilesFileSystem(t *testing.T) {
	// Test that Files implements fs.FS interface
	var _ fs.FS = Files

	// Try to open a file
	f, err := Files.Open("index.html")
	if err != nil {
		t.Fatalf("failed to open index.html: %v", err)
	}
	defer f.Close()

	// Get file info
	stat, err := f.Stat()
	if err != nil {
		t.Fatalf("failed to stat index.html: %v", err)
	}

	if stat.Size() == 0 {
		t.Error("index.html has zero size")
	}

	if stat.IsDir() {
		t.Error("index.html should not be a directory")
	}
}

func TestReadDir(t *testing.T) {
	entries, err := fs.ReadDir(Files, ".")
	if err != nil {
		t.Fatalf("failed to read directory: %v", err)
	}

	if len(entries) == 0 {
		t.Error("expected at least one file in embedded filesystem")
	}

	// Check for expected files
	found := make(map[string]bool)
	for _, entry := range entries {
		found[entry.Name()] = true
	}

	expectedFiles := []string{"index.html", "app.js", "style.css"}
	for _, file := range expectedFiles {
		if !found[file] {
			t.Errorf("expected to find %s in embedded filesystem", file)
		}
	}
}

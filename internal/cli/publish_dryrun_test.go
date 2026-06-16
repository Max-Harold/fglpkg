package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/4js-mikefolcher/fglpkg/internal/manifest"
)

// TestPublishPackageDryRunNoNetwork verifies that publishPackage with
// dryRun=true returns successfully without performing any network I/O.
// The tokens passed in are deliberately bogus; if the function tried to
// contact GitHub or the registry it would fail with an auth or connection
// error, which this test would surface.
func TestPublishPackageDryRunNoNetwork(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, content string) {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(full, []byte(content), 0644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	write("fglpkg.json", `{
  "name": "dryrun-test",
  "version": "1.0.0",
  "description": "test",
  "author": "me",
  "license": "UNLICENSED",
  "dependencies": { "fgl": {} }
}`)
	write("Main.42m", "MAIN\nEND MAIN\n")

	origDir, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(origDir) })
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	m, err := manifest.Load(".")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	err = publishPackage(
		m,
		"http://127.0.0.1:1", // registryURL — unreachable; if dry-run violates the contract the test will fail
		"6",                  // generoMajor
		true,                 // dryRun
	)
	if err != nil {
		t.Fatalf("dry-run publishPackage returned error: %v", err)
	}

	// The zip is held only in memory during dry-run; no files should be
	// left behind in the working directory beyond what the test created.
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	allowed := map[string]bool{"fglpkg.json": true, "Main.42m": true}
	for _, e := range entries {
		if !allowed[e.Name()] {
			t.Errorf("unexpected file left behind after dry-run: %s", e.Name())
		}
	}
}

// TestPublishPackageDryRunListsMetadata verifies the dry-run preview prints
// the rich metadata block: scalar fields, dependency counts, README size,
// and a (truncated) flag for an oversized USERGUIDE.
func TestPublishPackageDryRunListsMetadata(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, content string) {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(full, []byte(content), 0644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	write("fglpkg.json", `{
  "name": "meta-test",
  "version": "1.0.0",
  "description": "test",
  "author": "Acme <dev@acme.com>",
  "license": "MIT",
  "repository": "https://github.com/acme/meta-test",
  "genero": "^6.0.0",
  "dependencies": { "fgl": { "json-path": "^1.0.0" }, "java": [ { "groupId": "com.acme", "artifactId": "x", "version": "1.2.3" } ] }
}`)
	write("Main.42m", "MAIN\nEND MAIN\n")
	write("README.md", "# Meta Test")
	write("USERGUIDE.md", strings.Repeat("a", maxReadmeBytes+100)) // forces truncation

	origDir, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(origDir) })
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	m, err := manifest.Load(".")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Capture stdout for the duration of the dry-run.
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	runErr := publishPackage(m, "http://127.0.0.1:1", "6", true)
	_ = w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	out := buf.String()

	if runErr != nil {
		t.Fatalf("dry-run publishPackage returned error: %v", runErr)
	}

	wantSubstrings := []string{
		"metadata:",
		"repository:   https://github.com/acme/meta-test",
		"author:       Acme <dev@acme.com>",
		"license:      MIT",
		"genero:       ^6.0.0",
		"dependencies: 1 fgl, 1 java",
		"readme:       0.0 KB", // "# Meta Test" is well under 1 KB
		"userguide:",           // size line present
		"(truncated)",          // oversized USERGUIDE flagged
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run output missing %q\n---output---\n%s", want, out)
		}
	}
}

package hooks_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/4js-mikefolcher/fglpkg/internal/hooks"
	"github.com/4js-mikefolcher/fglpkg/internal/manifest"
)

// ─── mkdir ────────────────────────────────────────────────────────────────────

func TestMkdirCreatesNestedDirs(t *testing.T) {
	root := t.TempDir()
	h := manifest.Hooks{
		manifest.HookPostInstall: {{Op: manifest.HookOpMkdir, Path: "var/cache/sub"}},
	}
	if err := hooks.Run(h, manifest.HookPostInstall, root); err != nil {
		t.Fatalf("Run: %v", err)
	}
	info, err := os.Stat(filepath.Join(root, "var", "cache", "sub"))
	if err != nil {
		t.Fatalf("expected dir created: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("expected directory, got file")
	}
}

func TestMkdirIdempotent(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "x"), 0o755); err != nil {
		t.Fatal(err)
	}
	h := manifest.Hooks{
		manifest.HookPostInstall: {{Op: manifest.HookOpMkdir, Path: "x"}},
	}
	if err := hooks.Run(h, manifest.HookPostInstall, root); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestMkdirRefusesAbsolutePath(t *testing.T) {
	root := t.TempDir()
	h := manifest.Hooks{
		manifest.HookPostInstall: {{Op: manifest.HookOpMkdir, Path: "/etc/evil"}},
	}
	err := hooks.Run(h, manifest.HookPostInstall, root)
	if err == nil {
		t.Fatal("expected absolute-path error")
	}
	if !strings.Contains(err.Error(), "relative") {
		t.Errorf("error should mention relative, got: %v", err)
	}
}

func TestMkdirRefusesParentTraversal(t *testing.T) {
	root := t.TempDir()
	// Build the manifest by hand (bypassing manifest.Validate) to ensure
	// the executor's own safety check catches the escape.
	h := manifest.Hooks{
		manifest.HookPostInstall: {{Op: manifest.HookOpMkdir, Path: "../escape"}},
	}
	err := hooks.Run(h, manifest.HookPostInstall, root)
	if err == nil {
		t.Fatal("expected escape error")
	}
	if !strings.Contains(err.Error(), "escape") {
		t.Errorf("error should mention escape, got: %v", err)
	}
}

// ─── copy-files ───────────────────────────────────────────────────────────────

func TestCopyFilesSingleFile(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src.txt")
	if err := os.WriteFile(src, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := manifest.Hooks{
		manifest.HookPostInstall: {{Op: manifest.HookOpCopyFiles, From: "src.txt", To: "out/dst.txt"}},
	}
	if err := hooks.Run(h, manifest.HookPostInstall, root); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(root, "out", "dst.txt"))
	if err != nil {
		t.Fatalf("dst missing: %v", err)
	}
	if string(got) != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

func TestCopyFilesGlob(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "templates"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.tpl", "b.tpl", "ignore.txt"} {
		if err := os.WriteFile(filepath.Join(root, "templates", name), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	h := manifest.Hooks{
		manifest.HookPostInstall: {{Op: manifest.HookOpCopyFiles, From: "templates/*.tpl", To: "share"}},
	}
	if err := hooks.Run(h, manifest.HookPostInstall, root); err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, name := range []string{"a.tpl", "b.tpl"} {
		if _, err := os.Stat(filepath.Join(root, "share", name)); err != nil {
			t.Errorf("expected %s copied: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "share", "ignore.txt")); err == nil {
		t.Errorf("non-matching file should not have been copied")
	}
}

func TestCopyFilesDirectoryTree(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "in")
	if err := os.MkdirAll(filepath.Join(src, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "top.txt"), []byte("t"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "sub", "nested.txt"), []byte("n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := manifest.Hooks{
		manifest.HookPostInstall: {{Op: manifest.HookOpCopyFiles, From: "in", To: "out"}},
	}
	if err := hooks.Run(h, manifest.HookPostInstall, root); err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, p := range []string{"out/top.txt", "out/sub/nested.txt"} {
		if _, err := os.Stat(filepath.Join(root, p)); err != nil {
			t.Errorf("expected %s copied: %v", p, err)
		}
	}
}

func TestCopyFilesGlobMatchedNothing(t *testing.T) {
	root := t.TempDir()
	h := manifest.Hooks{
		manifest.HookPostInstall: {{Op: manifest.HookOpCopyFiles, From: "*.nope", To: "share"}},
	}
	if err := hooks.Run(h, manifest.HookPostInstall, root); err == nil {
		t.Fatal("expected error when glob matches nothing")
	}
}

// ─── ordering / abort ─────────────────────────────────────────────────────────

func TestOpsRunInOrderAndAbortOnFirstError(t *testing.T) {
	root := t.TempDir()
	// First op succeeds (creates dir1); second op fails (absolute path);
	// third op would create dir3 but should never run.
	h := manifest.Hooks{
		manifest.HookPostInstall: {
			{Op: manifest.HookOpMkdir, Path: "dir1"},
			{Op: manifest.HookOpMkdir, Path: "/abs"},
			{Op: manifest.HookOpMkdir, Path: "dir3"},
		},
	}
	err := hooks.Run(h, manifest.HookPostInstall, root)
	if err == nil {
		t.Fatal("expected error from absolute path op")
	}
	if _, err := os.Stat(filepath.Join(root, "dir1")); err != nil {
		t.Errorf("dir1 should have been created before the failing op: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "dir3")); err == nil {
		t.Errorf("dir3 should NOT exist — execution must abort on first error")
	}
}

func TestUnknownEventIsNoop(t *testing.T) {
	root := t.TempDir()
	// An empty hooks map for the requested event must be a clean no-op,
	// even if other events are populated.
	h := manifest.Hooks{
		manifest.HookPostInstall: {{Op: manifest.HookOpMkdir, Path: "x"}},
	}
	if err := hooks.Run(h, manifest.HookPrePublish, root); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "x")); err == nil {
		t.Error("postinstall ops must not run for prepublish event")
	}
}

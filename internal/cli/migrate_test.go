package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// migrateStub serves a single replacement package "newpkg" with two versions
// via the consumer protocol, so registry.Resolve("newpkg", "latest", ...)
// resolves to 2.0.0.
func migrateStub(t *testing.T) *httptest.Server {
	t.Helper()
	detail := map[string]any{
		"slug": "newpkg",
		"name": "newpkg",
		"versions": []map[string]any{
			{"version": "1.0.0", "artifacts": []map[string]any{
				{"variant": "default", "sha256": "a", "download_url": "https://example.com/newpkg-1.0.0.zip"},
			}},
			{"version": "2.0.0", "artifacts": []map[string]any{
				{"variant": "default", "sha256": "b", "download_url": "https://example.com/newpkg-2.0.0.zip"},
			}},
		},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/registry/packages/", func(w http.ResponseWriter, r *http.Request) {
		slug := strings.TrimPrefix(r.URL.Path, "/registry/packages/")
		if slug != "newpkg" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(detail)
	})
	return httptest.NewServer(mux)
}

// chdirTemp writes fglpkg.json with the given body into a temp dir and chdirs
// into it for the duration of the test.
func chdirTemp(t *testing.T, manifestJSON string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "fglpkg.json"), []byte(manifestJSON), 0644); err != nil {
		t.Fatalf("write fglpkg.json: %v", err)
	}
	orig, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(orig) })
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	return dir
}

func TestMigrateUsageError(t *testing.T) {
	for _, args := range [][]string{{}, {"only-one"}, {"a", "b", "c"}} {
		if err := cmdMigrate(args); err == nil {
			t.Errorf("cmdMigrate(%v) = nil, want usage error", args)
		}
	}
}

func TestMigrateOldNotDeclared(t *testing.T) {
	chdirTemp(t, `{
  "name": "proj", "version": "0.1.0",
  "dependencies": { "fgl": { "demo": "^1.0.0" } }
}`)
	err := cmdMigrate([]string{"absent", "newpkg"})
	if err == nil {
		t.Fatal("expected error migrating an undeclared package, got nil")
	}
	if !strings.Contains(err.Error(), "not a declared dependency") {
		t.Errorf("error = %q, want it to mention 'not a declared dependency'", err.Error())
	}
}

func TestMigrateSamePackage(t *testing.T) {
	chdirTemp(t, `{
  "name": "proj", "version": "0.1.0",
  "dependencies": { "fgl": { "demo": "^1.0.0" } }
}`)
	err := cmdMigrate([]string{"demo", "demo"})
	if err == nil {
		t.Fatal("expected error migrating a package to itself, got nil")
	}
	if !strings.Contains(err.Error(), "same") {
		t.Errorf("error = %q, want it to mention 'same'", err.Error())
	}
}

func TestMigrateDryRunLeavesManifestUntouched(t *testing.T) {
	ts := migrateStub(t)
	t.Cleanup(ts.Close)
	t.Setenv("FGLPKG_REGISTRY", ts.URL)
	t.Setenv("FGLPKG_GENERO_VERSION", "6.00.01")

	body := `{
  "name": "proj", "version": "0.1.0",
  "dependencies": { "fgl": { "demo": "^1.0.0" } }
}`
	dir := chdirTemp(t, body)

	stdout, err := captureStdout(t, func() error {
		return cmdMigrate([]string{"demo", "newpkg", "--dry-run"})
	})
	if err != nil {
		t.Fatalf("dry-run returned error: %v", err)
	}

	// Output should describe the planned swap, resolving newpkg to latest (2.0.0).
	for _, sub := range []string{"demo", "newpkg", "2.0.0", "dependencies"} {
		if !strings.Contains(stdout, sub) {
			t.Errorf("dry-run output missing %q\n---\n%s", sub, stdout)
		}
	}

	// The manifest on disk must be byte-for-byte unchanged.
	got, _ := os.ReadFile(filepath.Join(dir, "fglpkg.json"))
	if string(got) != body {
		t.Errorf("dry-run modified fglpkg.json:\n--- got ---\n%s\n--- want ---\n%s", got, body)
	}
}

func TestMigrateDryRunPreservesScope(t *testing.T) {
	ts := migrateStub(t)
	t.Cleanup(ts.Close)
	t.Setenv("FGLPKG_REGISTRY", ts.URL)
	t.Setenv("FGLPKG_GENERO_VERSION", "6.00.01")

	// "demo" lives in devDependencies; the dry-run plan must keep that scope.
	chdirTemp(t, `{
  "name": "proj", "version": "0.1.0",
  "dependencies": { "fgl": {} },
  "devDependencies": { "fgl": { "demo": "^1.0.0" } }
}`)

	stdout, err := captureStdout(t, func() error {
		return cmdMigrate([]string{"demo", "newpkg", "--dry-run"})
	})
	if err != nil {
		t.Fatalf("dry-run returned error: %v", err)
	}
	if !strings.Contains(stdout, "devDependencies") {
		t.Errorf("dry-run output should preserve devDependencies scope\n---\n%s", stdout)
	}
}

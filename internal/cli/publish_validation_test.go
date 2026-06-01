package cli

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/4js-mikefolcher/fglpkg/internal/manifest"
	"github.com/4js-mikefolcher/fglpkg/internal/registry"
)

// versionStubServer responds to /packages/:name/versions with the
// supplied per-package versions map. Returns the legacy flat shape
// (no VersionEntries) so existing tests exercise the back-compat
// branch of checkVariantNotPublished. Unknown names produce a 404.
func versionStubServer(t *testing.T, versionsByName map[string][]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// expecting: /packages/<name>/versions
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/packages/"), "/")
		if len(parts) != 2 || parts[1] != "versions" {
			http.NotFound(w, r)
			return
		}
		name := parts[0]
		versions, ok := versionsByName[name]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":     name,
			"versions": versions,
		})
	}))
}

// variantStubServer responds with the modern shape including
// versionEntries[].variants so checkVariantNotPublished can use the
// variant-aware path. Unknown names produce a 404.
func variantStubServer(t *testing.T, byName map[string][]registry.VersionEntry) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/packages/"), "/")
		if len(parts) != 2 || parts[1] != "versions" {
			http.NotFound(w, r)
			return
		}
		name := parts[0]
		entries, ok := byName[name]
		if !ok {
			http.NotFound(w, r)
			return
		}
		flat := make([]string, 0, len(entries))
		for _, e := range entries {
			flat = append(flat, e.Version)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":           name,
			"versions":       flat,
			"versionEntries": entries,
		})
	}))
}

func TestCheckVariantNotPublishedFirstPublish(t *testing.T) {
	ts := versionStubServer(t, nil) // empty map → every package is 404
	defer ts.Close()
	t.Setenv("FGLPKG_REGISTRY", ts.URL)

	m := manifest.New("brand-new", "0.1.0", "", "")
	if err := checkVariantNotPublished(m, "6"); err != nil {
		t.Errorf("expected nil for first publish, got %v", err)
	}
}

// Legacy server (only flat Versions): same version always blocks.
func TestCheckVariantNotPublishedLegacyServerBlocks(t *testing.T) {
	ts := versionStubServer(t, map[string][]string{
		"demo": {"1.0.0", "1.1.0", "1.2.0"},
	})
	defer ts.Close()
	t.Setenv("FGLPKG_REGISTRY", ts.URL)

	m := manifest.New("demo", "1.1.0", "", "")
	err := checkVariantNotPublished(m, "6")
	if err == nil {
		t.Fatal("expected error when version already published on legacy server")
	}
	if !strings.Contains(err.Error(), "already published") {
		t.Errorf("err = %v, want one mentioning 'already published'", err)
	}
	if !strings.Contains(err.Error(), "fglpkg version") {
		t.Errorf("err = %v, want guidance pointing at `fglpkg version`", err)
	}
}

func TestCheckVariantNotPublishedDifferentVersion(t *testing.T) {
	ts := versionStubServer(t, map[string][]string{
		"demo": {"1.0.0", "1.1.0"},
	})
	defer ts.Close()
	t.Setenv("FGLPKG_REGISTRY", ts.URL)

	m := manifest.New("demo", "2.0.0", "", "")
	if err := checkVariantNotPublished(m, "6"); err != nil {
		t.Errorf("expected nil when bumping past existing versions, got %v", err)
	}
}

// New server shape: same version + same variant blocks.
func TestCheckVariantNotPublishedSameVariantBlocks(t *testing.T) {
	ts := variantStubServer(t, map[string][]registry.VersionEntry{
		"demo": {{Version: "1.0.2", Variants: []string{"6"}}},
	})
	defer ts.Close()
	t.Setenv("FGLPKG_REGISTRY", ts.URL)

	m := manifest.New("demo", "1.0.2", "", "")
	err := checkVariantNotPublished(m, "6")
	if err == nil {
		t.Fatal("expected error when same variant already published")
	}
	if !strings.Contains(err.Error(), "Genero 6") {
		t.Errorf("err = %v, want one mentioning 'Genero 6'", err)
	}
}

// New server shape: same version, different variant ALLOWED — this is the
// regression Laurent hit when publishing the GBL5 variant after GBL6.
func TestCheckVariantNotPublishedNewVariantAllowed(t *testing.T) {
	ts := variantStubServer(t, map[string][]registry.VersionEntry{
		"genero-crypto-api": {{Version: "1.0.2", Variants: []string{"6"}}},
	})
	defer ts.Close()
	t.Setenv("FGLPKG_REGISTRY", ts.URL)

	m := manifest.New("genero-crypto-api", "1.0.2", "", "")
	if err := checkVariantNotPublished(m, "5"); err != nil {
		t.Errorf("expected nil when adding new variant to existing version, got %v", err)
	}
}

func TestCheckVariantNotPublishedRegistryDown(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer ts.Close()
	t.Setenv("FGLPKG_REGISTRY", ts.URL)

	m := manifest.New("demo", "1.0.0", "", "")
	err := checkVariantNotPublished(m, "6")
	if err == nil {
		t.Fatal("expected error when registry is unreachable")
	}
	if !strings.Contains(err.Error(), "cannot check") {
		t.Errorf("err = %v, want one starting with 'cannot check'", err)
	}
}

// TestFetchVersionListWrapsErrNotFound verifies the sentinel survives
// the fmt.Errorf("...: %w", ...) wrapping inside FetchVersionList so
// callers can use errors.Is.
func TestFetchVersionListWrapsErrNotFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer ts.Close()
	t.Setenv("FGLPKG_REGISTRY", ts.URL)

	_, err := registry.FetchVersionList("anything")
	if err == nil {
		t.Fatal("expected error on 404, got nil")
	}
	if !errors.Is(err, registry.ErrNotFound) {
		t.Errorf("errors.Is(err, ErrNotFound) = false; err = %v", err)
	}
}

package installer

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

// writeTestZip builds a zip at zipPath containing the given name→content
// entries, returning the path.
func writeTestZip(t *testing.T, zipPath string, entries map[string]string) {
	t.Helper()
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	defer f.Close()
	w := zip.NewWriter(f)
	for name, content := range entries {
		fw, err := w.Create(name)
		if err != nil {
			t.Fatalf("zip.Create %s: %v", name, err)
		}
		if _, err := fw.Write([]byte(content)); err != nil {
			t.Fatalf("zip write %s: %v", name, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("zip.Close: %v", err)
	}
}

// TestExtractZipRoutedMixedPackage verifies the mixed-zip routing: a zip
// that contains both BDL files and a COMPONENTTYPE directory gets split —
// the COMPONENTTYPE bundle lands under webcomponentsDir, everything else
// under destDir.
func TestExtractZipRoutedMixedPackage(t *testing.T) {
	tmp := t.TempDir()
	zipPath := filepath.Join(tmp, "pkg.zip")
	writeTestZip(t, zipPath, map[string]string{
		"fglpkg.json":         `{"name":"chart-3d","version":"1.0.0","webcomponents":["3DChart"]}`,
		"ChartDemo.42m":       "BDL\n",
		"3DChart/3DChart.html": "<html/>",
		"3DChart/3DChart.js":  "// js",
	})

	destDir := filepath.Join(tmp, "packages", "chart-3d")
	wcDir := filepath.Join(tmp, "webcomponents")

	if err := extractZipRouted(zipPath, destDir, wcDir, []string{"3DChart"}); err != nil {
		t.Fatalf("extractZipRouted: %v", err)
	}

	mustExist := []string{
		filepath.Join(destDir, "fglpkg.json"),
		filepath.Join(destDir, "ChartDemo.42m"),
		filepath.Join(wcDir, "3DChart", "3DChart.html"),
		filepath.Join(wcDir, "3DChart", "3DChart.js"),
	}
	for _, p := range mustExist {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected %s on disk: %v", p, err)
		}
	}

	mustNotExist := []string{
		filepath.Join(destDir, "3DChart", "3DChart.html"), // must NOT leak into packages/
		filepath.Join(wcDir, "ChartDemo.42m"),             // must NOT leak into webcomponents/
		filepath.Join(wcDir, "fglpkg.json"),
	}
	for _, p := range mustNotExist {
		if _, err := os.Stat(p); err == nil {
			t.Errorf("unexpected file at %s — routing leaked", p)
		}
	}
}

// TestExtractZipRoutedPureBDL falls back to the unrouted extraction when
// the manifest declares no webcomponents.
func TestExtractZipRoutedPureBDL(t *testing.T) {
	tmp := t.TempDir()
	zipPath := filepath.Join(tmp, "pkg.zip")
	writeTestZip(t, zipPath, map[string]string{
		"fglpkg.json": `{"name":"pure-bdl","version":"1.0.0"}`,
		"Lib.42m":     "BDL\n",
	})

	destDir := filepath.Join(tmp, "packages", "pure-bdl")
	wcDir := filepath.Join(tmp, "webcomponents")

	if err := extractZipRouted(zipPath, destDir, wcDir, nil); err != nil {
		t.Fatalf("extractZipRouted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destDir, "Lib.42m")); err != nil {
		t.Errorf("expected BDL file in destDir: %v", err)
	}
}

// TestReadWebcomponentsFromZip pulls the webcomponents list out of the
// manifest inside a zip without extracting anything else.
func TestReadWebcomponentsFromZip(t *testing.T) {
	tmp := t.TempDir()
	zipPath := filepath.Join(tmp, "pkg.zip")
	writeTestZip(t, zipPath, map[string]string{
		"fglpkg.json": `{"name":"m","version":"1.0.0","webcomponents":["A","B"]}`,
	})
	got, err := readWebcomponentsFromZip(zipPath)
	if err != nil {
		t.Fatalf("readWebcomponentsFromZip: %v", err)
	}
	if len(got) != 2 || got[0] != "A" || got[1] != "B" {
		t.Errorf("unexpected webcomponents list: %v", got)
	}
}

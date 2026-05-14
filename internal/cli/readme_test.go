package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCollectReadmeMissing verifies that an absent README produces
// ("", nil) rather than an error — publishing without a README is
// allowed.
func TestCollectReadmeMissing(t *testing.T) {
	dir := t.TempDir()
	got, err := collectReadme(dir)
	if err != nil {
		t.Fatalf("collectReadme on empty dir: %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty string", got)
	}
}

// TestCollectReadmePrefersMarkdown verifies that when both README.md
// and README.txt exist, the markdown file wins (it's earlier in
// readmeCandidates).
func TestCollectReadmePrefersMarkdown(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, dir, "README.md", "from md")
	mustWrite(t, dir, "README.txt", "from txt")

	got, err := collectReadme(dir)
	if err != nil {
		t.Fatalf("collectReadme: %v", err)
	}
	if got != "from md" {
		t.Errorf("got %q, want %q (markdown should win)", got, "from md")
	}
}

// TestCollectReadmeCaseInsensitive verifies that a lower-cased
// `readme.md` is found despite the candidate list being all-caps.
func TestCollectReadmeCaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, dir, "readme.md", "lower case")

	got, err := collectReadme(dir)
	if err != nil {
		t.Fatalf("collectReadme: %v", err)
	}
	if got != "lower case" {
		t.Errorf("got %q, want %q", got, "lower case")
	}
}

// TestCollectReadmeFallsBackToPlain verifies that a bare `README`
// (no extension) is still picked up.
func TestCollectReadmeFallsBackToPlain(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, dir, "README", "plain readme")

	got, err := collectReadme(dir)
	if err != nil {
		t.Fatalf("collectReadme: %v", err)
	}
	if got != "plain readme" {
		t.Errorf("got %q, want %q", got, "plain readme")
	}
}

// TestCollectReadmeTruncates verifies that READMEs larger than the cap
// are truncated and end with the truncation marker.
func TestCollectReadmeTruncates(t *testing.T) {
	dir := t.TempDir()
	huge := strings.Repeat("a", maxReadmeBytes+100)
	mustWrite(t, dir, "README.md", huge)

	got, err := collectReadme(dir)
	if err != nil {
		t.Fatalf("collectReadme: %v", err)
	}
	if !strings.HasSuffix(got, readmeTruncationMarker) {
		t.Errorf("output did not end with truncation marker")
	}
	wantLen := maxReadmeBytes + len(readmeTruncationMarker)
	if len(got) != wantLen {
		t.Errorf("len(got) = %d, want %d", len(got), wantLen)
	}
}

// TestCollectReadmeIgnoresDirectories verifies that a directory named
// "README.md" (rare but possible) is skipped rather than treated as a
// file.
func TestCollectReadmeIgnoresDirectories(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "README.md"), 0755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, dir, "README.txt", "real content")

	got, err := collectReadme(dir)
	if err != nil {
		t.Fatalf("collectReadme: %v", err)
	}
	if got != "real content" {
		t.Errorf("got %q, want fallback to README.txt", got)
	}
}

func mustWrite(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

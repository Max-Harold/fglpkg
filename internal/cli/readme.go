package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// readmeCandidates are filename prefixes searched in the package root,
// in priority order. The match is case-insensitive on the basename.
// First match wins; later candidates are ignored once one is found.
var readmeCandidates = []string{
	"README.md",
	"README.markdown",
	"README.rst",
	"README.txt",
	"README",
}

// userguideCandidates mirror readmeCandidates for the package's user
// guide. Same ordering, same case-insensitive root-level match.
var userguideCandidates = []string{
	"USERGUIDE.md",
	"USERGUIDE.markdown",
	"USERGUIDE.rst",
	"USERGUIDE.txt",
	"USERGUIDE",
}

// maxReadmeBytes is the largest doc body (README or USERGUIDE) the CLI
// will send in a publish payload. The registry's hard cap is 2x this —
// see registry-package-metadata. Anything larger gets truncated with a
// trailing marker so consumers know the content was cut.
const maxReadmeBytes = 256 * 1024

const readmeTruncationMarker = "\n\n*(README truncated at 256 KB)*\n"

const userguideTruncationMarker = "\n\n*(USERGUIDE truncated at 256 KB)*\n"

// collectReadme scans the given directory for a top-level README in
// order of preference (markdown first, then rst, txt, plain). Returns
// the file content as a string, truncated to maxReadmeBytes if larger.
// Returns ("", nil) when no README is present — that is not an error,
// publishing without a README is allowed.
func collectReadme(dir string) (string, error) {
	return collectDoc(dir, "README", readmeCandidates, readmeTruncationMarker)
}

// collectUserguide is the USERGUIDE sibling of collectReadme — same
// root-level, case-insensitive, capped scan. Returns ("", nil) when no
// USERGUIDE is present.
func collectUserguide(dir string) (string, error) {
	return collectDoc(dir, "USERGUIDE", userguideCandidates, userguideTruncationMarker)
}

// collectDoc is the shared implementation behind collectReadme and
// collectUserguide. label is used only for error messages. candidates
// are tried in order; the first matching root-level file (matched
// case-insensitively on the basename) wins and is read with the cap +
// marker applied.
func collectDoc(dir, label string, candidates []string, marker string) (string, error) {
	if dir == "" {
		dir = "."
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("cannot scan %s for %s: %w", dir, label, err)
	}

	// Build a lower-cased index so we can match case-insensitively
	// without doing N*M comparisons in the candidate loop.
	byLower := make(map[string]string, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		byLower[strings.ToLower(e.Name())] = e.Name()
	}
	for _, want := range candidates {
		if actual, ok := byLower[strings.ToLower(want)]; ok {
			return readWithCap(filepath.Join(dir, actual), marker)
		}
	}
	return "", nil
}

// readWithCap reads path and, if the content exceeds maxReadmeBytes,
// truncates to that cap and appends marker. The marker tells human
// readers the content was cut and tells the registry's size check the
// payload is intentional rather than malformed.
func readWithCap(path, marker string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("cannot read %s: %w", path, err)
	}
	if len(data) <= maxReadmeBytes {
		return string(data), nil
	}
	return string(data[:maxReadmeBytes]) + marker, nil
}

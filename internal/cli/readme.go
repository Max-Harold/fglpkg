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

// maxReadmeBytes is the largest README the CLI will send in a publish
// payload. The server's hard cap is 2x this — see
// registry/server.MaxReadmeBytes. Anything larger gets truncated with
// a trailing marker so consumers know the content was cut.
const maxReadmeBytes = 256 * 1024

const readmeTruncationMarker = "\n\n*(README truncated at 256 KB)*\n"

// collectReadme scans the given directory for a top-level README in
// order of preference (markdown first, then rst, txt, plain). Returns
// the file content as a string, truncated to maxReadmeBytes if larger.
// Returns ("", nil) when no README is present — that is not an error,
// publishing without a README is allowed.
func collectReadme(dir string) (string, error) {
	if dir == "" {
		dir = "."
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("cannot scan %s for README: %w", dir, err)
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
	for _, want := range readmeCandidates {
		if actual, ok := byLower[strings.ToLower(want)]; ok {
			return readWithCap(filepath.Join(dir, actual))
		}
	}
	return "", nil
}

// readWithCap reads path and, if the content exceeds maxReadmeBytes,
// truncates to that cap and appends a marker. The marker tells human
// readers the content was cut and tells the server's size check the
// payload is intentional rather than malformed.
func readWithCap(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("cannot read %s: %w", path, err)
	}
	if len(data) <= maxReadmeBytes {
		return string(data), nil
	}
	return string(data[:maxReadmeBytes]) + readmeTruncationMarker, nil
}

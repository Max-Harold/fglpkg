// Package github provides helpers for uploading and downloading fglpkg
// package zips via GitHub Releases on a shared repository.
package github

import (
	"strings"
)

// VariantAssetName returns the zip filename for a Genero variant release asset.
// Example: VariantAssetName("poiapi", "1.0.0", "4") → "poiapi-1.0.0-genero4.zip"
func VariantAssetName(name, version, generoMajor string) string {
	return name + "-" + version + "-genero" + generoMajor + ".zip"
}

// IsGitHubURL returns true if the URL points to the GitHub API (used to
// decide whether to attach auth headers during downloads).
func IsGitHubURL(url string) bool {
	return strings.HasPrefix(url, "https://api.github.com/")
}

package cli

import (
	"errors"
	"fmt"

	"github.com/4js-mikefolcher/fglpkg/internal/manifest"
	"github.com/4js-mikefolcher/fglpkg/internal/registry"
)

// checkVariantNotPublished returns nil if the (m.Name, m.Version, generoMajor)
// triple is safe to publish. It returns:
//
//   - nil when the package is unknown to the registry (first publish), when
//     the version exists but the specific Genero major variant does not, or
//     when the registry response carries variant info we can read.
//   - a guidance error pointing at `fglpkg version` if the same version
//     AND the same variant are already published.
//   - a wrapped network/server error if the check itself failed —
//     callers must treat this as "we cannot tell whether re-publish
//     would clobber" and abort, not silently allow.
//
// Back-compat: older registry servers omit VersionEntries[].Variants and
// return only the flat Versions list. In that case we fall back to today's
// behaviour and block on any version-string match, since we have no way
// to tell which variants exist.
func checkVariantNotPublished(m *manifest.Manifest, generoMajor string) error {
	vl, err := registry.PublisherVersionList(m.Name)
	if err != nil {
		if errors.Is(err, registry.ErrNotFound) {
			// First publish for this package name. Nothing to clobber.
			return nil
		}
		return fmt.Errorf("cannot check whether version %s is already published: %w",
			m.Version, err)
	}

	// Prefer the variant-aware path when the server returned VersionEntries.
	for _, e := range vl.VersionEntries {
		if e.Version != m.Version {
			continue
		}
		// Found the version. If this specific variant is already published,
		// reject; otherwise the publish is adding a new variant and is fine.
		for _, v := range e.Variants {
			if v == generoMajor {
				return fmt.Errorf(
					"version %s of %s is already published for Genero %s\n"+
						"bump the version before publishing again:\n"+
						"    fglpkg version patch     # %s -> next patch\n"+
						"    fglpkg version minor     # next minor\n"+
						"    fglpkg version major     # next major",
					m.Version, m.Name, generoMajor, m.Version)
			}
		}
		return nil
	}

	// Legacy server fallback: VersionEntries empty or no matching entry.
	// We can't tell which variant the existing version owns, so block on
	// any version-string match — same behaviour as before this refactor.
	if len(vl.VersionEntries) == 0 {
		for _, v := range vl.Versions {
			if v == m.Version {
				return fmt.Errorf(
					"version %s of %s is already published\n"+
						"bump the version before publishing again:\n"+
						"    fglpkg version patch     # %s -> next patch\n"+
						"    fglpkg version minor     # next minor\n"+
						"    fglpkg version major     # next major",
					m.Version, m.Name, m.Version)
			}
		}
	}
	return nil
}

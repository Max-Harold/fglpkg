// Package signing implements Layer 1 of the fglpkg package-signing design:
// verification of registry-signed artifacts (see specs/package-signing.md).
//
// The Genero Intelligence (GI) registry signs every artifact at publish time
// with Ed25519 over an RFC 8785 (JCS) canonical payload, and publishes its
// working public keys in a root-signed manifest at
// GET /registry/.well-known/keys.json. This package reconstructs that canonical
// payload on the client, verifies the manifest against a root public key pinned
// in the binary (root.go), and verifies each artifact signature against the
// working key named in its envelope.
//
// It is the Go counterpart of the reference verifier in
// test/signatures/verify-signing.mjs and docs/manual-signing-test.md; the two
// must agree byte-for-byte on the canonical payload or every signature fails.
package signing

import (
	"errors"
	"fmt"
)

// Sentinel errors mirror the taxonomy in specs/package-signing.md §"Error
// taxonomy". Callers detect them with errors.Is.
var (
	// ErrKeyUnknown is returned when an artifact's signature names a keyid
	// that is not present in the current keys manifest.
	ErrKeyUnknown = errors.New("signing key not found in keys manifest")

	// ErrKeyExpired is returned when the artifact's upload time falls outside
	// the validity window of the key that signed it.
	ErrKeyExpired = errors.New("signing key validity window does not cover upload time")

	// ErrManifestUnverified is returned when the keys manifest does not verify
	// against the pinned root key — the client refuses to trust unsigned key
	// material.
	ErrManifestUnverified = errors.New("keys manifest could not be verified against the pinned root key")

	// ErrUnsigned is returned when an artifact record carries no signature.
	// Whether this is fatal is the caller's decision (enforce mode).
	ErrUnsigned = errors.New("artifact is not signed")
)

// ErrSignatureMismatch is returned when an artifact's Ed25519 signature does
// not verify against the reconstructed canonical payload. Modelled on
// checksum.ErrMismatch: it carries enough context for a descriptive message.
type ErrSignatureMismatch struct {
	Name    string
	Version string
	Variant string
	KeyID   string
}

func (e *ErrSignatureMismatch) Error() string {
	return fmt.Sprintf(
		"signature mismatch for %s@%s (%s): the registry signature (keyid %s) does not "+
			"verify against the package metadata.\n"+
			"The package may have been tampered with. Delete it and retry, or contact "+
			"the package author.",
		e.Name, e.Version, e.Variant, e.KeyID,
	)
}

package signing

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Key is one working public key from the signed keys manifest.
type Key struct {
	KeyID     string `json:"keyid"`
	Alg       string `json:"alg"`
	Pub       string `json:"pub"` // base64 raw 32-byte Ed25519 public key
	ValidFrom string `json:"validFrom"`
	ValidTo   string `json:"validTo"`
}

// manifestSig is the root-signature block of the manifest.
type manifestSig struct {
	RootKeyID string `json:"rootKeyid"`
	Alg       string `json:"alg"`
	Sig       string `json:"sig"`
}

// Manifest is a parsed and root-verified keys manifest.
type Manifest struct {
	Keys     []Key       `json:"keys"`
	IssuedAt string      `json:"issuedAt"`
	Sig      manifestSig `json:"sig"`
}

// ParseAndVerifyManifest parses raw keys.json bytes and verifies the manifest
// signature against a pinned root key. It returns ErrManifestUnverified if the
// root is untrusted or the signature does not verify — the caller must never
// trust the keys inside an unverified manifest.
func ParseAndVerifyManifest(raw []byte) (*Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("invalid keys manifest: %w", err)
	}
	if err := verifyManifestRoot(raw, m.Sig); err != nil {
		return nil, err
	}
	return &m, nil
}

// verifyManifestRoot verifies that raw's {issuedAt, keys} canonicalization is
// signed by the pinned root key named in sig. It canonicalizes from the raw
// JSON (not the typed struct) so any fields the registry may add to a key entry
// are preserved and included in the signed bytes, exactly as the server signed
// them.
func verifyManifestRoot(raw []byte, sig manifestSig) error {
	root, ok := rootKeyByID(sig.RootKeyID)
	if !ok {
		return fmt.Errorf("%w: manifest signed by untrusted root %q", ErrManifestUnverified, sig.RootKeyID)
	}

	var generic map[string]any
	if err := json.Unmarshal(raw, &generic); err != nil {
		return fmt.Errorf("%w: %v", ErrManifestUnverified, err)
	}
	signed, err := canonicalJSON(map[string]any{
		"issuedAt": generic["issuedAt"],
		"keys":     generic["keys"],
	})
	if err != nil {
		return fmt.Errorf("%w: %v", ErrManifestUnverified, err)
	}

	ok, err = verifyEd25519(root.Pub, signed, sig.Sig)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrManifestUnverified, err)
	}
	if !ok {
		return fmt.Errorf("%w: root signature does not verify", ErrManifestUnverified)
	}
	return nil
}

// KeyByID returns the working key with the given keyid, if present.
func (m *Manifest) KeyByID(keyid string) (Key, bool) {
	for _, k := range m.Keys {
		if k.KeyID == keyid {
			return k, true
		}
	}
	return Key{}, false
}

// ArtifactSignature is the signature envelope attached to an artifact record.
type ArtifactSignature struct {
	KeyID string
	Alg   string
	Sig   string
}

// VerifyArtifact verifies an artifact's signature against the manifest's
// working keys. It performs the same three checks as the reference verifier:
//  1. the signature's keyid is present in the manifest (else ErrKeyUnknown);
//  2. the artifact's upload time falls within that key's validity window
//     (else ErrKeyExpired);
//  3. the Ed25519 signature verifies against the reconstructed canonical
//     payload (else *ErrSignatureMismatch).
//
// Note (backfill): the registry signs backfilled historical artifacts with the
// current working key but sets uploaded_at to the artifact's original
// created_at, which can predate the key's validFrom. Such artifacts fail the
// window check here and surface as ErrKeyExpired; under the default warn
// enforcement that is a warning, not a hard failure. Selection is otherwise by
// keyid, matching the reference verifier.
func (m *Manifest) VerifyArtifact(p Payload, sig ArtifactSignature) error {
	key, ok := m.KeyByID(sig.KeyID)
	if !ok {
		return fmt.Errorf("%w: keyid %q (run 'fglpkg update' or upgrade the CLI)", ErrKeyUnknown, sig.KeyID)
	}
	if err := checkWindow(p.UploadedAt, key); err != nil {
		return err
	}
	verified, err := verifyEd25519(key.Pub, p.Canonical(), sig.Sig)
	if err != nil {
		return err
	}
	if !verified {
		return &ErrSignatureMismatch{
			Name: p.Name, Version: p.Version, Variant: p.Variant, KeyID: sig.KeyID,
		}
	}
	return nil
}

// checkWindow reports whether uploadedAt falls within [validFrom, validTo] of
// key. uploadedAt may be an RFC 3339 string or the SQLite "YYYY-MM-DD HH:MM:SS"
// form the registry stores; both are normalised to UTC before comparison,
// mirroring the reference verifier.
func checkWindow(uploadedAt string, key Key) error {
	up, err := parseTimestamp(uploadedAt)
	if err != nil {
		return fmt.Errorf("%w: cannot parse upload time %q: %v", ErrKeyExpired, uploadedAt, err)
	}
	from, err := parseTimestamp(key.ValidFrom)
	if err != nil {
		return fmt.Errorf("%w: cannot parse validFrom %q: %v", ErrKeyExpired, key.ValidFrom, err)
	}
	to, err := parseTimestamp(key.ValidTo)
	if err != nil {
		return fmt.Errorf("%w: cannot parse validTo %q: %v", ErrKeyExpired, key.ValidTo, err)
	}
	if up.Before(from) || up.After(to) {
		return fmt.Errorf("%w: uploaded %s, key %q valid [%s .. %s]",
			ErrKeyExpired, uploadedAt, key.KeyID, key.ValidFrom, key.ValidTo)
	}
	return nil
}

// parseTimestamp accepts RFC 3339 timestamps and the SQLite
// "YYYY-MM-DD HH:MM:SS" form (assumed UTC), returning a UTC time.
func parseTimestamp(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC(), nil
	}
	// SQLite datetime('now') form: "2026-06-06 23:07:09" (UTC, no zone).
	if t, err := time.Parse("2006-01-02 15:04:05", s); err == nil {
		return t.UTC(), nil
	}
	return time.Time{}, fmt.Errorf("unrecognised timestamp format")
}

package signing

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
)

// decodeRawPub decodes a base64 (standard alphabet) raw 32-byte Ed25519 public
// key, the wire form used by both the keys manifest (`pub`) and the pinned root
// keys (root.go).
func decodeRawPub(b64 string) (ed25519.PublicKey, error) {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("invalid base64 public key: %w", err)
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("expected %d-byte Ed25519 public key, got %d bytes",
			ed25519.PublicKeySize, len(raw))
	}
	return ed25519.PublicKey(raw), nil
}

// decodeSig decodes a base64 (standard alphabet) raw 64-byte Ed25519 signature.
func decodeSig(b64 string) ([]byte, error) {
	sig, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("invalid base64 signature: %w", err)
	}
	if len(sig) != ed25519.SignatureSize {
		return nil, fmt.Errorf("expected %d-byte Ed25519 signature, got %d bytes",
			ed25519.SignatureSize, len(sig))
	}
	return sig, nil
}

// verifyEd25519 reports whether sigB64 is a valid Ed25519 signature over
// message under the base64 raw public key pubB64. It returns an error (not just
// false) when either input is malformed, so callers can distinguish "bad key
// material" from "signature does not verify".
func verifyEd25519(pubB64 string, message []byte, sigB64 string) (bool, error) {
	pub, err := decodeRawPub(pubB64)
	if err != nil {
		return false, err
	}
	sig, err := decodeSig(sigB64)
	if err != nil {
		return false, err
	}
	return ed25519.Verify(pub, message, sig), nil
}

package signing

// RootKey is a pinned root public key: the out-of-band trust anchor that
// verifies the keys manifest. The public key must arrive compiled into the
// binary — never fetched — because it is used to decide whether to trust the
// registry that serves the manifest (fetching it from that registry would be
// circular). Rotating a root key requires a new CLI release.
type RootKey struct {
	KeyID string
	Pub   string // base64 raw 32-byte Ed25519 public key
}

// pinnedRootKeys are the root public keys this build trusts. Multiple entries
// allow overlapping old+new roots across a root-key rotation.
//
// root-test-1 is the Genero Intelligence *test* registry
// (genero-intelligence-test.michael-folcher.workers.dev) root. When the
// production registry gains a root, add its key here alongside this one.
var pinnedRootKeys = []RootKey{
	{
		KeyID: "root-test-1",
		Pub:   "IT1y7PBb9/ZXkbIuWcAPRSANiez/A3yLe9z5ps+DoXk=",
	},
}

// rootKeyByID returns the pinned root key with the given keyid, if trusted.
func rootKeyByID(keyid string) (RootKey, bool) {
	for _, k := range pinnedRootKeys {
		if k.KeyID == keyid {
			return k, true
		}
	}
	return RootKey{}, false
}

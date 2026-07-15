package signing

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
)

// testKey builds a deterministic Ed25519 key pair from a one-byte seed pattern
// so golden vectors are reproducible across runs.
func testKey(seedByte byte) (pub string, priv ed25519.PrivateKey) {
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = seedByte
	}
	priv = ed25519.NewKeyFromSeed(seed)
	pub = base64.StdEncoding.EncodeToString(priv.Public().(ed25519.PublicKey))
	return pub, priv
}

func sign(priv ed25519.PrivateKey, msg []byte) string {
	return base64.StdEncoding.EncodeToString(ed25519.Sign(priv, msg))
}

// ─── Canonicalization golden vectors ───────────────────────────────────────

func TestCanonicalGoldenVectors(t *testing.T) {
	cases := []struct {
		name string
		p    Payload
		want string
	}{
		{
			name: "typical artifact",
			p: Payload{
				Name: "chart-3d", Version: "1.0.0", Variant: "genero6",
				SHA256: "b6e1", Size: 87477,
				UploadedAt: "2026-07-02 14:22:00", Uploader: "partner:pt_7f2a",
			},
			want: `{"artifact":{"name":"chart-3d","sha256":"b6e1","size":87477,` +
				`"uploaded_at":"2026-07-02 14:22:00","uploader":"partner:pt_7f2a",` +
				`"variant":"genero6","version":"1.0.0"}}`,
		},
		{
			name: "boundary size zero, empty uploader",
			p: Payload{
				Name: "qrcode", Version: "0.0.1", Variant: "genero6",
				SHA256: "abc", Size: 0, UploadedAt: "2026-01-01T00:00:00Z", Uploader: "",
			},
			want: `{"artifact":{"name":"qrcode","sha256":"abc","size":0,` +
				`"uploaded_at":"2026-01-01T00:00:00Z","uploader":"",` +
				`"variant":"genero6","version":"0.0.1"}}`,
		},
		{
			name: "boundary large size",
			p: Payload{
				Name: "big", Version: "2.3.4", Variant: "default",
				SHA256: "ff", Size: 999999999, UploadedAt: "2026-07-06 16:39:08", Uploader: "partner:x",
			},
			want: `{"artifact":{"name":"big","sha256":"ff","size":999999999,` +
				`"uploaded_at":"2026-07-06 16:39:08","uploader":"partner:x",` +
				`"variant":"default","version":"2.3.4"}}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := string(tc.p.Canonical())
			if got != tc.want {
				t.Errorf("canonical mismatch\n got: %s\nwant: %s", got, tc.want)
			}
		})
	}
}

// TestCanonicalKeyOrderIndependence proves that object key insertion order does
// not affect the canonical bytes — the core JCS guarantee.
func TestCanonicalKeyOrderIndependence(t *testing.T) {
	a, err := canonicalJSON(map[string]any{"b": "2", "a": "1", "c": int64(3)})
	if err != nil {
		t.Fatal(err)
	}
	b, err := canonicalJSON(map[string]any{"c": int64(3), "a": "1", "b": "2"})
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != string(b) {
		t.Fatalf("key order affected output: %s vs %s", a, b)
	}
	if want := `{"a":"1","b":"2","c":3}`; string(a) != want {
		t.Fatalf("got %s want %s", a, want)
	}
}

func TestCanonicalStringEscaping(t *testing.T) {
	got, err := canonicalJSON(map[string]any{"k": "a\"b\\c\n\td"})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"k":"a\"b\\c\n\td"}`
	if string(got) != want {
		t.Fatalf("got %s want %s", got, want)
	}
}

func TestCanonicalRejectsFloat(t *testing.T) {
	if _, err := canonicalJSON(map[string]any{"n": 1.5}); err == nil {
		t.Fatal("expected error for non-integer number")
	}
}

// ─── Artifact verification ──────────────────────────────────────────────────

func validKeyManifest(t *testing.T, keyid, pub, from, to string) *Manifest {
	t.Helper()
	return &Manifest{
		Keys:     []Key{{KeyID: keyid, Alg: "ed25519", Pub: pub, ValidFrom: from, ValidTo: to}},
		IssuedAt: "2026-07-06T00:00:00Z",
	}
}

func TestVerifyArtifactHappyPath(t *testing.T) {
	pub, priv := testKey(0x11)
	m := validKeyManifest(t, "test-key-1", pub, "2026-07-01T00:00:00Z", "2027-07-01T00:00:00Z")

	p := Payload{Name: "qrcode", Version: "1.0.0", Variant: "genero6",
		SHA256: "deadbeef", Size: 512, UploadedAt: "2026-07-02 14:22:00", Uploader: "partner:x"}
	sig := ArtifactSignature{KeyID: "test-key-1", Alg: "ed25519", Sig: sign(priv, p.Canonical())}

	if err := m.VerifyArtifact(p, sig); err != nil {
		t.Fatalf("expected verify to pass, got %v", err)
	}
}

func TestVerifyArtifactTampered(t *testing.T) {
	pub, priv := testKey(0x22)
	m := validKeyManifest(t, "k", pub, "2026-01-01T00:00:00Z", "2030-01-01T00:00:00Z")

	p := Payload{Name: "p", Version: "1.0.0", Variant: "genero6",
		SHA256: "aaaa", Size: 10, UploadedAt: "2026-07-02 14:22:00", Uploader: "partner:x"}
	sig := ArtifactSignature{KeyID: "k", Sig: sign(priv, p.Canonical())}

	p.Size = 11 // tamper after signing
	err := m.VerifyArtifact(p, sig)
	var mm *ErrSignatureMismatch
	if !errors.As(err, &mm) {
		t.Fatalf("expected *ErrSignatureMismatch, got %v", err)
	}
}

func TestVerifyArtifactUnknownKey(t *testing.T) {
	pub, priv := testKey(0x33)
	m := validKeyManifest(t, "k1", pub, "2026-01-01T00:00:00Z", "2030-01-01T00:00:00Z")
	p := Payload{Name: "p", Version: "1", Variant: "v", SHA256: "x", Size: 1,
		UploadedAt: "2026-07-02 14:22:00"}
	sig := ArtifactSignature{KeyID: "k2", Sig: sign(priv, p.Canonical())}
	if err := m.VerifyArtifact(p, sig); !errors.Is(err, ErrKeyUnknown) {
		t.Fatalf("expected ErrKeyUnknown, got %v", err)
	}
}

func TestVerifyArtifactExpiredWindow(t *testing.T) {
	pub, priv := testKey(0x44)
	m := validKeyManifest(t, "k", pub, "2026-07-10T00:00:00Z", "2027-07-10T00:00:00Z")
	p := Payload{Name: "p", Version: "1", Variant: "v", SHA256: "x", Size: 1,
		UploadedAt: "2026-07-02 14:22:00"} // before validFrom
	sig := ArtifactSignature{KeyID: "k", Sig: sign(priv, p.Canonical())}
	if err := m.VerifyArtifact(p, sig); !errors.Is(err, ErrKeyExpired) {
		t.Fatalf("expected ErrKeyExpired, got %v", err)
	}
}

// ─── Manifest root verification ─────────────────────────────────────────────

// buildSignedManifest assembles a keys.json signed by a test root key that is
// temporarily added to the pinned set.
func buildSignedManifest(t *testing.T, rootKeyID, rootPub string, rootPriv ed25519.PrivateKey, workingKey Key) []byte {
	t.Helper()
	keysJSON, _ := json.Marshal([]Key{workingKey})
	var keysAny any
	_ = json.Unmarshal(keysJSON, &keysAny)

	issuedAt := "2026-07-06T00:00:00Z"
	signed, err := canonicalJSON(map[string]any{"issuedAt": issuedAt, "keys": keysAny})
	if err != nil {
		t.Fatal(err)
	}
	sig := sign(rootPriv, signed)

	full := map[string]any{
		"keys":     keysAny,
		"issuedAt": issuedAt,
		"sig":      map[string]any{"rootKeyid": rootKeyID, "alg": "ed25519", "sig": sig},
	}
	raw, _ := json.Marshal(full)
	return raw
}

func withPinnedRoot(t *testing.T, rk RootKey) {
	t.Helper()
	orig := pinnedRootKeys
	pinnedRootKeys = append([]RootKey{rk}, orig...)
	t.Cleanup(func() { pinnedRootKeys = orig })
}

func TestParseAndVerifyManifestHappyPath(t *testing.T) {
	rootPub, rootPriv := testKey(0x55)
	withPinnedRoot(t, RootKey{KeyID: "root-x", Pub: rootPub})
	wkPub, _ := testKey(0x56)
	wk := Key{KeyID: "wk-1", Alg: "ed25519", Pub: wkPub,
		ValidFrom: "2026-07-01T00:00:00Z", ValidTo: "2027-07-01T00:00:00Z"}

	raw := buildSignedManifest(t, "root-x", rootPub, rootPriv, wk)
	m, err := ParseAndVerifyManifest(raw)
	if err != nil {
		t.Fatalf("expected manifest to verify, got %v", err)
	}
	if _, ok := m.KeyByID("wk-1"); !ok {
		t.Fatal("working key missing after parse")
	}
}

func TestParseAndVerifyManifestUntrustedRoot(t *testing.T) {
	rootPub, rootPriv := testKey(0x66)
	// Do NOT pin this root.
	wkPub, _ := testKey(0x67)
	wk := Key{KeyID: "wk", Alg: "ed25519", Pub: wkPub,
		ValidFrom: "2026-07-01T00:00:00Z", ValidTo: "2027-07-01T00:00:00Z"}
	raw := buildSignedManifest(t, "root-unpinned", rootPub, rootPriv, wk)
	if _, err := ParseAndVerifyManifest(raw); !errors.Is(err, ErrManifestUnverified) {
		t.Fatalf("expected ErrManifestUnverified, got %v", err)
	}
}

func TestParseAndVerifyManifestTampered(t *testing.T) {
	rootPub, rootPriv := testKey(0x77)
	withPinnedRoot(t, RootKey{KeyID: "root-y", Pub: rootPub})
	wkPub, _ := testKey(0x78)
	wk := Key{KeyID: "wk", Alg: "ed25519", Pub: wkPub,
		ValidFrom: "2026-07-01T00:00:00Z", ValidTo: "2027-07-01T00:00:00Z"}
	raw := buildSignedManifest(t, "root-y", rootPub, rootPriv, wk)

	// Tamper with issuedAt after signing.
	var full map[string]any
	_ = json.Unmarshal(raw, &full)
	full["issuedAt"] = "2099-01-01T00:00:00Z"
	tampered, _ := json.Marshal(full)

	if _, err := ParseAndVerifyManifest(tampered); !errors.Is(err, ErrManifestUnverified) {
		t.Fatalf("expected ErrManifestUnverified for tampered manifest, got %v", err)
	}
}

// TestPinnedRootDecodes guards against a malformed pinned production/test root
// key (wrong length / bad base64).
func TestPinnedRootDecodes(t *testing.T) {
	for _, rk := range pinnedRootKeys {
		if _, err := decodeRawPub(rk.Pub); err != nil {
			t.Errorf("pinned root %q does not decode: %v", rk.KeyID, err)
		}
	}
}

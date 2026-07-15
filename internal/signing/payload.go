package signing

import (
	"bytes"
	"fmt"
	"math"
	"sort"
	"strconv"
)

// Payload is the artifact record the registry signs. It is constructed
// identically on the server (at signing) and here (at verify) from the artifact
// read-model, then serialised with RFC 8785 (JCS) canonicalization to produce a
// deterministic byte sequence that Ed25519 signs directly.
//
// The field set and its JSON shape are fixed by specs/package-signing.md:
//
//	{"artifact":{"name","version","variant","sha256","size","uploaded_at","uploader"}}
//
// Every field is a string except Size, which is an integer — this deliberately
// sidesteps JCS's floating-point number-formatting pitfalls (see the spec's JCS
// parity guardrails).
type Payload struct {
	Name       string
	Version    string
	Variant    string
	SHA256     string
	Size       int64
	UploadedAt string // verbatim as returned by the registry — do NOT reformat
	Uploader   string
}

// Canonical returns the RFC 8785 (JCS) canonical byte serialisation of the
// payload, which is exactly the byte sequence the registry signed. The signing
// input is these bytes themselves — Ed25519 hashes internally, so there is no
// separate pre-hash step.
func (p Payload) Canonical() []byte {
	artifact := map[string]any{
		"name":        p.Name,
		"version":     p.Version,
		"variant":     p.Variant,
		"sha256":      p.SHA256,
		"size":        p.Size,
		"uploaded_at": p.UploadedAt,
		"uploader":    p.Uploader,
	}
	b, _ := canonicalJSON(map[string]any{"artifact": artifact})
	return b
}

// canonicalJSON produces the RFC 8785 (JCS) canonical serialisation of a value
// tree of the shapes fglpkg signs and verifies: objects, arrays, strings, and
// integers. It is byte-identical to the reference implementations used on both
// sides — ECMAScript's canonicalize (Worker) and gowebpki/jcs (documented in
// the spec) — for this constrained alphabet.
//
// It intentionally rejects non-integer numbers: no fglpkg signed payload
// contains a float, and float formatting is the one place independent JCS
// implementations most often diverge.
func canonicalJSON(v any) ([]byte, error) {
	var b bytes.Buffer
	if err := writeCanonical(&b, v); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func writeCanonical(b *bytes.Buffer, v any) error {
	switch t := v.(type) {
	case nil:
		b.WriteString("null")
	case bool:
		if t {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
	case string:
		writeJSONString(b, t)
	case int:
		b.WriteString(strconv.FormatInt(int64(t), 10))
	case int64:
		b.WriteString(strconv.FormatInt(t, 10))
	case float64:
		// JSON numbers decode to float64. Accept only integral values and
		// emit them as integers, matching String(int) on the reference side.
		if math.IsInf(t, 0) || math.IsNaN(t) || t != math.Trunc(t) {
			return fmt.Errorf("canonical JSON: non-integer number %v is not supported", t)
		}
		b.WriteString(strconv.FormatInt(int64(t), 10))
	case []any:
		b.WriteByte('[')
		for i, e := range t {
			if i > 0 {
				b.WriteByte(',')
			}
			if err := writeCanonical(b, e); err != nil {
				return err
			}
		}
		b.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		// JCS sorts object keys by UTF-16 code unit; for the ASCII keys used
		// here that is identical to Go's byte-wise string ordering.
		sort.Strings(keys)
		b.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				b.WriteByte(',')
			}
			writeJSONString(b, k)
			b.WriteByte(':')
			if err := writeCanonical(b, t[k]); err != nil {
				return err
			}
		}
		b.WriteByte('}')
	default:
		return fmt.Errorf("canonical JSON: unsupported type %T", v)
	}
	return nil
}

// writeJSONString writes s as a JSON string literal using the same escaping as
// ECMAScript JSON.stringify (which RFC 8785 mandates): escape the two required
// characters and the short control-character forms, emit remaining control
// characters as \u00XX, and pass everything else through verbatim.
func writeJSONString(b *bytes.Buffer, s string) {
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 {
				fmt.Fprintf(b, `\u%04x`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
}

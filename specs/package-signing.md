# Spec: Package signing & provenance (v1)

**Status:** Draft
**Date:** 2026-07-02
**Author:** Mike Folcher
**Tracking:** Workstream C, item #6 (P0) in [docs/outstanding-work.md](../docs/outstanding-work.md); market-readiness gap #6.

---

## Summary

Add two independent authenticity layers on top of the existing SHA256 integrity check:

1. **Layer 1 — registry-signed artifacts.** The Genero Intelligence (GI) registry generates an Ed25519 signing keypair, signs every artifact at publish time over a canonical payload, and exposes its public keys via a signed keys manifest. `fglpkg install` verifies the signature by default. This proves "the registry served you what it stored" — it defends against tarball tampering in transit, at rest, and by an MITM.
2. **Layer 2 — Sigstore provenance.** Publishers running in a supported CI environment can attach a Sigstore attestation to a published artifact (`fglpkg publish --provenance`). Consumers can require attestations on install (`fglpkg install --require-provenance`). This proves "this artifact was built by this workflow, from this source repo, at this commit" — it defends against a compromised publisher account.

The two layers ship in that order, in separate PRs, and are independently useful. Layer 1 is on-by-default; Layer 2 is opt-in on both publish and consume sides.

## Motivation

fglpkg already verifies a SHA256 digest supplied by the registry against the actual bytes streamed from disk on install ([internal/checksum](../internal/checksum)). That defends against a corrupted or truncated download but not against a maliciously altered one where the registry's stated digest was also swapped. The threat model gap is:

- **Compromised transport / mirror / cache:** a rogue proxy or a compromised cache node hands the client a tampered zip plus a matching digest. Registry-signed artifacts (Layer 1) close this.
- **Compromised publisher credentials:** a stolen token uploads malware to an existing legitimate package. Sigstore provenance (Layer 2) closes this by tying artifacts to the CI build identity, not to the token.
- **Insider threat / registry-side tampering:** a rogue registry operator retroactively swaps an artifact. Only Sigstore (Rekor's public transparency log) closes this. Layer 1 alone doesn't.

Layer 1 delivers most of the value with near-zero publisher friction. Layer 2 delivers the strongest supply-chain guarantee for publishers who care.

## Goals

- Every artifact served by the GI registry carries a verifiable Ed25519 signature over a canonical payload including name, version, variant, sha256, size, upload time, and uploader partner_id.
- `fglpkg install` verifies signatures by default; a signature mismatch aborts before extraction with a `signing.ErrSignatureMismatch` parallel to the existing checksum error.
- The registry's active signing keys are discoverable via a signed manifest at `GET /registry/.well-known/keys.json`, verifiable against a root key pinned in the CLI binary.
- Key rotation is a single-key-swap operation; the CLI transparently accepts any key currently in the manifest.
- `fglpkg audit signatures` walks the lockfile and re-verifies every entry.
- Publishers running in CI can enrich a publish with a Sigstore attestation (`fglpkg publish --provenance`). Supported CI providers are anything sigstore-go's identity providers recognize (GitHub Actions, GitLab CI, Buildkite, CircleCI, GCP Cloud Build, etc.).
- Consumers can require attestations on install per-package via manifest, per-invocation via `--require-provenance`, or per-registry-package via a `require_provenance` toggle set by the package owner.
- Provenance verification uses only the public-good Sigstore instance (`fulcio.sigstore.dev` / `rekor.sigstore.dev`) — no support for private Sigstore instances in v1.

## Non-goals (v1)

- **Author-side long-term keys (PGP-style detached signatures).** Every ecosystem that tried this walked it back. Registry-signed + Sigstore-provenance covers the same threat model with dramatically less publisher friction.
- **A TUF-style multi-role trust hierarchy.** The root/rotation key + working-keys distinction is TUF-adjacent but deliberately not TUF; we accept the simpler model and its rotation semantics.
- **Custom trust roots for a private Sigstore instance.** Air-gapped customers keep Layer 1 (registry-signed) but forgo Layer 2. Revisit if a customer specifically asks.
- **Offline install verification of Sigstore attestations.** Layer 1 works offline against the cached keys manifest and the lockfile-recorded signature. Layer 2 requires a live Rekor inclusion-proof check; offline provenance verification is deferred.
- **Attestation revocation.** Rekor entries are append-only and Sigstore has no revocation channel. If an attestation needs to be repudiated, the package version is unpublished; consumers see a `pending`-status version disappear.
- **Signing of Java JARs by the GI registry.** JARs pulled from Maven Central have their own transitive trust chain (checksums today; the manifest supports a per-JAR `checksum` field). Signing GI-hosted JARs is possible under Layer 1 but is out of scope for v1 — the JAR install path just carries the existing checksum verify.

## Design decisions (locked)

| Decision | Choice | Rationale |
|---|---|---|
| Signing algorithm | **Ed25519** | ~64-byte signatures, stdlib support, no r/s encoding surprises. |
| Root key custody | **Cloudflare KMS / Secrets Store** | Fully cloud-native ops; can be strengthened later without wire-protocol change. |
| CI platforms in Layer 2 v1 | **All sigstore-go supported providers** | GitHub Actions, GitLab, Buildkite, CircleCI, GCP Cloud Build. Env detection + OIDC fetch is a thin per-provider adapter. |
| Laptop publish when package requires provenance | **Blocked unless `--no-provenance --i-accept-risk` is passed** | Middle ground: preserves emergency-fix path but records the operator's conscious override in the audit log. |
| Sigstore trust root | **Hardcoded public-good instance** | No pluggable trust root in v1. |
| Canonical JSON | **RFC 8785 (JSON Canonicalization Scheme)** | Deterministic bytes for signing; well-specified; existing Go libraries. |

## Layer 1 — Registry-signed artifacts

### Wire format

**Canonical signing payload.** Constructed identically on server (at signing) and client (at verify) from the artifact record:

```json
{
  "artifact": {
    "name": "chart-3d",
    "version": "1.0.0",
    "variant": "genero6",
    "sha256": "b6e1…",
    "size": 87477,
    "uploaded_at": "2026-07-02T14:22:00Z",
    "uploader": "partner:pt_7f2a…"
  }
}
```

The payload is serialised with **RFC 8785 JSON Canonicalization** to produce a deterministic byte sequence, then signed with Ed25519. The signing input is the RFC-8785 output — not a hash of it. Ed25519 hashes internally.

**Signature envelope** (as returned in responses and stored in the lockfile):

```json
{
  "keyid": "fglpkg-gi-2026-1",
  "alg": "ed25519",
  "sig": "<base64 raw 64-byte signature>",
  "canonicalized_payload_sha256": "<hex>"
}
```

The `canonicalized_payload_sha256` is a courtesy debugging field — verification uses the payload the client reconstructs, not this value.

**Signed keys manifest** at `GET /registry/.well-known/keys.json`:

```json
{
  "keys": [
    {
      "keyid": "fglpkg-gi-2026-1",
      "alg": "ed25519",
      "pub": "<base64 32-byte pubkey>",
      "validFrom": "2026-07-01T00:00:00Z",
      "validTo":   "2027-07-01T00:00:00Z"
    }
  ],
  "issuedAt": "2026-07-02T00:00:00Z",
  "sig": {
    "rootKeyid": "fglpkg-gi-root-1",
    "alg": "ed25519",
    "sig": "<base64>"
  }
}
```

The signature covers the RFC-8785 canonicalization of `{keys, issuedAt}`. The **root key** is pinned in the CLI binary. Working keys are rotated by issuing a new keys.json signed by the root key; the CLI accepts any key in the current manifest whose validity window covers the artifact's `uploaded_at`.

### GI service — new & changed

#### New: KMS integration

- Root private key held in Cloudflare Secrets Store, tagged `fglpkg-root`. Access via Workers binding, restricted to the "keys manifest signer" endpoint only.
- Working private key held in Cloudflare Secrets Store, tagged `fglpkg-signing-<year>-<n>`. Access via Workers binding, restricted to the artifact-upload path.
- A small `internal/signing/` module in the GI Worker abstracts sign/verify + key resolution.

#### New: `GET /registry/.well-known/keys.json`

- Returns the current signed keys manifest (schema above).
- Cache headers: `Cache-Control: public, max-age=3600` (working keys), `s-maxage=300` on the edge.
- Regenerated whenever a key is added, retired, or the root key rotates — not on every request.

#### Changed: `PUT /registry/packages/:slug/versions/:version/artifacts/:variant`

After the existing size/sha256 computation, before storing the DB row:

1. Load current working private key from Secrets Store.
2. Build the canonical payload from the artifact record (name/version/variant/sha256/size/uploaded_at/uploader).
3. Sign with Ed25519.
4. Store `signature.keyid`, `signature.sig` in a new `registry_artifacts.signature` JSON column.

Failure to sign is a 500 — the artifact must not land in the DB unsigned.

#### Changed: artifact reads

Extend the artifact record everywhere it's returned (info, resolve, version-list) to include:

```json
"signature": {
  "keyid": "fglpkg-gi-2026-1",
  "alg": "ed25519",
  "sig": "<base64>"
}
```

#### New: backfill script

A one-shot Worker script that signs every historical artifact. Records `signed_at` = "now" (not `uploaded_at`) so the audit trail is clear about which signatures are historical backfill vs. at-publish.

#### New: schema additions

- `registry_artifacts.signature JSON NOT NULL` (backfilled first, NOT NULL constraint applied after)
- `registry_signing_keys(keyid PK, alg, pub_pem, valid_from, valid_to, retired_at)`
- `registry_keys_manifests(issued_at PK, body, sig)` — stores each issued keys.json for audit.

### fglpkg CLI — new & changed

#### New: `internal/signing/`

```
internal/signing/
    keys.go            // Ed25519 verify wrapper
    payload.go         // Canonical payload builder (RFC 8785 JCS)
    root.go            // Pinned root public key(s), embedded at build time
    manifest.go        // Fetch, cache, verify /registry/.well-known/keys.json
    errors.go          // ErrSignatureMismatch, ErrKeyUnknown, ErrKeyExpired
```

`payload.go` is authoritative — the GI Worker imports the same JCS logic (or a fixture-tested equivalent). Add a golden-vectors test in this repo that both sides verify against.

`root.go` embeds the root public key via a Go string constant. Rotating the root key requires a CLI release. This is deliberate — the root key is high-value and rotates rarely.

#### New: keys cache

Cached at `~/.fglpkg/keys.json` with the raw signed manifest bytes. On CLI start (or first verify), if the cache is older than the manifest's `Cache-Control` or missing, refetch, verify against the pinned root, replace. Never write an unverifiable manifest to the cache.

#### Changed: install download path

In [internal/installer/installer.go](../internal/installer/installer.go), immediately after the existing `checksum.NewDigestingReader.Verify` call:

```go
if err := signing.Verify(art, keysManifest); err != nil {
    return err  // signing.ErrSignatureMismatch etc.
}
```

`art` here carries the resolved artifact record including `signature`. If `signing.enforce = warn`, log and continue; if `require` (default from v1.1 onward), abort.

#### Changed: lockfile

Add to `PackageEntry`:

```go
Signature      string `json:"signature,omitempty"`       // base64
SignatureKeyID string `json:"signature_keyid,omitempty"`
```

Reinstalls from lockfile re-verify these against the cached keys manifest — no network call needed for the signature check (only for the keys.json refresh, and only on TTL expiry).

#### New: `fglpkg audit signatures`

Walks the lockfile, re-verifies every entry against the current keys manifest, prints one line per package:

```
✓ chart-3d@1.0.0 (genero6)      keyid=fglpkg-gi-2026-1
✓ odatalib@0.4.2 (genero6)      keyid=fglpkg-gi-2026-1
✗ legacy-pkg@0.1.0 (genero6)    ERROR: signature missing
```

Exit non-zero if anything is missing or mismatched, for CI use.

#### New: CLI flags & config

- `fglpkg install --no-verify-signature` — escape hatch, documented and discouraged.
- `~/.fglpkg/config.json` field `signing.enforce`: `"require" | "warn" | "off"`. Default `"warn"` in v1.0 (transition), `"require"` in v1.1.
- Env override: `FGLPKG_SIGNING=off` (mirrors the pattern of other env overrides).

#### Rollout gating

- v1.0 (Layer 1): CLI defaults to `signing.enforce = warn`. Missing/mismatched signatures emit a `Warning:` line but don't fail. This gives the backfill time to complete and any legacy consumers time to upgrade.
- v1.1 (Layer 1 GA): CLI flips default to `require`. A missing signature aborts install. Ship this one release cycle after the backfill is verified complete.

#### Docs

- README section "Signature verification" — what it does, what it doesn't do, how to disable in emergencies.
- Note in the publish doc that the registry signs automatically — publishers do nothing.

## Layer 2 — Sigstore provenance

### Wire format

Attestations are stored as **Sigstore bundles** (`application/vnd.dev.sigstore.bundle+json`, defined by the sigstore-go project). A bundle contains:

- The DSSE (Dead Simple Signing Envelope) with the in-toto Statement.
- The Fulcio-issued short-lived X.509 certificate the signer used.
- The Rekor inclusion proof.

The in-toto Statement carries a **SLSA v1 Provenance predicate**:

```json
{
  "_type": "https://in-toto.io/Statement/v1",
  "subject": [{
    "name": "chart-3d-1.0.0-genero6.zip",
    "digest": {"sha256": "b6e1…"}
  }],
  "predicateType": "https://slsa.dev/provenance/v1",
  "predicate": {
    "buildDefinition": {
      "buildType": "https://slsa-framework.github.io/github-actions-buildtypes/workflow/v1",
      "externalParameters": {
        "workflow": {
          "ref": "refs/tags/v1.0.0",
          "repository": "https://github.com/4js-mikefolcher/chart-3d",
          "path": ".github/workflows/publish.yml"
        }
      }
    },
    "runDetails": {
      "builder": {"id": "https://github.com/actions"},
      "metadata": {"invocationId": "12345"}
    }
  }
}
```

### GI service — new & changed

#### New: `PUT /registry/packages/:slug/versions/:version/attestations/:variant`

- Body: `application/vnd.dev.sigstore.bundle+json` (the raw bundle bytes; typically 2–8 KB).
- Auth: same OAuth bearer as artifact upload.
- Server-side verification **before storing**:
  1. Parse the bundle via sigstore-go's `verify.Verifier`.
  2. Verify Rekor inclusion proof (log integrity).
  3. Verify Fulcio cert chain against the pinned Sigstore trust root (bundled with GI Worker at deploy time).
  4. Extract the DSSE payload's subject digest, confirm it equals the artifact record's stored sha256.
  5. Extract the signer identity claims (issuer, subject, e.g. GitHub Actions repo + workflow ref).
- On failure: 400 with a descriptive error. Don't store partial state.
- On success: store the bundle in R2 keyed as `<slug>/<version>/<variant>.sigstore.json` + a row in `registry_attestations` with parsed identity fields for query.

#### New: `GET /registry/packages/:slug/versions/:version/attestations/:variant`

Returns the stored bundle bytes. Public — no auth required (attestations are public regardless of package visibility, matching Rekor's public log).

#### Changed: artifact reads

Extend each artifact record with:

```json
"attestation": {
  "present": true,
  "issuer": "https://token.actions.githubusercontent.com",
  "subject": "https://github.com/4js-mikefolcher/chart-3d/.github/workflows/publish.yml@refs/tags/v1.0.0",
  "rekorEntryUUID": "24296fb2…"
}
```

`present: false` when no attestation has been uploaded.

#### New: per-package policy

- New column `registry_packages.require_provenance BOOLEAN NOT NULL DEFAULT FALSE`.
- New endpoint `PATCH /registry/packages/:slug { "require_provenance": true }`. Owner-only, audit-logged.
- Enforced at `POST /registry/packages/:slug/versions/:version/submit`: if `require_provenance = true` and any artifact under this version has no attestation, submit returns 409 with a message pointing at the missing variants.

#### New: schema additions

- `registry_packages.require_provenance BOOLEAN NOT NULL DEFAULT FALSE`
- `registry_attestations(id PK, slug, version, variant, bundle_r2_key, issuer, subject, rekor_uuid, created_at)`

### fglpkg CLI — new & changed

#### New: `internal/attest/`

```
internal/attest/
    ci.go              // CI provider detection & OIDC token fetch
    fulcio.go          // Fulcio client (thin wrapper on sigstore-go)
    rekor.go           // Rekor submission (thin wrapper on sigstore-go)
    dsse.go            // In-toto Statement + SLSA v1 Provenance predicate builder
    verify.go          // Consumer-side verification pipeline
    errors.go          // ErrNoOIDCToken, ErrCIUnknown, ErrProvenanceMissing, etc.
```

The core cryptographic operations use `github.com/sigstore/sigstore-go` — this is not code we're writing from scratch. Our surface is:
- **CI adapters** (`ci.go`): detect provider from env, fetch OIDC token with `sigstore` audience.
- **Publish orchestration**: OIDC → Fulcio → sign → Rekor → build bundle → upload.
- **Consumer orchestration**: fetch bundle → verify with sigstore-go → apply identity policy → report.

#### CI provider detection

`ci.DetectProvider()` returns a `Provider` interface with `OIDCToken(ctx)` and a display name. Providers implemented in v1:

| Provider | Env sentinel | OIDC token source |
|---|---|---|
| GitHub Actions | `GITHUB_ACTIONS=true` | `curl $ACTIONS_ID_TOKEN_REQUEST_URL&audience=sigstore` with `ACTIONS_ID_TOKEN_REQUEST_TOKEN` |
| GitLab CI | `GITLAB_CI=true` | `$SIGSTORE_ID_TOKEN` (project must enable ID tokens with audience `sigstore`) |
| Buildkite | `BUILDKITE=true` | `buildkite-agent oidc request-token --audience sigstore` |
| CircleCI | `CIRCLECI=true` | `$CIRCLE_OIDC_TOKEN` (project must set audience via context) |
| GCP Cloud Build | `CLOUD_BUILD_ID` set | Metadata server: `GET .../instance/service-accounts/default/identity?audience=sigstore` |

If none match, `ErrCIUnknown`. `fglpkg publish --provenance` from an unknown CI (or a laptop) fails unless `--no-provenance --i-accept-risk` is passed for a package that requires provenance (see below).

#### Changed: `fglpkg publish`

New flags:
- `--provenance` — attach a Sigstore attestation. Requires a supported CI env.
- `--no-provenance` — explicitly opt out. Silently succeeds today; interacts with `--i-accept-risk` below when the package requires provenance.
- `--i-accept-risk` — required companion to `--no-provenance` when the package (or the registry's per-package policy) requires provenance. Records the override in the CI publish log.

Publish orchestration when `--provenance`:

1. Detect CI provider, fetch OIDC token.
2. Build the zip and compute sha256 (already happens today).
3. Upload the artifact (existing PUT).
4. Fulcio: submit OIDC token + ephemeral pubkey, get short-lived cert.
5. Build the in-toto Statement + SLSA v1 Provenance predicate (subject = artifact filename + sha256).
6. Sign the DSSE payload with the ephemeral private key.
7. Rekor: submit the signed DSSE + cert, get inclusion proof.
8. Bundle into a Sigstore bundle.
9. PUT the bundle to `/registry/…/attestations/:variant`.
10. Print the Rekor entry URL for user reference.

Failure at any step is non-fatal to the artifact upload (it landed in step 3) but fatal to the publish overall — the version is left in draft state and can be resubmitted.

#### Changed: `fglpkg install`

New flags:
- `--require-provenance` — fail install if any resolved package/variant lacks a verified attestation.
- `--verify-provenance=<mode>` — `off | warn | require` (mirrors `signing.enforce` semantics).

Consumer manifest declaration:

```json
{
  "signing": {
    "requireProvenance": ["chart-3d", "another-critical-pkg"]
  }
}
```

Packages listed here are treated as if `--require-provenance` had been passed for them individually.

Consumer verification pipeline (per artifact, when required or `warn` non-off):

1. GET `/registry/…/attestations/:variant` — if 404 and provenance required, fail.
2. Verify the bundle end-to-end via sigstore-go against the pinned public Sigstore trust root.
3. Confirm the DSSE subject digest equals the artifact's sha256 (which we already verified against the tarball).
4. Extract signer identity (issuer + subject).
5. Apply the identity policy: **default** is to accept any identity whose subject repo URL matches the package's `repository` field in the resolved manifest. Fancier policies (allowlisted workflows, tag/branch constraints) are per-package config in a future revision.

#### Changed: `fglpkg info`

When an attestation is present, print an additional block:

```
Provenance:  verified
  Issuer:    https://token.actions.githubusercontent.com
  Subject:   .github/workflows/publish.yml@refs/tags/v1.0.0
  Repo:      https://github.com/4js-mikefolcher/chart-3d
  Rekor:     https://search.sigstore.dev/?logIndex=42007123
```

#### Docs

A new `docs/provenance.md`:
- Concept: what Sigstore/Fulcio/Rekor are.
- Publisher setup: sample `.github/workflows/publish.yml`, `.gitlab-ci.yml`, `.buildkite/pipeline.yml`, CircleCI config with OIDC context, GCP Cloud Build config.
- Consumer setup: how to opt in per-package via manifest; how to force-verify a whole tree.
- Troubleshooting: common OIDC / identity-mismatch failures.

## Data-flow summary

### Publish flow

```
                   ┌─────────────────────────────────────────────┐
publisher (CLI)    │  1. pack → sha256                            │
                   │  2. PUT artifact  ──────────► GI: sign, store │
                   │                                signature      │
                   │  3. (if --provenance in CI)                   │
                   │     OIDC → Fulcio → sign → Rekor → bundle     │
                   │     PUT attestation ────────► GI: verify,     │
                   │                                store bundle   │
                   │  4. POST submit  ────────────► GI: enforce    │
                   │                                require_prov   │
                   └─────────────────────────────────────────────┘
```

### Install flow

```
                   ┌─────────────────────────────────────────────┐
consumer (CLI)     │  1. resolve ─────────────► GI: return meta    │
                   │     incl. signature + attestation.present    │
                   │  2. keys.json cache miss? ─► GI: keys.json    │
                   │     verify root sig                          │
                   │  3. download artifact ───► sha256 verify      │
                   │  4. signing.Verify(art, keys)                 │
                   │  5. (if --require-provenance)                 │
                   │     GET attestation, sigstore-go verify,      │
                   │     identity policy check                     │
                   │  6. extract                                   │
                   └─────────────────────────────────────────────┘
```

## Error taxonomy

| Error | Where raised | User-facing message shape |
|---|---|---|
| `signing.ErrSignatureMismatch` | Install, audit | `signature mismatch for chart-3d@1.0.0 (genero6): expected keyid <a> matches, but signature does not verify — may be tampered with, delete and retry or contact package author` |
| `signing.ErrKeyUnknown` | Install, audit | `signature for chart-3d@1.0.0 was signed by keyid <x>, not in current keys manifest — run 'fglpkg update' or upgrade the CLI` |
| `signing.ErrKeyExpired` | Install, audit | `signature for chart-3d@1.0.0 uses expired key <x> (retired <date>) — package may be legitimate but should be republished` |
| `signing.ErrManifestUnverified` | Keys fetch | `could not verify /registry/.well-known/keys.json against the pinned root key — refusing to trust unsigned key material` |
| `attest.ErrNoOIDCToken` | Publish --provenance | `--provenance requires an OIDC-issuing CI environment; none detected — run from CI, or pass --no-provenance` |
| `attest.ErrCIUnknown` | Publish --provenance | `--provenance: unsupported CI environment; supported: GitHub Actions, GitLab, Buildkite, CircleCI, GCP Cloud Build` |
| `attest.ErrProvenanceMissing` | Install --require-provenance | `chart-3d@1.0.0 has no attestation but --require-provenance was set` |
| `attest.ErrProvenanceMismatch` | Install --require-provenance | `chart-3d@1.0.0 attestation subject does not match declared repository <url>` |
| `attest.ErrBundleVerification` | Install --require-provenance | `chart-3d@1.0.0 Sigstore bundle verification failed: <sigstore-go error>` |
| `publish.ErrProvenanceRequired` | Publish --no-provenance without --i-accept-risk | `chart-3d requires provenance but --no-provenance was passed without --i-accept-risk` |

## Acceptance milestones

**M1 — Layer 1 registry (~1 week)**
- Cloudflare Secrets Store bindings wired; root + working keys generated.
- `POST/PUT` to artifacts signs and stores.
- `GET /registry/.well-known/keys.json` returns a manifest that verifies against the root key.
- Artifact JSON reads include the signature.
- Backfill script has signed every historical artifact; DB has NOT NULL constraint on `signature`.

**M2 — Layer 1 CLI (~1 week)**
- `internal/signing/` package with golden-vectors test suite (shared with the GI-side JCS impl).
- Keys manifest fetched, verified against pinned root, cached.
- `fglpkg install` verifies signatures; hand-editing a byte in a served zip triggers `ErrSignatureMismatch`.
- Lockfile round-trips `signature` + `signature_keyid`.
- `fglpkg audit signatures` prints a per-package report and exits non-zero on any failure.
- Ships with `signing.enforce = warn` default.

**M2b — Layer 1 GA (~2 weeks after M2)**
- CLI default flipped to `signing.enforce = require`.
- Documentation and release notes updated.

**M3 — Layer 2 registry (~2 weeks)**
- Attestation PUT/GET endpoints.
- Server-side sigstore-go verify at receive time (Fulcio chain, Rekor inclusion, subject digest match).
- `require_provenance` toggle + submit-time enforcement.
- Artifact JSON includes parsed attestation identity fields.

**M4 — Layer 2 CLI (~3 weeks)**
- `internal/attest/` package with per-provider CI adapters (GH, GitLab, Buildkite, CircleCI, GCP).
- `fglpkg publish --provenance` end-to-end from a real GitHub Actions run: attestation attached, `fglpkg info` shows it, Rekor entry visible in the public log.
- `fglpkg install --require-provenance` passes on the legit build, fails on a hand-edited attestation.
- Consumer manifest `signing.requireProvenance` list is honored.
- `--no-provenance --i-accept-risk` records the override in the CI publish log (audited server-side).
- `docs/provenance.md` covers all supported CI providers.

## Testing strategy

**Unit tests**
- `internal/signing/payload.go` — golden JCS output vectors (shared with GI).
- `internal/signing/manifest.go` — root-key verify happy path + all failure modes (tampered signature, unknown root, expired working key, etc.).
- `internal/attest/ci.go` — one table-driven test per provider with env-var setups.
- `internal/attest/verify.go` — fixture bundles for pass/tampered/subject-mismatch/expired-cert/no-rekor-inclusion.

**Integration tests**
- Layer 1: mock registry serves signed + unsigned + tampered fixtures; installer accepts, warns, aborts respectively.
- Layer 2: a fixture Sigstore bundle (captured from a real staging run and checked into `testdata/`) verifies end-to-end; a modified bundle fails.

**End-to-end (manual, pre-release)**
- Publish `chart-3d@X` from staging GH Actions with `--provenance`; verify from a local install; hand-edit the attestation bundle in R2, observe consumer failure.

## Open questions (post-v1 backlog)

- **Multi-signer identity policy.** Today the consumer verifies against the package's `repository` field only. A future revision might let package owners declare `signing.trustedIdentities: [{repo, workflow, branch}]` for tighter control.
- **Java JAR signing.** GI-hosted JARs could ride the same Layer 1 signing infrastructure; Maven-Central JARs cannot. Deferred until the JAR-hosting story stabilizes.
- **Attestations for BDL package tarballs from mirrors.** If we introduce read-only mirrors, they need to serve the same signatures + attestations — no re-signing at the mirror.
- **Offline attestation verification.** Would need Rekor inclusion-proof caching in the lockfile. Not for v1.
- **Publisher-held long-term keys.** Explicitly out of scope; revisit only if a customer with an existing PKI investment specifically asks.

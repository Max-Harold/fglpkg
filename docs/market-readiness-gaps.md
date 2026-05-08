# fglpkg Market Readiness: Gap Analysis vs. npm / gem / maven

**Document Version:** 1.1
**Date:** April 22, 2026
**Purpose:** Catalogue the features fglpkg needs to reach parity with mainstream package managers (npm, RubyGems/Bundler, Maven), sequenced for a phased PS → enterprise → public rollout.

## Purpose

This document is a sibling to [fglpkg-enhancement-roadmap.md](fglpkg-enhancement-roadmap.md). The roadmap answers *"what should Professional Services build to prove internal value?"* This doc answers *"what does fglpkg need to be taken seriously as a package manager?"* The two should be read together: the roadmap sets strategy and budget; this doc sets the feature bar against which fglpkg will be judged by developers coming from npm, gem, or Maven.

The analysis assumes a phased market entry — PS internal first, then external enterprise customers, then (optionally) a public Genero developer community. **"Market-ready" is defined as the start of Phase 2 (external enterprise customer entry)**; Phase 1 (PS internal) is an earlier, lower-bar milestone. Priorities map onto this timeline: **P0 items must land before Phase 2 begins**; P1 during Phase 2; P2/P3 into Phase 3 and beyond.

---

## Current State

fglpkg is architecturally mature with strong foundations.

**Commands already implemented**: `init`, `install`, `remove`, `update`, `list`, `search`, `publish`, `unpublish`, `login`, `logout`, `whoami`, `env` (with `--local`/`--global`/`--gst`), `bdl` (with `--list`), `run` (with `--list`), `docs`, `workspace`, `owner`, `token`, `config github-repos`, `version`, `help`.

**Manifest fields**: `name`, `version`, `description`, `author`, `license`, `repository`, `main`, `genero`, `root`, `files`, `bin`, `docs`, `programs`, `scripts` (defined but **not executed**), `dependencies.fgl`, `dependencies.java`.

**Differentiators vs. mainstream**: Genero version variants (one artifact per major), monorepo workspaces with local member linking, Java JAR resolution from Maven Central, GitHub Releases as blob backend with Fly.io metadata registry, context-aware local/global scope.

**Test coverage**: 12 internal packages have `_test.go` coverage (resolver, installer, registry server, semver, lockfile, credentials, manifest, cli, workspace, genero, checksum). Coverage breadth is good but there is currently **no CI gate** preventing broken tests from landing on `main` (see P0 #8 below); quality improvements are required before Phase 2.

---

## Feature Matrix

| Capability | fglpkg | npm | gem/bundler | maven | Priority |
|---|---|---|---|---|---|
| Install / remove / update | Yes | Yes | Yes | Yes | — |
| Semver constraints | Yes | Yes | Yes (~>) | Yes (ranges) | — |
| Lockfile | Yes | Yes | Yes | Yes | — |
| Search (CLI) | Yes | Yes | Yes | Yes | — |
| Workspaces / monorepo | Yes | Yes | No | Yes (reactor) | — |
| Package signing / verification (Sigstore) | **No** | Yes (Sigstore provenance) | Yes (x509) | Yes (GPG, required by Central) | **P0** |
| Web registry UI | **No** | Yes (npmjs.com) | Yes (rubygems.org) | Yes (search.maven.org) | **P0** |
| Dev / optional deps (no peer) | **No** (all prod) | Yes | Yes (groups) | Yes (scopes) | **P0** |
| Declarative lifecycle steps (no arbitrary shell) | Yes (`hooks` field, ops: `copy-files`, `mkdir`) | Yes (shell — being deprecated) | Yes | Yes (plugin goals) | **P0** |
| CI gate blocking merge on failing tests | **No** | Yes | Yes | Yes | **P0** |
| `outdated` command | **No** | Yes | Yes | Yes (versions plugin) | **P0** |
| `audit` (vulnerability check) | **No** | Yes | Yes (bundler-audit) | Yes (dependency-check) | **P0** |
| `version` bump command | **No** | Yes (patch/minor/major + tag) | Yes | Yes (release plugin) | **P0** |
| `pack` / `publish --dry-run` | **No** | Yes | Yes | Yes | **P1** |
| `deprecate` command | **No** | Yes | Yes (yank) | Yes | **P1** |
| `info` / `view` (metadata inspection) | **No** | Yes | Yes (info) | Yes | **P1** |
| Dist-tags / channels (latest/beta) | **No** | Yes | No | Yes (SNAPSHOT) | **P1** |
| Organizations / scopes | **No** | Yes (@scope/pkg) | No | Yes (groupId) | **P1** |
| 2FA for publishing | **No** | Yes | Yes | Yes | **P1** |
| `.fglpkgignore` | Yes | Yes | — (gemspec explicit) | — | **P1** |
| Shell completions (bash/zsh/fish/ps) | **No** | Yes | Yes | Yes | **P2** |
| IDE / editor extension | **No** | Yes (many) | Yes | Yes (IntelliJ, Eclipse) | **P2** |
| JSON schema for manifest autocomplete | **No** | Yes (schemastore) | — | Yes (xsd) | **P2** |
| CI/CD helper (GitHub Action etc.) | Partial (release.yml for fglpkg itself) | Yes (setup-node) | Yes (setup-ruby) | Yes (setup-java) | **P2** |
| Self-hosted deploy (Docker/Helm) | **No** | Yes (Verdaccio) | Yes (Geminabox) | Yes (Nexus/Artifactory) | **P2** |
| Download stats / dependents graph | **No** | Yes | Yes | Yes | **P3** |
| `link` (local dev symlink) | Partial (workspace members only) | Yes | No | No | **P3** |
| Offline cache | **No** | Partial | No | Yes (local repo) | **P3** |
| LDAP/SAML/SSO | **No** | Enterprise | Enterprise | Enterprise (Nexus) | **P3 enterprise** |

---

## Gap List (Prioritized)

### P0 — Table stakes for "market-ready" (must-have)

These are features developers **assume exist** in a package manager. Their absence is a credibility issue.

1. **Dependency scopes (dev + optional)** — Add `devDependencies` and `optionalDependencies` to `fglpkg.json`. Peer dependencies are intentionally **out of scope** — they solve a JS/TS singleton problem (React, TypeScript) that has no clean analog in BDL's module layout. The resolver already tracks `RequiredBy`; extend to filter by scope. Without this, test-only tooling leaks into production installs.
2. **Declarative lifecycle steps (no arbitrary shell)** — Execute a declarative allow-list of operations on well-known events: `preinstall`, `postinstall`, `prepublish`, `postpublish`, `preuninstall`. Supported operations are a fixed vocabulary (e.g. `fetch-jar`, `compile-bdl`, `copy-files`, `emit-schema`) — **not** arbitrary shell commands. Rationale: npm's shell-based `scripts` field is the dominant supply-chain attack vector and npm itself is deprecating it; fglpkg starts from the safer position. The existing `scripts` field is defined but unimplemented; it will be replaced by a new `hooks` field with declarative semantics rather than reused.
3. **`fglpkg version <patch|minor|major|prerelease>`** — Bump version, update `fglpkg.json`, optionally create a git tag and commit. Currently manual, error-prone, and inconsistent with `fglpkg publish` expectations.
4. **`fglpkg outdated`** — Compare installed versions against latest registry versions. Required for maintenance workflows.
5. **`fglpkg audit`** — Cross-check Java JAR dependencies against CVE databases (NIST NVD, OSS Index, GitHub Advisory DB). BDL packages can use the same mechanism once an advisory data store exists. Without audit, enterprise security teams will block adoption.
6. **Package signing and verification (Sigstore)** — Publishers sign via Sigstore's keyless OIDC flow (GitHub Actions identity in Phase 1; broader OIDC providers later); the registry stores the attestation bundle; `fglpkg install` verifies the bundle against the transparency log. Rationale: keyless + short-lived certificates + transparency log avoids long-lived key distribution and CI-key-theft, and gives provenance (who/what/when built the artifact) on top of integrity. The lockfile already stores SHA256 for integrity — Sigstore adds authenticity. GPG is explicitly **not** chosen; the web-of-trust model does not fit a closed-ecosystem tool. See full design in a forthcoming `docs/signing-design.md`.
7. **Web registry UI** — Covered in the roadmap's Phase 1.1 and detailed in [`docs/web-registry-ui.md`](web-registry-ui.md). The MVP scope in that doc targets an initial group of 5–10 trusted users (no self-service signup yet — admin creates accounts). The full market-ready scope adds **self-service user registration** (signup, email verification, rate limiting, anti-abuse), download statistics, and the dependents graph, as an evolution path from the MVP. This is the single biggest adoption driver — you cannot "bring to market" a package manager whose registry has no web face, and you cannot have a public community without self-service signup.
8. **CI gate blocking merge on failing tests** — A GitHub Actions workflow runs `go test ./...` and `go build ./...` on every push and PR; branch protection requires the check to pass before merge. Rationale: baseline credibility — a package manager shipping with failing tests on `main` is not market-ready, regardless of other features. Pre-existing failures on `main` (lockfile, semver, registry/server compile error) demonstrated the need. Low effort; high signal. See `.github/workflows/release.yml` for the one existing workflow — the new `ci.yml` is a sibling.

### P1 — Expected by professional users (should-have)

8. **`fglpkg pack`** — Produce the publishable zip without uploading. Required for CI pipelines that build the artifact first and upload separately.
9. **`fglpkg publish --dry-run`** — Validate manifest, list files that would be included, show target URL and asset name. Prevents accidental bad publishes.
10. **`fglpkg info <pkg>[@version]`** / **`view`** — Fetch registry metadata without installing. Developers need this to evaluate packages.
11. **`fglpkg deprecate <pkg>@<version> "<message>"`** — Mark a version deprecated with a warning shown on install. The roadmap mentions this; not yet implemented.
12. **`.fglpkgignore`** — Exclude files from the published zip. Currently the `files` globs are inclusion-only; exclusion patterns are more ergonomic for large projects.
13. **Dist-tags / release channels** — `fglpkg publish --tag beta`, `fglpkg install pkg@beta`. Critical for pre-release workflows.
14. **Organizations / scoped names** — `@fourjs/poiapi` namespace. Prevents name squatting and enables team-based ownership. Requires a registry schema change.
15. **2FA for publish** — TOTP or WebAuthn at publish time. Account takeover is the dominant supply-chain attack vector.
16. **Prepublish validation** — Fail publish if the manifest is missing required fields (description, license, repository), or if the version was not bumped since the last publish.

### P2 — Ecosystem integration (enables broader adoption)

17. **VS Code extension** — Manifest autocomplete, install/search UI, dependency tree panel. Covered in roadmap Phase 2.2.
18. **JSON schema for `fglpkg.json`** — Publish at a stable URL (e.g. `https://fglpkg.io/schema/v1.json`); editors auto-discover via `$schema`. Low effort, high UX return.
19. **Genero Studio plugin** — Deep GST integration (generate variables into `.4pw`, right-click install, etc.). Native audience; a true differentiator.
20. **Shell completions** — `fglpkg completion bash|zsh|fish|powershell`. Table stakes for CLI polish.
21. **GitHub Action** — `fourjs/setup-fglpkg@v1` action published to Marketplace. Roadmap Phase 3.1.
22. **Docker image + Helm chart for registry server** — Self-hosted deployment kit. Roadmap Phase 2.3.
23. **Telemetry (opt-in)** — Install timing, error rates, command usage. Roadmap Phase 1.4.
24. **SBOM generation** — `fglpkg sbom` emits CycloneDX or SPDX JSON. Increasingly required by customer procurement.

### P3 — Competitive / delight features

25. **Download statistics** — Per-version, weekly/monthly counts. Shown on the web UI and via `fglpkg info`.
26. **Dependents graph** — "Who depends on this package?" Helps maintainers assess the blast radius of breaking changes.
27. **`fglpkg link`** — Symlink a local in-development package into another project (outside workspaces).
28. **Offline install from cache** — Install works with no network if all packages are cached.
29. **Parallel downloads** — Current install appears serial; parallelizing is table-stakes performance work.
30. **Progress bars / status UI** — Install UX polish.
31. **Package migration / rename** — `fglpkg migrate old new` — update manifest and lockfile references. Roadmap Phase 2.4.
32. **LDAP / SAML / SSO** — Enterprise auth. Required for customer deployments where a new identity source is unacceptable. Roadmap Phase 2.1.
33. **Audit log with retention** — Who published/unpublished/deprecated, when, from what IP. Compliance requirement.

---

## Genero-Specific Differentiators to Preserve and Expand

fglpkg's moat is its Genero focus; these features have no direct analog in npm/gem/maven and should be preserved and expanded:

- **Genero version variants** — One package, multiple runtime variants. Already a differentiator.
- **`fglpkg bdl`** — Run BDL programs from packages with environment auto-configured. No mainstream package manager is "runner + dependency manager" combined like this.
- **Genero Studio `--gst` env output** — Seamless GST integration. Expand to a full GST plugin (P2 #19).
- **Java JAR + BDL unified dependency graph** — Rare to have a language ecosystem that resolves both. Worth promoting in positioning.
- **Workspace + local member linking** — Monorepo support tuned for BDL module layout.

---

## Phased Sequencing (Mapped onto Existing Roadmap)

The existing [fglpkg-enhancement-roadmap.md](fglpkg-enhancement-roadmap.md) defines three phases: PS Ready (3 months), Customer Ready (6 months), Enterprise & Ecosystem (12 months). This gap list maps onto those phases as follows — items in **bold** are additions the existing roadmap does not yet enumerate.

### Phase 1 — Professional Services Ready

Already in roadmap: web UI, docs integration, PS templates, usage analytics.

**Adds from this gap analysis:**
- **Dependency scopes (dev + optional)** (P0 #1)
- **Declarative lifecycle steps** (P0 #2)
- **`fglpkg version` bump command** (P0 #3)
- **`fglpkg outdated`** (P0 #4)
- **CI gate on passing tests** (P0 #8)
- **`fglpkg pack`** (P1 #8)
- **`fglpkg publish --dry-run`** (P1 #9)
- **`.fglpkgignore`** (P1 #12)
- **JSON schema for `fglpkg.json`** (P2 #18)
- **Shell completions** (P2 #20)

Rationale: these are CLI-local or infra-local and do not require registry schema changes. PS developers hit all of them daily. The CI gate lands early because every subsequent change depends on a green baseline.

### Phase 2 — Customer Ready

Already in roadmap: enterprise auth, VS Code extension, self-hosted deploy kit, deprecate/migrate/audit/outdated.

**Adds / reinforces from this gap analysis:**
- **`fglpkg audit`** (P0 #5) — roadmap mentions this but as Phase 2.4; this analysis promotes it to P0 because enterprise security review will gate customer deployment.
- **Package signing and verification** (P0 #6) — not in roadmap; supply-chain authenticity requirement.
- **`fglpkg deprecate`** (P1 #11) — roadmap Phase 2.4.
- **`fglpkg info` / `view`** (P1 #10)
- **Dist-tags / release channels** (P1 #13)
- **Organizations / scoped names** (P1 #14)
- **2FA for publish** (P1 #15)
- **Prepublish validation** (P1 #16)
- **SBOM generation** (P2 #24)

### Phase 3 — Enterprise & Ecosystem

Already in roadmap: CI/CD integration, advanced security (signing, SBOM, vulnerability mgmt), analytics platform.

**Adds from this gap analysis:**
- **LDAP / SAML / SSO** (P3 #32) — roadmap mentions LDAP/AD integration at Phase 2.1; this doc keeps it in Phase 3 because it's a deep enterprise-specific integration.
- **Audit log with retention** (P3 #33)
- **Download statistics** (P3 #25)
- **Dependents graph** (P3 #26)
- **`fglpkg link`** (P3 #27)
- **Offline install from cache** (P3 #28)
- **Parallel downloads** (P3 #29)
- **Package migration / rename** (P3 #31) — roadmap Phase 2.4.

---

## Registry Infrastructure & Domain Governance — Open Questions

Status: **unresolved, blocking Phase 2**. Must be answered before any public-facing announcement because the production URL ends up hardcoded into thousands of `~/.fglpkg/credentials.json` files once the community grows — switching it later is expensive (we already hit a small version of this pain with the `registry.fglpkg.dev` / `fglpkg-registry.fly.dev` mismatch).

The questions below are grouped by decision point. Each needs an owner and a decision; none should be left as "we'll figure it out when we need to."

### 1. Domain ownership and registration

- Who is the **legal registrant** of the production domain(s)? Four J's corporate entity, a subsidiary, or an individual? (Individual registrations create succession risk.)
- Which TLDs should be registered to prevent squatting? Candidates: `.dev` (Google, HSTS-preloaded), `.io` (developer convention, expensive), `.com` (safest, usually taken), `.org`, `.app`.
- Who pays for and renews the registration? Who gets the expiry notifications? Single point of failure?
- Is there a **dispute-resolution plan** if an employee leaves with domain-admin access?

### 2. Canonical production URL

- What is the single canonical URL for the production registry? Options:
  - `https://registry.fglpkg.<tld>` (current code default, matches npm/PyPI convention)
  - `https://fglpkg.<tld>` (shorter, but mixes web UI and API on same host)
  - `https://api.fglpkg.<tld>` + `https://www.fglpkg.<tld>` (split — more moving parts but clearer)
- Does the URL need to be **API-versioned** (`/v1/packages/...`) to allow future breaking changes without a second domain? The current API is unversioned.

### 3. Fly.dev URL — keep, front, or retire?

The current `fglpkg-registry.fly.dev` URL is the actual hosted service. Three paths:

- **Front it** — point `registry.fglpkg.<tld>` at the Fly service via custom CNAME; `fly.dev` URL keeps working but users use the custom one. Low effort, keeps existing users working.
- **Migrate and retire** — all users switch to custom domain; Fly URL stops serving. Cleanest long-term, requires a credential migration path (flag day, or dual-support window with deprecation warning).
- **Dual-support forever** — both URLs work. Not recommended; doubles cert/monitoring/support surface and confuses documentation.

Recommendation needed.

### 4. TLS / certificates

- Issuer: Let's Encrypt via Fly's automatic cert (free, renews automatically) vs a managed cert from a commercial CA (required for some enterprise customers' security policies)?
- Rotation owner: who gets paged if cert renewal fails?
- Does any enterprise customer's procurement require EV certs?

### 5. Environment split

- Do we need separate **staging** and **prod** registries (and URLs)? If yes: `staging.registry.fglpkg.<tld>`?
- What about a **sandbox** URL for tutorials / demos where test publishes don't pollute prod?

### 6. Self-hosted / air-gapped customers

Feeds into P2 #22 (Helm chart) but the naming convention is a governance decision now:

- When a customer deploys their own fglpkg registry, what URL convention do we recommend? `fglpkg.<customer-domain>`?
- Do we provide a **corporate-mirror** mode where an on-prem instance proxies the public registry (cache + audit), for companies that need offline but still want public packages?

### 7. Namespace / scope URLs (interacts with P1 #14)

If organizations / scoped names land (`@fourjs/poiapi`):

- Does the URL pattern become `/@fourjs/poiapi` or `/packages/@fourjs/poiapi`? How does URL-encoding of `@` interact with curl, browsers, corporate proxies?
- Does `@scope/name` conflict with existing unscoped `name` routes? Need a migration/compatibility answer.

### 8. Data residency & compliance (Phase 3 / enterprise)

- Any customer requirements for EU-only hosting? US-only? This affects Fly region choice and may require a multi-region deployment story later.
- Retention policy on published artifacts — do we promise "packages never disappear" like npm, or reserve the right to purge? This is policy, not just infra.

### Suggested next step

Pick the top three questions from this list, assign owners, and get answers in a single short document (or just as inline resolutions here). The rest can wait a few weeks — but ownership of the **domain registration** (section 1) should not wait even a day.

---

## Cross-References

- [fglpkg-enhancement-roadmap.md](fglpkg-enhancement-roadmap.md) — Strategy, budget, timeline, resource model. Read first.
- [user-guide.md](user-guide.md) — Current documented command surface; baseline for what's already shipped.
- [github-token-setup.md](github-token-setup.md) — Publisher/consumer auth model, relevant for 2FA (P1 #15) and signing (P0 #6) design discussions.

### Items that overlap between documents

| This doc | Existing roadmap |
|---|---|
| P0 #5 `audit` | Phase 2.4 (advanced package features) |
| P0 #7 Web UI | Phase 1.1 |
| P1 #11 `deprecate` | Phase 2.4 |
| P1 #14 Organizations | Phase 2.1 (enterprise auth) |
| P2 #17 VS Code | Phase 2.2 |
| P2 #21 GitHub Action | Phase 3.1 |
| P2 #22 Self-hosted kit | Phase 2.3 |
| P2 #23 Telemetry | Phase 1.4 |
| P2 #24 SBOM | Phase 3.2 |
| P3 #31 Migration | Phase 2.4 |
| P3 #32 LDAP/SSO | Phase 2.1 |

### Items unique to this gap analysis

Not found in the existing roadmap and therefore net-new work to consider:

- Dependency scopes — dev + optional (P0 #1)
- Declarative lifecycle steps (P0 #2)
- `fglpkg version` bump (P0 #3)
- `fglpkg outdated` (P0 #4)
- Package signing and verification — Sigstore (P0 #6)
- CI gate on passing tests (P0 #8)
- `fglpkg pack` (P1 #8)
- `fglpkg publish --dry-run` (P1 #9)
- `fglpkg info` / `view` (P1 #10)
- `.fglpkgignore` (P1 #12)
- Dist-tags (P1 #13)
- 2FA for publish (P1 #15)
- Prepublish validation (P1 #16)
- JSON schema (P2 #18)
- Genero Studio plugin (P2 #19)
- Shell completions (P2 #20)
- Download statistics (P3 #25)
- Dependents graph (P3 #26)
- `fglpkg link` (P3 #27)
- Offline cache (P3 #28)
- Parallel downloads (P3 #29)
- Progress bars (P3 #30)
- Audit log with retention (P3 #33)

# fglpkg Web Registry UI — MVP Design

**Document Version:** 0.1 (draft)
**Date:** April 22, 2026
**Status:** Design draft — not yet implemented
**Scope:** Minimum viable web interface for an initial user group of **5–10 trusted users** (internal PS team + a few early-access partners). This is deliberately **not** the public-community UI described in `market-readiness-gaps.md` P0 #7; that scope is an evolution target, not the starting point.

---

## 1. Purpose and scope

The CLI is currently the only way to interact with the fglpkg registry. For 5–10 users who already have registry tokens, the absence of a web UI is tolerable but frustrating: there is no way to browse what packages exist, read a package's description, or share a link to a specific version with a colleague. This document defines the smallest web interface that removes those frustrations without committing to the full community-facing scope.

### In scope (MVP)

- Browse a list of published packages.
- Search by name or description.
- View a single package: description, author, license, repository link, rendered README, version list, per-version metadata (Genero variants, size, checksum), dependencies.
- View the currently authenticated user (`whoami`) and rotate your own token from the UI.
- Admin-only: create new user accounts, list users, manage package owners.
- Copy-to-clipboard install commands (`fglpkg install <pkg>`).

### Out of scope (deferred)

These items belong to P0 #7 in `market-readiness-gaps.md` but are **not** needed for 5–10 trusted users:

- Self-service signup / registration forms.
- Email verification.
- Password recovery flows (there are no passwords — see §5).
- Rate limiting / anti-abuse (reverse proxy or Fly.io native is sufficient at this scale).
- Download counts, trending packages, popular-this-week panels.
- Dependents graph ("what uses this package").
- User profile pages, avatars, bios, organization pages.
- Scoped/namespaced package browsing (no scopes exist yet).
- Full-text README search (name + description search only).
- Two-factor authentication (UI should not preclude adding it later).
- Internationalization (English only).
- Dark mode (no strong reason not to use system `prefers-color-scheme`, but no explicit theming work).

Documenting these as out of scope now prevents a reviewer from asking "where is X" and getting a vague "later" — it will be in the evolution plan in §9.

---

## 2. User personas

Three concrete roles; a single user can have more than one.

| Role | Count | Primary tasks |
|---|---|---|
| **Consumer** | ~7 of 10 | Browse packages, read READMEs, copy install commands, search. Rarely publishes. |
| **Publisher** | ~3 of 10 | Everything a Consumer does, plus: verify own publishes went through, manage packages they own, rotate their token. |
| **Admin** | 1–2 | Everything above, plus: create user accounts, list users, revoke tokens, manage package owners on behalf of others, manage registry configuration (GitHub repo allow-list). |

All three roles exist in the registry server today. The UI does not introduce new roles.

---

## 3. Screen inventory

Eight screens. Each is a single route; there is no complex client-side routing.

### 3.1 `GET /` — Home / package list

- List of all packages, sorted by name (default) or recently-updated.
- Each row: name, current latest version, one-line description, author, last-updated date.
- Search box at top (filters the list in-place or redirects to `/search?q=…`).
- "Sign in" link top-right if not authenticated; username + menu if authenticated.
- **API used:** `GET /search?q=` with empty query to list all, or a new `GET /packages` endpoint (see §7.1).

### 3.2 `GET /search?q=<term>` — Search results

- Same layout as home but filtered. Highlights matched substrings in name / description.
- Empty state: "No packages match '<term>'. Try a different search, or browse all packages."
- **API used:** `GET /search?q=<term>`.

### 3.3 `GET /p/<name>` — Package detail (current / latest)

- Header: name, latest version, author, license badge, repository link.
- Tabs or sections:
  - **README** — rendered from the zip's `README.md` (§7.2).
  - **Versions** — list, newest first. Each version links to `/p/<name>/<version>`.
  - **Dependencies** — FGL and Java deps for the latest version.
  - **Owners** — list of usernames with publish rights.
- Sidebar / top: copy-paste install command `fglpkg install <name>`.
- If the viewer owns this package, a "Manage owners" button (admin or owner only).
- **API used:** `GET /packages/<name>/versions`, `GET /packages/<name>/<latest>`, `GET /packages/<name>/owners`.

### 3.4 `GET /p/<name>/<version>` — Package detail (specific version)

- Same as 3.3 but pinned to the given version. A banner indicates "You are viewing version X; the latest is Y" with a link.
- Per-version metadata: Genero variants available (e.g. "4, 6"), zip size, SHA256 checksum, publish timestamp.
- "Download zip" button links to `/packages/<name>/<version>/download` with the user's bearer token attached — or, more practically, is a command the user runs from the CLI.
- **API used:** `GET /packages/<name>/versions`, `GET /packages/<name>/<version>`.

### 3.5 `GET /login` — Sign in

- Single field: paste your registry token (the same one in `~/.fglpkg/credentials.json`).
- Explanation: "Your token is the credential `fglpkg login` saved locally. Copy it from `~/.fglpkg/credentials.json` or run `fglpkg whoami` to confirm."
- On submit, POST to `/ui/session` (see §5); sets a session cookie and redirects to `/`.
- Deliberately **no** email/password form — the registry has no passwords, only tokens.
- **API used:** `GET /auth/whoami` (to validate the pasted token) then server-side session creation.

### 3.6 `GET /me` — Your account

- Shows username, role (admin / user), token metadata (last-rotated, not the token itself).
- Buttons: **Rotate token** (confirms via modal; new token displayed once, not stored in the UI), **Sign out** (destroys session).
- For admins: link to `/admin`.
- **API used:** `GET /auth/whoami`, `POST /auth/token/rotate`.

### 3.7 `GET /admin/users` — User management (admin only)

- Table of all users: username, role, created-at, token last-rotated.
- "Create user" button → modal with a username field. On submit, `POST /auth/token` returns the new user's token, which is displayed **once** with a copy-to-clipboard button and a warning that it will not be shown again.
- Per-row "Revoke token" action (`DELETE /auth/token`).
- **API used:** `GET /auth/users`, `POST /auth/token`, `DELETE /auth/token`.

### 3.8 `GET /admin/config` — Registry config (admin only)

- List of allow-listed GitHub repos (the blob backend target list).
- Add / remove buttons.
- **API used:** `GET /config`, `POST /config/github-repos`, `DELETE /config/github-repos/:owner/:repo`.

No other screens. Anything else (trending, stats, dependents, etc.) is out of scope.

---

## 4. Out-of-scope decisions, with rationale

One paragraph each so future reviewers understand **why** these were excluded at MVP.

- **Self-service signup.** With 5–10 known users, the admin creates accounts. Adding signup forms introduces abuse vectors (CAPTCHA, email verification, anti-spam, rate limiting) that are disproportionate effort for the audience.
- **Publish from the UI.** Publishing happens via `fglpkg publish` from a developer's machine or CI. A web "publish" button would duplicate that flow and introduce a file-upload attack surface. Consumers and publishers read from the UI; writes (publish, unpublish) stay on the CLI.
- **Downloads from the browser.** Zip downloads work via authenticated API but are awkward in a browser session (token header required). The UI shows the CLI command instead; a cached-download button can come later if demand appears.
- **Dependency graphs (visual).** A dep tree for a single package is useful; a full "who depends on X" graph requires registry-wide indexing that we do not have. Text-list deps only.

---

## 5. Authentication model

The UI uses **session cookies fronting existing token auth**. No changes to the CLI auth contract.

1. User visits `/login`, pastes their registry token.
2. Browser posts `{ token }` to a new endpoint `POST /ui/session`.
3. Server calls its own `authenticate()` against the pasted token. If valid, it:
   - Generates a random session ID (32 random bytes, base64-encoded).
   - Stores `{ sessionID → (userID, expires) }` in memory (or in the existing data dir — see §8.2).
   - Sets `Set-Cookie: fglpkg_session=<id>; HttpOnly; Secure; SameSite=Strict; Max-Age=86400`.
   - Returns 204 No Content.
4. All subsequent UI requests carry the cookie; middleware translates cookie → token → user.
5. `POST /ui/session` DELETE method (or `POST /ui/logout`) destroys the session.

**Why a session, not bearer-token-per-request?** Pasting a token once per day is acceptable UX for trusted users. Sending the raw token on every request (including image/CSS fetches) leaks it into server logs and any reverse proxy. A session ID does not.

**Token rotation.** When the user rotates via §3.6, the server invalidates the old token, issues a new token, and silently updates the session's underlying token mapping. The user does not need to re-login.

**2FA-ready.** Cookie-based sessions leave room to add a TOTP step at `/login` later without redesigning the whole flow.

---

## 6. Tech stack recommendation

**Recommendation: server-rendered HTML from Go using `html/template`, served from the existing registry server binary. No SPA framework, no build step, no new repo.**

Rationale:

- The audience is 10 people; interactive UX gains from a SPA are not worth a new build pipeline.
- Zero deployment change: the existing `cmd/registry` binary already runs on Fly.io; adding HTML routes next to the JSON routes means "one deploy, one artifact."
- Go's standard library has everything needed: `html/template`, `net/http`, plus one small markdown library for the README rendering.
- Static assets (one CSS file, minimal JS for clipboard-copy and the rotate-token modal) are served from `/static/`, embedded via `embed.FS`.
- Evolution path is clean: if and when the public scope lands (P0 #7 full scope), the HTML routes can be replaced by a SPA that talks to the same JSON API without touching the backend.

Concrete dependencies:

| Need | Choice | Rationale |
|---|---|---|
| Markdown rendering | `github.com/yuin/goldmark` | Single-purpose, zero runtime dependencies, safe HTML output. |
| CSS | Hand-written single file, ~150 lines | No Tailwind/Bootstrap at this scale; adds build complexity. |
| Client JS | Vanilla, inline where possible | Clipboard API, `<dialog>` element — no framework. |
| Icons | Unicode / SVG inline | No icon library. |

Total new dependencies: **one** (goldmark).

### Why not a SPA (Next.js, Remix, SvelteKit, Astro)?

Because we would be paying upfront cost for:

- A second CI pipeline (npm/pnpm install, build, lint).
- A second deploy target or coupled build output.
- Client-side routing / state management for four screens.
- Node version management.

None of which make the lives of 10 users meaningfully better. These become reasonable trade-offs around the "public community" inflection point — not before.

---

## 7. API touchpoints and gaps

### 7.1 Endpoints the UI uses (existing)

All defined in `internal/registry/server/server.go`:

- `GET /search?q=<term>` — home / search.
- `GET /packages/:name/versions` — version list.
- `GET /packages/:name/:version` — single version metadata.
- `GET /packages/:name/owners` — owner list.
- `GET /auth/whoami` — session validation, "Your account".
- `GET /auth/users` — admin user list.
- `POST /auth/token` — admin create user.
- `DELETE /auth/token` — admin revoke / user revoke.
- `POST /auth/token/rotate` — user token rotation.
- `GET /config` + `POST|DELETE /config/github-repos/*` — admin registry config.

### 7.2 Missing endpoints the UI needs

Three small additions to the server:

1. `GET /packages` — list all packages, paginated. Currently `/search?q=` with an empty query may or may not return all; a dedicated list endpoint is cleaner and easier to paginate. Output: `{ packages: [{ name, latestVersion, description, author, updatedAt }], next }`.
2. `GET /packages/:name/:version/readme` — extract and return rendered markdown from the zip's `README.md`. Needed because the UI should not download the full zip just to render the README. Output: `{ markdown: "..." }` (the server sends raw markdown; the browser renders it via goldmark-compiled HTML — or, simpler, the server renders markdown → HTML and returns HTML).
3. `POST /ui/session` / `DELETE /ui/session` — the session lifecycle endpoints from §5. Separate from `/auth/*` because they are UI-specific.

Each is a small addition; none requires a schema change.

### 7.3 Endpoints NOT needed by the MVP UI

For clarity:

- `POST /packages/:name/:version/publish` — publish is CLI-only.
- `GET /packages/:name/:version/download` — download is CLI-only (UI shows the command).
- `DELETE /packages/:name/:version` — unpublish is CLI-only.

---

## 8. Deployment plan

### 8.1 Same binary, same deploy

`cmd/registry` grows a `--ui-enable` flag (default on). When enabled:

- HTML routes mount at `/`, `/p/...`, `/login`, `/me`, `/admin/*`.
- Static assets mount at `/static/*` from an `embed.FS`.
- Existing JSON routes continue to serve on their existing paths. No conflict: `/packages/...` JSON routes remain; the HTML route is `/p/...`.

Fly.io deploy is unchanged.

### 8.2 Session storage

Two options; pick one before implementing.

- **In-memory map.** Sessions lost on process restart (one restart a day on Fly.io is typical). Users re-login. For 10 users this is fine and keeps the backend stateless.
- **On-disk (existing data dir).** Sessions survive restarts. Adds a tiny file-store, no new dependency.

Recommendation: **in-memory** for MVP. Upgrade later only if users complain.

### 8.3 HTTPS / cookies

The existing Fly deployment terminates TLS at the edge. The `Secure; HttpOnly; SameSite=Strict` cookie attributes are safe to set unconditionally in production. For local dev, a `--ui-dev` flag relaxes `Secure`.

---

## 9. Evolution path to full P0 #7 scope

When the time comes to open to the broader community, these are the upgrade steps, in order:

1. **Self-service signup** — add a `/signup` route, email verification via SES/Postmark/similar. Requires an email field on users (schema change: `users.email`).
2. **Rate limiting** — Fly-level or a middleware layer. Required before signup is public.
3. **Password reset / token recovery** — users who lose their token currently ask an admin. At public scale, self-serve via email.
4. **2FA** — TOTP at login, required for publish. The cookie-session design leaves room for this.
5. **Download stats** — counter in the registry on each download; aggregated per day. UI surfaces on package page.
6. **Dependents graph** — requires a reverse index at publish time. Biggest backend change.
7. **SPA migration (optional)** — at this point, interactivity demands may justify a SPA. Keep the same JSON API; swap the template layer.

Each step is independently shippable. The MVP is not a throwaway prototype — its backend (session model, template structure, API surface) survives into the community-scale product.

---

## 10. Open questions (pre-implementation)

Answer these before writing code, because they affect structure:

1. **Who hosts the UI's DNS record?** Tied to the governance worksheet in `market-readiness-gaps.md` §Registry Infrastructure. If the canonical URL is `https://registry.fglpkg.<tld>`, does the UI live at the root (`/`) or a subpath (`/ui`)? Recommendation: root, because 10 users will type the base URL into a browser.
2. **Do admins want an audit log in the UI?** "Who created account X, when" is easy to add at MVP if we persist the audit events now; retrofitting later means losing early history. Recommendation: log events to the existing data dir from day one; surface in the UI in step 1 of §9.
3. **What is the "description" source for the package-list view?** Current registry metadata stores `description` from `fglpkg.json`. Good. Confirm it is indexed for search.
4. **README size cap.** A malicious READMEs with 10 MB of content will make the UI slow. Cap server-side markdown rendering at (say) 256 KB with truncation and a "README truncated" notice.
5. **Clipboard copy UX.** The install-command copy button is core UX. Confirm `navigator.clipboard` is acceptable (HTTPS-only; works on all targeted browsers).
6. **Which browsers are supported?** Recommendation: last two major versions of Chrome, Firefox, Safari, Edge. No IE, no polyfills.

---

## 11. Work estimate

Rough sizing, for scheduling only:

| Chunk | Effort |
|---|---|
| HTML templates + CSS, 8 screens | 2–3 days |
| Session + login flow | 0.5 day |
| Goldmark integration + README endpoint | 0.5 day |
| `GET /packages` listing endpoint | 0.5 day |
| Admin screens (users, config) | 1 day |
| Rotate-token modal + JS | 0.25 day |
| End-to-end manual testing with 3–5 test accounts | 0.5 day |
| Documentation updates (user-guide.md) | 0.25 day |
| **Total** | **≈ 5–6 days of one engineer's focused time** |

This assumes the API additions in §7.2 are done alongside the UI, not as a separate milestone.

---

## 12. Approval needed before implementation

- Product / stakeholder sign-off on §1 scope (what is in, what is deferred).
- Agreement on the tech-stack recommendation in §6 (or a documented dissent).
- Answers to the open questions in §10, at least #1, #2, and #4.

# NetBerth Release & Build Guide

## Current Release

**v1.4.1** (2026-08-14) — UDP forwarding reliability

- Fixed: UDP forwarding could stop working entirely in environments where the
  IPv6 listener fails to bind (the failing listener closed the shared session
  map).
- Fixed: stopping or reloading a UDP rule could panic the process
  ("close of closed channel"); the shared UDP session map is now closed
  exactly once, tied to rule lifetime.
- Added mandatory data-plane devil tests: real TCP/UDP traffic through the
  forward engine and WebSocket through the reverse proxy.

Previous release: **v1.4.0** (2026-08-13) — Bilingual UI & encrypted backups

- Bilingual admin panel (English/中文), zero-dependency i18n, header toggle
- Encrypted backup/restore: `.nbbk` (AES-256-GCM + Argon2id), plaintext `.db`
  fully backward compatible; NBBK2 whole-stream integrity
- Restore hardening: 0600 temp files, fsync, atomic replace with rollback

Previous release: **v1.3.1** (2026-08-13) — STUN/ACME hardening

- STUN RFC 5389 compliance: MAPPED-ADDRESS / ALTERNATE-SERVER plain encoding,
  IPv6 XOR-MAPPED-ADDRESS, FINGERPRINT CRC-32 validation, response source and
  transaction-ID binding, hole-punch packet truncation fix
- ACME hardening: per-step timeouts, idempotent Stop, corrupt account key fails
  loudly, key files normalized to 0600
- Shared constant-time HMAC helpers (`pkg/security`) used by webhooks and CSRF
- Test coverage: acme 93.3%, websocket 97.8%, stun 80.9%, pkg/security 100%

Previous release: **v1.3.0** (2026-08-12) — Reliability & Notifications

- Webhook notifications: `/api/v1/webhooks` CRUD + test endpoint, admin UI,
  HMAC-SHA256 signatures, retries/backoff, bounded queue, event filtering
- Trusted proxy whitelist (`NB_TRUSTED_PROXIES`): proxy headers are ignored by
  default and only honored from explicitly trusted peers (IP/CIDR)
- Login/API hardening: body caps (8KB/64KB), password ≤128B, username ≤64B,
  per-IP brute-force lockout (5 fails → 5 min 429), non-spoofable rate limiting
- HTTP server hardening: `ReadHeaderTimeout` 5s, `IdleTimeout` 120s,
  `MaxHeaderBytes` 64KB; schema v4 with automatic pre-migration backup
- All prior v1.2.0 hardening: doctor, refresh-token revocation, schema
  versioning, `/api/v1/system/metrics`, HSTS, Go 1.26 + security upgrades

Earlier: **v1.1.0** — TLS panel, multi-user management, audit dashboard, PostgreSQL

## Container Images

Multi-arch images (linux/amd64, linux/arm64) are published to **GitHub Container
Registry** automatically by CI — no Docker Hub secrets required:

The package is public: anyone can pull without authentication.
All GHCR images built by the signing pipeline (2026-08-14 onward, including
`latest`) are signed (cosign keyless) and carry an SLSA provenance
attestation; verify with `./scripts/verify-image.sh`. v1.4.0 predates this
pipeline.

```bash
docker pull ghcr.io/netberth/netberth:latest
docker pull ghcr.io/netberth/netberth:v1.4.1
docker pull ghcr.io/netberth/netberth:v1.4.0
docker pull ghcr.io/netberth/netberth:v1.3.1
docker pull ghcr.io/netberth/netberth:v1.3.0
```

Docker Hub publishing runs automatically on `v*` tag pushes once
`DOCKER_USERNAME` / `DOCKER_PASSWORD` secrets are configured (see below).
Until then the publish job logs a skip message and does not fail.

## Docker Hub Publishing (human-only secret setup)

The publish job pushes the same multi-arch image to Docker Hub on every tag
push, in addition to GHCR. It is gated on two repository secrets so the badge
never turns red before they exist.

Setup (human only; never paste tokens into chat, issues, or CI logs):

1. Docker Hub: create an account and a repository (default name
   `netberth/netberth`, or your own namespace).
2. Docker Hub → Account Settings → Security → New Access Token, with
   read/write scope. Use the token, not the account password, for CI.
3. GitHub → repository → Settings → Secrets and variables → Actions:
   - New repository secret `DOCKER_USERNAME` = Docker Hub username.
   - New repository secret `DOCKER_PASSWORD` = the Docker Hub access token.
   - Optional repository variable `DOCKERHUB_REPO` = `username/netberth`
     if your Docker Hub namespace differs from `netberth`.
4. Push a new `v*` tag (or run the workflow manually with the `version`
   input). The publish job logs in and pushes `:latest` and `:<tag>`.

Verification: after the run, `docker pull <namespace>/netberth:latest` on any
machine, and check the publish job log for "Build and push" success.

## Building a Release

```bash
./scripts/release-gate.sh   # one-command gate: tests + build + public audit + public tests
./scripts/release.sh        # build artifacts only (requires zig: brew install zig)
```

`release-gate.sh` verifies:

1. Private tree is clean and `pkg/version` matches `web/package.json`
2. Full `go test ./...` passes
3. `release.sh` embeds the frontend, cross-compiles amd64/arm64 with
   `zig cc` (static musl, `-trimpath -buildvcs=false`), runs mandatory strings
   checks and writes `sha256sums.txt`
4. Private tree is mirrored to the public repo **in place** (the public `.git`
   is never deleted) with the canonical exclude list
5. Public tree audit: no HANDOVER/AGENTS/reports, no enterprise module, no
   user paths or historical secrets
6. Independent `go build` + `go test` inside the public tree

The release script itself:

1. Builds the React frontend and embeds it into `internal/api/handler/webroot/`
2. Cross-compiles `netberth-linux-amd64` and `netberth-linux-arm64` with
   `zig cc` (static musl), `-trimpath -buildvcs=false`, and a CC wrapper that
   maps `$HOME` to `/BUILDER`
3. Runs mandatory strings checks (user paths, legacy product names, internal
   IPs, historical secrets, other account names) — any nonzero fails the build
4. Writes `sha256sums.txt`

Artifacts land in `dist/release/` (override with `OUT=...`).

## Release Policy

- **Semver**: new features → minor (`1.3.0`), bug/security fixes → patch
  (`1.3.1`), breaking changes before 1.0 → minor. Keep a CHANGELOG entry per release.
- **Single source of truth**: the released tag must point to the exact commit
  the assets were built from (`git rev-parse HEAD` == tag == manifest commit).
  Any new merge invalidates a draft; rebuild the draft, never publish stale assets.
- **Immutable tags**: once a release is published, the tag is never moved or
  rewritten. If a fix is needed, cut a patch release.
- **Draft hygiene**: a draft must be rebuilt (or deleted) whenever code lands;
  title drafts with the commit SHA so a stale draft cannot be published by mistake.
- **Human gates**: only the operator may publish the GitHub release, configure
  Docker Hub secrets, or perform account-level actions.

## Publishing

```bash
./scripts/release-gate.sh                    # must be all green first
cd netberth-public   # your public checkout (must have origin github.com/netberth/netberth)
git add -A && git commit -m "release: v<version>"
git push origin main
git tag v<version> && git push origin v<version>

gh release create v<version> --draft --title "NetBerth v<version>" \
  --notes-file notes.md dist/release/netberth-linux-amd64 \
  dist/release/netberth-linux-arm64 dist/release/sha256sums.txt
# review the draft, then:
gh release edit v<version> --draft=false
```

After publishing, download the assets and verify `sha256sum -c sha256sums.txt`
from a clean machine. Never reuse a previous version's assets.

## Upgrade Path

```bash
./scripts/upgrade.sh    # auto-backup, pull, restart
```

Data in `/app/data` persists. Migrations run automatically (SQLite and PostgreSQL).

## License Tiers

| Tier | Max Rules/Module | Support | Price |
|------|-----------------|---------|-------|
| Free | 5 | Community | $0 |
| Pro | Unlimited | Email | Contact |
| Enterprise | Unlimited | SLA + Custom | Contact |

Activate via Settings page or `POST /api/v1/license/activate`.

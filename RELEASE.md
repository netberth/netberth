# NetBerth Release & Build Guide

## Current Release

**v1.2.0** (2026-08-12, draft pending review) — Stability & Usability

- `netberth doctor`: pre-flight self check (config, database integrity, TLS, state dir, ports)
- Refresh token revocation: logout, rotation, and full revocation on password change
- Schema versioning with automatic pre-migration backup (`.pre-upgrade.bak`)
- `/api/v1/system/metrics`: runtime, module counts, forward status, storage mounts
- HSTS over TLS; `govulncheck` + `npm audit` in CI; Go 1.26 toolchain
- Dependency security upgrades: chi v5.3.0, pgx v5.9.2, jwt v5.2.2, react-router-dom v7

Previous release: **v1.1.0** — TLS panel, multi-user management, audit dashboard, PostgreSQL

## Building a Release

```bash
./scripts/release.sh    # requires zig (brew install zig)
```

The script:

1. Builds the React frontend and embeds it into `internal/api/handler/webroot/`
2. Cross-compiles `netberth-linux-amd64` and `netberth-linux-arm64` with
   `zig cc` (static musl), `-trimpath -buildvcs=false`, and a CC wrapper that
   maps `$HOME` to `/BUILDER`
3. Runs mandatory strings checks (user paths, `netharbor`, `GenerateLicense`,
   internal IPs, historical secrets, other account names) — any nonzero fails the build
4. Writes `sha256sums.txt`

Artifacts land in `dist/release/` (override with `OUT=...`).

## Publishing

```bash
gh release create v<version> --draft --title "NetBerth v<version>" \
  --notes-file notes.md dist/release/netberth-linux-amd64 \
  dist/release/netberth-linux-arm64 dist/release/sha256sums.txt
# review the draft, then:
gh release edit v<version> --draft=false
```

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

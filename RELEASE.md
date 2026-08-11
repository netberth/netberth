# NetBerth Release & Build Guide

## Current Release

**v1.1.0** (2026-08-12) — [GitHub Release](https://github.com/netberth/netberth/releases/tag/v1.1.0)

- TLS termination for admin panel (`NB_TLS_ENABLED`, auto self-signed or user certs, TLS ≥ 1.2)
- Multi-user management (CRUD, roles, enable/disable, password reset, last-admin protection)
- Audit log dashboard (paginated/filtered API + admin UI)
- PostgreSQL support (`NB_DB_DRIVER=postgres` + `NB_DB_DSN`)
- Quality & security: full test coverage, SafePath hardening, deterministic FTP/WebDAV tests,
  explicit SQL error handling, config env-override fix for Docker deployments

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

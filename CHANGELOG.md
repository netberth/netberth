# Changelog

All notable changes to NetBerth are documented here. Versioning follows
[SemVer](https://semver.org/).

## [Unreleased]

### Added

- Container images published to GitHub Container Registry
  (`ghcr.io/netberth/netberth`), multi-arch linux/amd64 + linux/arm64 — no
  Docker Hub secrets required.
- Container images are **public** on GHCR: anonymous
  `docker pull ghcr.io/netberth/netberth:latest` verified end-to-end
  (container start + login + authenticated API).
- README/Quick Start refreshed for v1.3: screenshots, Webhook & trusted-proxy
  documentation, one-command container run.

## [1.3.1] - 2026-08-13

### Fixed

- STUN RFC 5389 compliance: MAPPED-ADDRESS / ALTERNATE-SERVER now use plain
  encoding (previously XORed by mistake); IPv6 XOR-MAPPED-ADDRESS decodes with
  cookie ‖ transaction ID; FINGERPRINT CRC-32 validated and required to be the
  last attribute; responses are bound to the server source and transaction ID;
  fixed hole-punch packets being truncated from `PUNCH` to 4 bytes.
- ACME hardening: per-step timeouts (30s steps, 10min authorization wait);
  idempotent `Stop()`; corrupt or unreadable account key fails loudly instead
  of silently regenerating; key files normalized to 0600.

### Changed

- New shared constant-time HMAC helpers in `pkg/security`; webhook signatures
  and CSRF tokens now use them (identical wire format).

### Tests

- acme 51% → 93.3% (full issuance success/error paths); websocket 69% →
  97.8% (broadcast, concurrency, abnormal disconnect); stun 80.9% (RFC 5389
  attributes, fingerprint, anti-spoofing); `pkg/security` 100%.

## [1.3.0] - 2026-08-12

### Added

- Webhook notifications: `/api/v1/webhooks` CRUD + test endpoint, admin UI page,
  HMAC-SHA256 signatures (`X-NetBerth-Signature`), 3 retries with exponential
  backoff, bounded delivery queue, event filtering (empty = all events).
- Trusted proxy whitelist `NB_TRUSTED_PROXIES` (IP or CIDR). Proxy headers are
  ignored by default; they are only honored from explicitly trusted peers.
- `scripts/release-gate.sh`: one-command release gate (tests → build → public
  mirror → public audit → independent public tests).

### Changed

- Login/change-password/user API bodies capped (`MaxBytesReader`, 8KB/64KB);
  passwords ≤128B and usernames ≤64B; oversized bodies return 400/413 before
  Argon2id is invoked.
- Brute-force protection wired to `/api/v1/auth/login`: 5 failures → 5-minute
  429 with `Retry-After`; successful login resets the counter. Keyed on the
  non-spoofable peer/client IP.
- HTTP server hardening: `ReadHeaderTimeout` 5s, `IdleTimeout` 120s,
  `MaxHeaderBytes` 64KB. Rate limit rate/burst configurable and validated.
- Schema v4 (`webhook_endpoints`) with automatic pre-migration backup.

### Security

- Replaced legacy `chi RealIP` (spoofable via XFF/X-Real-IP/True-Client-IP)
  with a trusted-proxy-aware client IP resolver used by rate limiting, login
  lockout, audit and logging.
- `govulncheck` and `npm audit` clean at release time (CI enforced).

## [1.2.0] - draft superseded (2026-08-12)

The original v1.2.0 draft was superseded by v1.3.0; its fixes are included
above (doctor, refresh-token revocation, schema versioning/backup, metrics,
HSTS, Go 1.26 and dependency security upgrades).

## [1.1.0] - 2026-08-12 (released)

- TLS termination for the admin panel (self-signed or user cert, TLS ≥1.2)
- Multi-user management (CRUD, roles, disable, password reset)
- Audit log dashboard
- PostgreSQL support (multi-replica)

## [1.0.0-rc1] - 2026-08-11 (released)

- TCP/UDP port forwarding with CIDR ACL
- HTTP reverse proxy with wildcard routing
- DDNS (9 providers), STUN/RFC 5389, FTP, WebDAV/FileBrowser
- WOL, Cron, ACME (Let's Encrypt)
- Single-binary Docker deployment (zig cross-compiled) + React admin panel

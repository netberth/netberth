# Security Policy

## Reporting

Please report vulnerabilities via [GitHub Security Advisories](https://github.com/netberth/netberth/security/advisories/new). Do NOT open public issues for security bugs.

We aim to acknowledge reports within 48 hours and release fixes within 7 days.

## Supported Versions

| Version | Status |
|---------|--------|
| 1.4.1 | Active |
| 1.4.0 | Previous |
| 1.3.1 | EOL |
| 1.3.0 | EOL |
| 1.2.0 | EOL |
| 1.1.0 | EOL |

## Security Architecture

- Authentication: Argon2id + JWT (HS256, 15m/7d rotation)
- Authorization: RBAC (admin/operator/viewer) + ForcePasswordChange

## Supply Chain

- GHCR images are signed with cosign using keyless signing (GitHub Actions
  OIDC) and carry an SLSA v1.0 provenance attestation. Signing is in effect
  for `latest` and release tags published after 2026-08-14 (v1.4.0 predates
  the signing pipeline).
- Signature identity is pinned to the `netberth/netberth` CI workflow and the
  `https://token.actions.githubusercontent.com` issuer.
- Users can verify any release image with `scripts/verify-image.sh`
  (requires cosign). Verification covers both the signature and the
  provenance attestation.
- Transport: rate limiting + CSRF + brute-force protection + trusted-proxy
  whitelist (`NB_TRUSTED_PROXIES`); webhook payloads signed with HMAC-SHA256
- Data: SQLite WAL (default, _txlock=immediate) or PostgreSQL (NB_DB_DRIVER/NB_DB_DSN)
- Path isolation: afero.BasePathFs (FTP) + HasPrefix guard (WebDAV)

## Known Limitations

- TLS is optional (`NB_TLS_ENABLED`); auto-generated certificates are self-signed and need manual trust
- SQLite is single-writer; use PostgreSQL for multi-writer deployments
- WebSocket: no auth on /ws endpoint (serves public stats only)

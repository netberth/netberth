# Security Policy

## Reporting

Please report vulnerabilities via [GitHub Security Advisories](https://github.com/netberth/netberth/security/advisories/new). Do NOT open public issues for security bugs.

We aim to acknowledge reports within 48 hours and release fixes within 7 days.

## Supported Versions

| Version | Status |
|---------|--------|
| 1.0.0-rc1 | Active development |

## Security Architecture

- Authentication: Argon2id + JWT (HS256, 15m/7d rotation)
- Authorization: RBAC (admin/operator/viewer) + ForcePasswordChange
- Transport: rate limiting + CSRF + brute-force protection
- Data: SQLite WAL with _txlock=immediate
- Path isolation: afero.BasePathFs (FTP) + HasPrefix guard (WebDAV)

## Known Limitations

- No TLS termination built-in (use reverse proxy)
- SQLite: single-writer concurrency model
- WebSocket: no auth on /ws endpoint (serves public stats only)

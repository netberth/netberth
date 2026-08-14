# NetBerth

[![CI](https://github.com/netberth/netberth/actions/workflows/ci.yml/badge.svg)](https://github.com/netberth/netberth/actions/workflows/ci.yml)

Self-hosted NAT traversal & networking toolbox for NAS and homelab. Port forwarding, reverse proxy, DDNS, STUN NAT traversal, Wake-on-LAN, cron scheduling, ACME certificate management, and network storage — all in one binary.

Announcements and community Q&A: [Discussions](https://github.com/netberth/netberth/discussions)

**v1.4.1** fixes UDP forwarding reliability: in environments where the IPv6
listener fails to bind, UDP forwarding could stop working entirely, and
stopping or reloading a UDP rule could panic the process. It also adds real
data-plane tests (TCP/UDP traffic through the forward engine, WebSocket through
the reverse proxy).

**v1.4.0** added a bilingual admin panel (English/中文), passphrase-encrypted
database backups (`.nbbk`, AES-256-GCM + Argon2id), and tightened release
guards; **v1.3.1** added RFC 5389 STUN compliance fixes and hardened ACME
issuance. Deployable via Docker in 30 seconds.

## Screenshots

![Dashboard](docs/screenshots/dashboard.png)
![Webhooks](docs/screenshots/webhooks.png)

## Supply Chain Verification

GHCR images are signed with cosign (keyless, GitHub Actions OIDC) and carry an
SLSA provenance attestation. This applies to `latest` and to release tags
published after the signing pipeline went live (v1.4.0 itself predates it).
Verify with:

```bash
./scripts/verify-image.sh
# or, for a specific signed tag:
./scripts/verify-image.sh ghcr.io/netberth/netberth:latest
```

This checks both the signature identity (only the netberth/netberth CI
workflow can produce it) and the provenance attestation.

## Quick Start

```bash
# Docker (recommended)
docker pull ghcr.io/netberth/netberth:latest
# Docker Hub (when publishing is enabled by the maintainer)
# docker pull netberth/netberth:latest
mkdir -p netberth-data
docker run -d --name netberth --restart unless-stopped --network host \
  -e NB_JWT_SECRET="$(openssl rand -base64 48)" \
  -e NB_SERVER_PORT=8443 \
  -v "$PWD/netberth-data:/app/data" \
  -v "$PWD/netberth-certs:/app/certs" \
  ghcr.io/netberth/netberth:latest

# Or with docker compose (builds from source)
echo "NB_JWT_SECRET=$(openssl rand -base64 48)" > .env
docker compose up -d

# Or build from source
git clone <repo-url> && cd netberth
make build && make run
```

**Admin panel**: `http://localhost:8443` (or `https://localhost:8443` with `NB_TLS_ENABLED=true`)  
**Pre-flight check**: `./netberth doctor` validates config, database integrity, TLS material and port availability.  
> Note: Docker Hub publishing runs automatically on release tags once
> `DOCKER_USERNAME` / `DOCKER_PASSWORD` secrets are configured (setup steps in
> RELEASE.md); until then use GHCR or build from source.
**Default credentials**: printed to `docker compose logs` on first run. Change immediately.

## Features

| Module | Capabilities |
|--------|-------------|
| **Port Forwarding** | TCP/UDP, IPv4/IPv6 dual-stack, whitelist/blacklist, scheduled switching |
| **Reverse Proxy** | HTTP/HTTPS, WebSocket, URL rewrite, basic auth, IP/UA ACL |
| **Dynamic DNS** | Cloudflare, Aliyun, DNSPod, GoDaddy, DuckDNS, No-IP, Dynv6, Namecheap, ClouDNS. Auto IP detection via interface or URL |
| **STUN Tunneling** | NAT traversal for services behind NAT without public IP |
| **Wake-on-LAN** | Magic packet sender, IoT platform integration ready |
| **Cron Scheduler** | Visual cron editor, shell commands, module toggle actions |
| **ACME Certificates** | Self-signed with ECDSA P-256. Auto-renew with configurable threshold |
| **Network Storage** | Local/WebDAV mount. FileBrowser, WebDAV, FTP service endpoints |
| **Webhooks** | Signed event delivery (HMAC-SHA256) for all module changes, with retries/backoff and admin UI |
| **User Management** | Multi-user accounts, admin/operator/viewer roles, enable/disable, password reset |
| **Security Hardening** | Per-IP brute-force lockout, rate limiting, trusted-proxy whitelist (`NB_TRUSTED_PROXIES`), request body/password caps, audit trail |
| **Admin TLS** | HTTPS panel with auto self-signed or user-provided certificates (TLS 1.2+) |
| **Audit Log** | Paginated audit trail with filters (admin only) |
| **Database** | SQLite by default; PostgreSQL via NB_DB_DRIVER / NB_DB_DSN |
| **Quality** | 8-suite QA harness (security/chaos/boundary/smoke/stress/sim/e2e/soak) + one-command release gate |

## Architecture

```
netberth/
├── cmd/netberth/        # Entry point, wiring, admin seed
├── internal/
│   ├── api/handler/      # REST handlers with event notifiers
│   ├── api/middleware/    # Auth, CORS, logging, rate limiting
│   ├── api/router/        # chi router + WebSocket endpoint
│   ├── api/websocket/     # Real-time status streaming (2s interval)
│   ├── auth/              # Argon2id + JWT + TOTP
│   ├── config/            # YAML + env override
│   ├── db/                # SQLite (default) or PostgreSQL, auto-migration
│   ├── engine/            # 8 network engines (each self-contained)
│   ├── model/             # Shared data models
│   └── service/           # EventBus + Wire — connects handlers to engines
├── pkg/                   # Logger, response utils, validator, retry, version
├── web/                   # React 18 + TypeScript + shadcn/ui + Tailwind
├── scripts/               # Docker entrypoint
├── Dockerfile             # Multi-stage, <20MB
├── docker-compose.yml     # Host network mode
└── Makefile               # Build, run, dev commands
```

## Security

- **Argon2id**: 64MB memory, 3 passes, 4 threads
- **JWT**: Access token 15min + refresh token 7d rotation  
- **RBAC**: admin / operator / viewer roles
- **Rate limiting**: Token bucket, 100 req/s per IP
- **2FA ready**: TOTP data model and generation
- **First-run password**: Randomly generated, printed to logs
- **No default credentials** in production
- **Optional TLS**: admin panel HTTPS with auto-generated self-signed cert, or user-provided cert/key

## API

All endpoints at `/api/v1/`. Authentication via `Bearer <token>` header.

| Method | Path | Description |
|--------|------|-------------|
| POST | `/auth/login` | Login, returns JWT pair |
| POST | `/auth/refresh` | Refresh access token |
| GET | `/auth/me` | Current user info |
| POST | `/auth/change-password` | Change password |
| GET | `/ws` | WebSocket real-time status |
| GET | `/system/status` | Server health + runtime info |
| GET | `/system/metrics` | Machine-readable runtime + module metrics |
| GET | `/system/backup` | Download database backup; add `X-NetBerth-Backup-Password` for an encrypted `.nbbk` |
| POST | `/system/restore` | Restore database (plain `.db` or encrypted `.nbbk` with password header) |
| CRUD | `/forward-rules` | Port forwarding rules |
| CRUD | `/proxy-rules` | Reverse proxy rules |
| CRUD | `/ddns` | DDNS configurations |
| CRUD | `/stun` | STUN tunnels |
| CRUD | `/wol` | WOL devices |
| POST | `/wol/{id}/wake` | Send magic packet |
| CRUD | `/cron` | Cron jobs |
| CRUD | `/acme` | SSL certificates |
| CRUD | `/storage` | Storage mounts |
| CRUD | `/webhooks` | Webhook endpoints (admin only) |
| POST | `/webhooks/{id}/test` | Send a test delivery (admin only) |
| CRUD | `/users` | User management (admin only) |
| POST | `/users/{id}/reset-password` | Reset a user password (admin only) |
| GET | `/audit` | Paginated audit trail with filters (admin only) |

## Configuration

Environment variables override `config/netberth.yaml`:

| Variable | Default | Description |
|----------|---------|-------------|
| `NB_SERVER_HOST` | `0.0.0.0` | Listen address |
| `NB_SERVER_PORT` | `8443` | Listen port |
| `NB_JWT_SECRET` | auto-generated | JWT signing key (required for multi-instance) |
| `NB_DB_PATH` | `./data/netberth.db` | SQLite database path |
| `NB_DB_DRIVER` | `sqlite` | `sqlite` (default) or `postgres` |
| `NB_DB_DSN` | empty | Postgres connection string (e.g. `postgres://user:pass@host:5432/netberth`) |
| `NB_LOG_LEVEL` | `info` | debug/info/warn/error |
| `NB_CONFIG_PATH` | `config/netberth.yaml` | Config file path |
| `NB_TLS_ENABLED` | `false` | Serve the admin panel over HTTPS (TLS 1.2+) |
| `NB_TLS_CERT` | auto self-signed | PEM certificate path (must be paired with NB_TLS_KEY) |
| `NB_TLS_KEY` | auto self-signed | PEM private key path (must be paired with NB_TLS_CERT) |
| `NB_TRUSTED_PROXIES` | empty | Comma-separated trusted proxy IPs/CIDRs; proxy headers are ignored unless the peer is trusted |
| `NB_RATE_LIMIT_RATE` | `100` | Per-IP token bucket rate (requests/sec) |
| `NB_RATE_LIMIT_BURST` | `200` | Per-IP token bucket burst |

## License

NetBerth is licensed under AGPL-3.0 (see [LICENSE](LICENSE)).
A commercial license for enterprise features is available —
contact us via [GitHub Issues](https://github.com/netberth/netberth/issues).

# NetBerth

[![CI](https://github.com/netberth/netberth/actions/workflows/ci.yml/badge.svg)](https://github.com/netberth/netberth/actions/workflows/ci.yml)

Self-hosted NAT traversal & networking toolbox for NAS and homelab. Port forwarding, reverse proxy, DDNS, STUN NAT traversal, Wake-on-LAN, cron scheduling, ACME certificate management, and network storage — all in one binary.

Deployable via Docker in 30 seconds.

## Quick Start

```bash
# Clone and run
git clone <repo-url> && cd netberth

# Docker (recommended)
echo "NB_JWT_SECRET=$(openssl rand -base64 48)" > .env
docker compose up -d

# Or build from source
make build && make run
```

**Admin panel**: `http://localhost:8443`  
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
│   ├── db/                # SQLite WAL, 10 tables, auto-migration
│   ├── engine/            # 8 network engines (each self-contained)
│   ├── model/             # Shared data models
│   └── service/           # EventBus + Wire — connects handlers to engines
├── pkg/                   # Logger, response utils
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
| CRUD | `/forward-rules` | Port forwarding rules |
| CRUD | `/proxy-rules` | Reverse proxy rules |
| CRUD | `/ddns` | DDNS configurations |
| CRUD | `/stun` | STUN tunnels |
| CRUD | `/wol` | WOL devices |
| POST | `/wol/{id}/wake` | Send magic packet |
| CRUD | `/cron` | Cron jobs |
| CRUD | `/acme` | SSL certificates |
| CRUD | `/storage` | Storage mounts |

## Configuration

Environment variables override `config/netberth.yaml`:

| Variable | Default | Description |
|----------|---------|-------------|
| `NB_SERVER_HOST` | `0.0.0.0` | Listen address |
| `NB_SERVER_PORT` | `8443` | Listen port |
| `NB_JWT_SECRET` | auto-generated | JWT signing key (required for multi-instance) |
| `NB_DB_PATH` | `./data/netberth.db` | SQLite database path |
| `NB_LOG_LEVEL` | `info` | debug/info/warn/error |
| `NB_CONFIG_PATH` | `config/netberth.yaml` | Config file path |

## License

NetBerth is licensed under AGPL-3.0 (see [LICENSE](LICENSE)).
A commercial license for enterprise features is available —
contact us via [GitHub Issues](https://github.com/netberth/netberth/issues).

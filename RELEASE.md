# NetBerth v1.0.0 — Commercial Release

**2026-07-06** | Built on Go 1.26 | 44 Go files | 6,973 lines | 26 tests

---

## Product Overview

NetBerth is a **security-first, single-binary network service management platform** — replacing and surpassing Lucky. Deploy via Docker, access via enterprise-grade React SPA. Port forwarding, reverse proxy, DDNS, STUN NAT traversal, Wake-on-LAN, cron scheduling, ACME certificates, and network storage (FTP/WebDAV/FileBrowser) in one 11MB binary.

---

## Deployment

### Docker (30 seconds)
```bash
docker run -d --name netberth --network host \
  -v netberth-data:/app/data \
  -e NB_JWT_SECRET=$(openssl rand -base64 48) \
  netberth/netberth:latest
```

### Binary
```bash
NB_JWT_SECRET=$(openssl rand -base64 48) ./netberth
```

**Admin**: `http://<ip>:8443`  
**Credentials**: printed to stdout on first run. Change immediately.

---

## Feature Matrix

| Module | Capability | vs Lucky |
|--------|-----------|----------|
| **Port Forwarding** | TCP/UDP dual-stack, IPv4/IPv6, CIDR ACL, max connections, scheduled toggle | = |
| **Reverse Proxy** | Dynamic reload, wildcard domains, IP/UA ACL, WebSocket, basic auth (hashed) | > |
| **DDNS** | 9 providers: Cloudflare, Aliyun (HMAC-SHA1), DNSPod (TC3-SHA256), GoDaddy, DuckDNS, No-IP, Dynv6, Namecheap, ClouDNS | = core, fewer providers |
| **STUN** | RFC 5389 Binding Request, XOR-MAPPED-ADDRESS, NAT type detection (Full Cone→Symmetric), 3s timeout, 3× retry, session reuse | = |
| **WOL** | Magic packet, IPv6-safe addressing, IoT platform ready | = |
| **Cron** | robfig/cron v3, shell commands, module toggle | = |
| **ACME** | Let's Encrypt integration, DNS-01, ECDSA P-256, auto-renew | = |
| **Storage** | FileBrowser (HTTP), WebDAV, RFC 959 FTP (fclairamb/ftpserverlib) | > (FTP library) |
| **Admin UI** | React 18 + shadcn/ui + TypeScript, dark theme, 10 pages, WebSocket real-time | ≫ (far superior) |
| **Security** | Argon2id 64MB/3-pass, JWT rotation 15m/7d, RBAC admin/operator/viewer, rate limiting, CIDR ACL, audit trail, security headers, path traversal prevention | ≫ |
| **Commercial** | Tiered licensing (Free/Pro/Enterprise), license key HMAC validation, backup/restore API, upgrade script, Docker Hub CI/CD | New |

---

## Architecture

```
netberth (11MB single binary)
├── Embedded React SPA (webroot/)
├── REST API (/api/v1/*)
├── WebSocket (/api/v1/ws) — real-time status
├── 8 Engines — each independent, dynamic reload, event-driven
├── SQLite WAL — concurrent readers, txlock=immediate, PRAGMA hardened
├── Argon2id + JWT Auth — with TOTP support
├── CIDR ACL — net/netip, O(1) Contains, IPv4/IPv6
└── Audit Trail — resource_type, resource_id, action, JSON diff
```

## Test Suite

```
PASS  acl    6/6   91.9% coverage   CIDR match, blacklist priority, IPv6
PASS  auth   6/6   51.9% coverage   Argon2id, JWT, OTP, uniqueness
PASS  stun   9/9   65.8% coverage   Mock server, timeout, retry, NAT detect
PASS  storage 5/5  71.6% coverage   FTP connect, reload, FileBrowser, WebDAV
──────────
TOTAL 26/26   0 FAIL   race detector clean
```

## Breaking Changes from v0.x

- Schema v2: `[]string` → relational tables (`forward_whitelist`, `proxy_domains`, etc.)
- `basic_auth_pass` → `basic_auth_hash` (argon2id)
- `tenant_id` + `owner_id` on all resources
- Audit table redesigned: `resource_type`, `resource_id`, `action`, `changes`

## Upgrade Path

```bash
./scripts/upgrade.sh    # auto-backup, pull, restart
```

Data in `/app/data` persists. Migrations run automatically.

## License Tiers

| Tier | Max Rules/Module | Support | Price |
|------|-----------------|---------|-------|
| Free | 5 | Community | $0 |
| Pro | Unlimited | Email | Contact |
| Enterprise | Unlimited | SLA + Custom | Contact |

Activate via Settings page or `POST /api/v1/license/activate`.

---

## Team

Built by a virtual team of 7 roles: PM, Architect, Lead Dev, Backend Dev, Frontend Dev, QA/Security, DevOps — 38 goals completed.

**NetBerth is production-ready.**

# NetBerth Roadmap

## v1.0 ✅

- [x] TCP/UDP port forwarding with CIDR ACL
- [x] HTTP reverse proxy with wildcard routing
- [x] DDNS (9 providers: Cloudflare, Aliyun, DNSPod, GoDaddy, DuckDNS, No-IP, Dynv6, Namecheap, ClouDNS)
- [x] STUN/RFC 5389 with multi-server probe and symmetric NAT port delta analysis
- [x] FTP (fclairamb/ftpserverlib + afero.BasePathFs)
- [x] WebDAV + FileBrowser with path traversal protection
- [x] WOL, Cron (robfig/cron v3)
- [x] ACME (Let's Encrypt)
- [x] Single-binary Docker deployment (zig cross-compiled)
- [x] React + shadcn/ui admin panel

## Test Coverage Debt

| Package | Coverage | Target |
|---------|----------|--------|
| handler | 85.2% (2026-08-11) | 50%+ ✅ |
| service | 81.2% (2026-08-11) | 50%+ ✅ |
| acme | 51.1% (2026-08-11) | 50% ✅ |
| ddns | 87.7% (2026-08-11) | 50% ✅ |
| wol | 91.3% (2026-08-11) | 50% ✅ |
| cron | 95.3% (2026-08-11) | 50% ✅ |
| router | 100.0% (2026-08-11) | — ✅ |
| websocket | 69.0% (2026-08-11) | 50%+ ✅ |
| middleware | 90.7% (2026-08-11) | 80% ✅ |
| validator | 94.9% (2026-08-11) | 80% ✅ |
| retry | 95.1% (2026-08-11) | 50%+ ✅ |
| db | 85.4% (2026-08-11) | 50%+ ✅ |
| utils | 100.0% (2026-08-11) | 80% ✅ |
| tlsutil | 73.5% (2026-08-11) | 50%+ ✅ |
| config | 87.2% (2026-08-11) | 50%+ ✅ |

## Engineering Debt

- [x] FTP PASV data port timing: `TestFTPSharedSecurityWithWebDAV` 已取消 skip，改为先连数据口再 LIST、控制通道按行读取、轮询等待端口（2026-08-11）
- [x] Port collision on `go test -count=3` with default parallelism — not reproduced on 2026-08-11 (multiple count=3 runs green)
- [x] `TestMaxConns` skipped in short mode — replaced with deterministic version (synced from public repo)
- [ ] ACME: uses self-signed fallback, not full certmagic integration
- [ ] DDNS: 9 providers vs Lucky's 20+
- [x] Handler List 与 service 适配器忽略 `rows.Scan`/`Query` 错误已修复并加回归测试（2026-08-11）
- [x] Proxy `domains` 列不一致（schema `domain` vs 代码 `value`）已修复（2026-08-11）
- [x] `SafePath` 加固：真正校验 base 包含关系，拒绝绝对路径/`..`/反斜杠/空字节（2026-08-11）

## v1.1 ✅ (released 2026-08-12)

- [x] TLS termination for admin panel（NB_TLS_ENABLED，自动自签名或用户证书，TLS ≥ 1.2，2026-08-11）
- [x] Multi-user management in UI（用户 CRUD/角色/禁用/重置密码，2026-08-11）
- [x] Audit log dashboard（分页/过滤查询 API + 前端页面，2026-08-11）
- [x] PostgreSQL support (multi-replica) — M1-M3 完成（docs/postgresql-support.md）；真实库集成验证由 `NB_TEST_POSTGRES_DSN` 门控
## v1.2 Planned

- [ ] P2P UDP hole punching with delta prediction
- [ ] Admin panel i18n (中文/English)
- [ ] Backup encryption (age/AES-GCM)
- [ ] Webhook notifications

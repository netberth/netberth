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
- [x] QA 魔鬼测试场（`qa/`：security/chaos/load(js)/e2e）落地；每个套件独立实例/端口/数据目录（2026-08-12）
- [x] WebSocket 经完整路由 Upgrade 500 —— `LoggingMiddleware` 丢 `http.Hijacker`，已修复并加路由级 WS 集成测试（2026-08-12，QA 首轮发现）
- [x] 登录请求体/密码长度上限：`MaxBytesReader` 8KB（登录/改密）与 64KB（CRUD），密码 ≤128B、用户名 ≤64B，超限 400/413，5MB 密码不再触发 argon2（2026-08-12，QA 发现）
- [x] `LoginMiddleware`（防爆破）接线 `/auth/login`：5 次失败 → 5 分钟 429（成功登录清零）；按 peer IP 计，不受 `X-Forwarded-For` 伪造影响（2026-08-12，QA 发现）
- [x] 移除 chi 旧 `RealIP`（可被 `True-Client-IP`/`X-Real-IP`/`X-Forwarded-For` 伪造，会绕过限流/防爆破），改用 `ClientIPFromRemoteAddr`，限流/防爆破/审计统一使用非伪造 peer IP（2026-08-12）
- [x] HTTP server 加固：`ReadHeaderTimeout` 5s（slowloris 防御）、`IdleTimeout` 120s、`MaxHeaderBytes` 64KB（2026-08-12）
- [x] 全局限流参数化：`NB_RATE_LIMIT_RATE` / `NB_RATE_LIMIT_BURST`（默认 100/200），QA 压测实例可放大以测量原始吞吐（2026-08-12）
- [x] QA 全量结果（2026-08-12，完整一轮 + 10 分钟 soak）：security 44/44、chaos 14/14、boundary 26/26、smoke 19/19、stress 15/15、sim 11/11、E2E 18/18、k6 ~1674rps 0 失败 WS 40/40、10 分钟 soak 852,354 checks / 849,947 请求 / 0 失败（p95 ≈0.92ms）
- [x] QA 扩展套件（2026-08-12）：boundary 26 项（畸形报文/CRLF/431/chunked 走私/路径穿越/slowloris 5s 切断）、smoke 19 项（首登改密后完整旅程）、stress 15 项（3k 洪水 0 连接错误/并发 CRUD/refresh 竞争/WS 洪水/fd=256 恢复/goroutine-RSS 无增长）、sim 11 项（TLS+HSTS/真实上游反代持久化/受限实例/doctor/共存）、soak 5min 复跑 442,841 checks 0 失败 p95≈0.86ms；`qa/rounds.sh` 支持多轮；`run-all.sh` 一键跑 8 套件（`NB_QA_SOAK=1` 追加长 soak）

## v1.1 ✅ (released 2026-08-12)

- [x] TLS termination for admin panel（NB_TLS_ENABLED，自动自签名或用户证书，TLS ≥ 1.2，2026-08-11）
- [x] Multi-user management in UI（用户 CRUD/角色/禁用/重置密码，2026-08-11）
- [x] Audit log dashboard（分页/过滤查询 API + 前端页面，2026-08-11）
- [x] PostgreSQL support (multi-replica) — M1-M3 完成（docs/postgresql-support.md）；真实库集成验证由 `NB_TEST_POSTGRES_DSN` 门控
## v1.2 ✅（已并入 v1.3.0 发布）

- [x] M1 可靠性地基：迁移版本化（schema_migrations）+ SQLite 迁移前备份、refresh token 吊销/轮换/登出/改密吊销
- [x] M2 用户第一公里：`netberth doctor` 自检命令、首次运行体验
- [x] M3 可观测性：`/api/v1/system/metrics` 接口（运行时/模块计数/forward 状态/存储挂载）
- [x] M4 安全与发布：TLS 下 HSTS、CI govulncheck/npm audit、Go 1.26、依赖安全升级、版本 1.2.0

## v1.3 ✅ (released 2026-08-12)

- [x] M1 Webhook 通知后端（2026-08-12）：`/api/v1/webhooks` 管理 API（CRUD + 测试发送）、事件总线全量订阅（forward/proxy/ddns/stun/wol/cron/acme/storage 的 created/updated/deleted）、HMAC-SHA256 签名（`X-NetBerth-Signature`）、最多 3 次重试与指数退避、有界并发（16）+ 队列满丢弃、事件过滤（空 = 全部）、secret 不回显（`has_secret`）、schema v4 迁移（含 Postgres 对齐）
- [x] 可信代理白名单（2026-08-12）：`NB_TRUSTED_PROXIES` / `trusted_proxies`（IP 或 CIDR）；仅当直连 peer 受信任时才解析 XFF/X-Real-IP/True-Client-IP，默认全部忽略（防伪造）；`rate_limit_rate/burst` 配置非法（<1）启动即报错
- [x] M2 Webhook 前端设置页（admin UI，2026-08-12）
- [x] **v1.3.0 已发布**（2026-08-12）：release-gate 8/8 全绿、CI main/tag 双绿、资产校验一致；发布协议升级（一键门禁/原地镜像/SemVer/不可变 tag/draft 卫生）
- [x] GHCR 容器镜像发布（2026-08-13）：`ghcr.io/netberth/netberth`（linux/amd64 + arm64，免 Docker Hub secrets；Docker Hub 保持手动）；包已公开，匿名 `docker pull` 验证通过
- [ ] P2P UDP hole punching with delta prediction
- [ ] Admin panel i18n (中文/English)
- [ ] Backup encryption (age/AES-GCM)

## v1.3.1 ✅ (released 2026-08-13)

- [x] STUN RFC 5389 合规修复：MAPPED-ADDRESS/ALTERNATE-SERVER 明文编码、IPv6
  XOR-MAPPED-ADDRESS、FINGERPRINT CRC-32 校验、响应源/事务 ID 反伪造、hole punch
  `PUNCH` 截断 bug
- [x] ACME 加固：每步超时、`Stop()` 幂等、损坏账户密钥失败退出、密钥 0600 归一化
- [x] HMAC 收敛：`pkg/security` 常量时间助手统一 webhook/CSRF 签名
- [x] 覆盖率：acme 93.3% / stun 80.9% / security 100% / websocket 97.8%；
  vet/构建/全量短测/企业测试/race 全绿

## v1.4 ✅ (released 2026-08-13)

- [x] Admin panel i18n（中文/English，零依赖，顶栏切换，默认英文）
- [x] 备份加密（NBBK2：AES-256-GCM + Argon2id，整体完整性认证；明文兼容）
- [x] 恢复加固（0600 临时文件、fsync、原子替换带回滚）
- [x] 发布防误推：私有仓库 pre-push hook（黑名单 SHA + 非 noreply 身份即拒）
- [x] cosign/SLSA 供应链签名（2026-08-14）：GHCR 镜像 keyless 签名 + SLSA
  provenance 证明（latest/后续 tag 自动执行，v1.4.0 为签名上线前版本）；
  `scripts/verify-image.sh` 本地校验实测通过

## v1.5 Planned（待用户反馈后再排）

- [ ] P2P UDP hole punching with delta prediction
- [ ] 其他候选：CLI 备份命令、Docker Hub 自动发布

# NetBerth Roadmap

## v1.0 (current)

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
| handler | 13.6% | 60% |
| service | 4.3% | 40% |
| acme | 0.0% | 50% |
| ddns | 0.0% | 50% |
| wol | 0.0% | 50% |
| cron | 0.0% | 50% |
| middleware | 67.6% | 80% |
| validator | 0.0% | 80% |

## Engineering Debt

- [ ] FTP PASV data port timing: `TestFTPSharedSecurityWithWebDAV` flaky (t.Skip)
- [ ] Port collision on `go test -count=3` with default parallelism (some packages use fixed ports)
- [ ] `TestMaxConns` skipped in short mode (timing-sensitive)
- [ ] ACME: uses self-signed fallback, not full certmagic integration
- [ ] DDNS: 9 providers vs Lucky's 20+

## v1.1 Planned

- [ ] TLS termination for admin panel
- [ ] Multi-user management in UI
- [ ] Audit log dashboard
- [ ] PostgreSQL support (multi-replica)
- [ ] P2P UDP hole punching with delta prediction

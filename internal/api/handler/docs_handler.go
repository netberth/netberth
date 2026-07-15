// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package handler

import (
	"encoding/json"
	"net/http"
)

var docs = map[string]interface{}{
	"title":   "NetBerth Documentation",
	"version": "1.0.0-rc1",
	"sections": []map[string]interface{}{
		{
			"title":       "Quick Start",
			"description": "Deploy NetBerth in 30 seconds",
			"content": `1. Docker: docker run -d --name netberth --network host -v netberth-data:/app/data -e NB_JWT_SECRET=$(openssl rand -base64 48) netberth/netberth:latest
2. Open http://YOUR_IP:8443
3. Login with admin / password from docker logs
4. Change password immediately in Settings`,
		},
		{
			"title":       "Port Forwarding",
			"description": "Forward TCP/UDP traffic from public to internal network",
			"content": `- Create a rule: Name > Protocol (TCP/UDP/Both) > Listen Port > Target IP:Port
- IPv6: toggle to enable IPv6 listening
- Whitelist/Blacklist: comma-separated IP addresses
- Status toggle: enable/disable rule without deleting
- Rules persist across restarts`,
		},
		{
			"title":       "Reverse Proxy",
			"description": "HTTP/HTTPS reverse proxy with domain-based routing",
			"content": `- Add domain(s): example.com, *.example.com
- Set target URL: http://192.168.1.100:8080
- Enable TLS for HTTPS termination (requires certificate)
- WebSocket support: toggle for WS connections
- ACL: IP whitelist/blacklist per proxy rule
- Basic Auth: add username/password protection`,
		},
		{
			"title":       "DDNS Configuration",
			"description": "Auto-update DNS records for dynamic IPs",
			"content": `Supported providers: Cloudflare, Aliyun, DNSPod/Tencent, GoDaddy, DuckDNS, No-IP, Dynv6, Namecheap, ClouDNS

Cloudflare: Create API token at dash.cloudflare.com > API Tokens. Credentials: {"api_token":"xxx","zone_id":"xxx"}
Aliyun: RAM access key from ram.console.aliyun.com. Credentials: {"access_key_id":"xxx","access_key_secret":"xxx"}
DNSPod: API key from console.dnspod.cn. Credentials: {"secret_id":"xxx","secret_key":"xxx"}
DuckDNS: Token from duckdns.org. Credentials: {"token":"xxx"}

Interval: seconds between updates (min 60). Record type: A for IPv4, AAAA for IPv6.`,
		},
		{
			"title":       "SSL Certificates (ACME)",
			"description": "Automatic SSL via Let's Encrypt",
			"content": `- Requires DNS provider credentials for DNS-01 challenge
- Domains: comma-separated, supports wildcards (*.example.com)
- Auto-renew: enabled by default, checks every 12 hours
- Renew threshold: days before expiry to trigger renewal
- Status: pending > valid > expired > error`,
		},
		{
			"title":       "STUN / NAT Traversal",
			"description": "Access services behind NAT without public IP",
			"content": `- STUN Server: default stun.l.google.com:19302
- Local Port: port exposed on local network
- Target: internal service IP:Port
- NAT type detection: automatic via STUN binding test
- Keep-alive: automatic 30-second refresh`,
		},
		{
			"title":       "Wake-on-LAN",
			"description": "Remotely wake devices via magic packet",
			"content": `- MAC Address: AA:BB:CC:DD:EE:FF format
- Broadcast: default 255.255.255.255 (subnet broadcast)
- Port: default 9 (standard WOL port)
- Click Wake button to send magic packet`,
		},
		{
			"title":       "Cron Jobs",
			"description": "Schedule commands and actions",
			"content": `- Schedule: standard cron expression (supports seconds)
- Type: Shell Command (executes /bin/sh -c) or Module Toggle (enables/disables other modules)
- Example: "0 */6 * * *" = every 6 hours
- Output logged to container logs`,
		},
		{
			"title":       "Storage",
			"description": "Mount storage and serve via FileBrowser/WebDAV/FTP",
			"content": `- Local Path: mount any directory
- WebDAV: connect remote WebDAV storage
- Services: FileBrowser (HTTP), WebDAV, FTP
- FTP port: configurable (default 2121)
- Permissions: ensure host directory is writable`,
		},
		{
			"title":       "Upgrading",
			"description": "How to upgrade NetBerth",
			"content": `1. Backup: Settings > click "Download Backup" (or use API /system/backup)
2. Stop: docker stop netberth && docker rm netberth
3. Pull new image: docker pull netberth/netberth:latest
4. Start: docker run ... (same command, data volume preserved)
5. Verify: check logs for errors, login
Data in /app/data persists across upgrades. DB migrations run automatically.`,
		},
		{
			"title":       "License & Tiers",
			"description": "NetBerth license information",
			"content": `Free: up to 5 rules per module, community support
Pro: unlimited rules, priority support, commercial use
Enterprise: unlimited rules, dedicated support, custom features, on-prem deployment

Activate: Settings > License > paste your license key
Generate key: contact sales@netberth.io`,
		},
	},
}

func DocsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "data": docs})
	}
}

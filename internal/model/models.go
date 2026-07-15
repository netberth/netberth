// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package model

import "time"

// === Tenant & User ===

type Tenant struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Plan      string    `json:"plan"` // free, pro, enterprise
	CreatedAt time.Time `json:"created_at"`
}

type User struct {
	ID           string    `json:"id"`
	TenantID     string    `json:"tenant_id"`
	Username     string    `json:"username"`
	Email        string    `json:"email,omitempty"`
	PasswordHash string    `json:"-"`
	Role         string    `json:"role"`
	OTPEnabled   bool      `json:"otp_enabled"`
	OTPSecret    string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// === Network ACL Entry (CIDR or single IP) ===

type ACLEntry struct {
	ID    string `json:"id"`
	Value string `json:"value"` // "192.168.1.1" or "10.0.0.0/8" or "2001:db8::/32"
}

// === Port Forwarding ===

type ForwardRule struct {
	ID          string     `json:"id"`
	TenantID    string     `json:"tenant_id"`
	OwnerID     string     `json:"owner_id"`
	Name        string     `json:"name"`
	Protocol    string     `json:"protocol"`
	ListenAddr  string     `json:"listen_addr"`
	ListenPort  int        `json:"listen_port"`
	TargetAddr  string     `json:"target_addr"`
	TargetPort  int        `json:"target_port"`
	EnableIPv6  bool       `json:"enable_ipv6"`
	Whitelist   []ACLEntry `json:"whitelist,omitempty"`
	Blacklist   []ACLEntry `json:"blacklist,omitempty"`
	MaxConns    int        `json:"max_conns"` // 0 = unlimited
	Enabled     bool       `json:"enabled"`
	ScheduleOn  string     `json:"schedule_on,omitempty"`
	ScheduleOff string     `json:"schedule_off,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// === Reverse Proxy ===

type ProxyRule struct {
	ID            string     `json:"id"`
	TenantID      string     `json:"tenant_id"`
	OwnerID       string     `json:"owner_id"`
	Name          string     `json:"name"`
	Domains       []string   `json:"domains"`
	TargetURL     string     `json:"target_url"`
	TLSEnabled    bool       `json:"tls_enabled"`
	CertID        string     `json:"cert_id,omitempty"`
	ForceHTTPS    bool       `json:"force_https"`
	HTTP2         bool       `json:"http2"`
	Websocket     bool       `json:"websocket"`
	URLRewrite    string     `json:"url_rewrite,omitempty"`
	BasicAuthUser string     `json:"basic_auth_user,omitempty"`
	BasicAuthHash string     `json:"-"` // bcrypt/argon2 hash, never exposed
	IPWhitelist   []ACLEntry `json:"ip_whitelist,omitempty"`
	IPBlacklist   []ACLEntry `json:"ip_blacklist,omitempty"`
	UAWhitelist   []string   `json:"ua_whitelist,omitempty"`
	UABlacklist   []string   `json:"ua_blacklist,omitempty"`
	Enabled       bool       `json:"enabled"`
	MaxConns      int        `json:"max_conns"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// === DDNS ===

type DDNSConfig struct {
	ID           string            `json:"id"`
	TenantID     string            `json:"tenant_id"`
	Name         string            `json:"name"`
	Provider     string            `json:"provider"`
	Domain       string            `json:"domain"`
	SubDomain    string            `json:"sub_domain"`
	RecordType   string            `json:"record_type"`
	TTL          int               `json:"ttl"`
	Credentials  map[string]string `json:"credentials"`
	GetIPURL     string            `json:"get_ip_url"`
	GetIPType    string            `json:"get_ip_type"`
	NetInterface string            `json:"net_interface,omitempty"`
	Interval     int               `json:"interval"`
	Enabled      bool              `json:"enabled"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
}

// === STUN ===

type STUNTunnel struct {
	ID         string    `json:"id"`
	TenantID   string    `json:"tenant_id"`
	Name       string    `json:"name"`
	Protocol   string    `json:"protocol"`
	LocalPort  int       `json:"local_port"`
	RemotePort int       `json:"remote_port"`
	STUNServer string    `json:"stun_server"`
	TargetAddr string    `json:"target_addr"`
	TargetPort int       `json:"target_port"`
	Enabled    bool      `json:"enabled"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// === WOL ===

type WOLDevice struct {
	ID          string    `json:"id"`
	TenantID    string    `json:"tenant_id"`
	Name        string    `json:"name"`
	MAC         string    `json:"mac"`
	Broadcast   string    `json:"broadcast"`
	Port        int       `json:"port"`
	Platform    string    `json:"platform,omitempty"`
	PlatformKey string    `json:"platform_key,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// === Cron ===

type CronJob struct {
	ID         string     `json:"id"`
	TenantID   string     `json:"tenant_id"`
	Name       string     `json:"name"`
	Schedule   string     `json:"schedule"`
	Type       string     `json:"type"`
	Command    string     `json:"command,omitempty"`
	ModuleID   string     `json:"module_id,omitempty"`
	ModuleType string     `json:"module_type,omitempty"`
	Enabled    bool       `json:"enabled"`
	LastRun    *time.Time `json:"last_run,omitempty"`
	NextRun    *time.Time `json:"next_run,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// === ACME ===

type ACMECertificate struct {
	ID          string            `json:"id"`
	TenantID    string            `json:"tenant_id"`
	Name        string            `json:"name"`
	Domains     []string          `json:"domains"`
	Provider    string            `json:"provider"`
	DNSProvider string            `json:"dns_provider"`
	DNSConfig   map[string]string `json:"dns_config"`
	Email       string            `json:"email"`
	AutoRenew   bool              `json:"auto_renew"`
	RenewDays   int               `json:"renew_days"`
	CertPath    string            `json:"cert_path,omitempty"`
	KeyPath     string            `json:"key_path,omitempty"`
	ExpiresAt   *time.Time        `json:"expires_at,omitempty"`
	Status      string            `json:"status"`
	Error       string            `json:"error,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// === Storage ===

type StorageMount struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	Source    string    `json:"source"`
	Username  string    `json:"username,omitempty"`
	Password  string    `json:"-"`
	Services  []string  `json:"services"`
	FTPPort   int       `json:"ftp_port,omitempty"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// === Audit ===

type AuditEvent struct {
	ID           int64     `json:"id"`
	TenantID     string    `json:"tenant_id"`
	UserID       string    `json:"user_id"`
	Username     string    `json:"username"`
	Action       string    `json:"action"`        // created, updated, deleted
	ResourceType string    `json:"resource_type"` // forward_rule, proxy_rule, etc.
	ResourceID   string    `json:"resource_id"`   // rule UUID
	Changes      string    `json:"changes"`       // JSON diff: {"before":{...},"after":{...}}
	RemoteAddr   string    `json:"remote_addr"`
	CreatedAt    time.Time `json:"created_at"`
}

// === System ===

type SystemStatus struct {
	Uptime       int64               `json:"uptime"`
	Version      string              `json:"version"`
	CPUPercent   float64             `json:"cpu_percent"`
	MemoryMB     uint64              `json:"memory_mb"`
	ForwardRules []ForwardRuleStatus `json:"forward_rules"`
}

type ForwardRuleStatus struct {
	ID          string `json:"id"`
	Active      bool   `json:"active"`
	Connections int64  `json:"connections"`
	BytesIn     uint64 `json:"bytes_in"`
	BytesOut    uint64 `json:"bytes_out"`
}

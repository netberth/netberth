export interface User {
  id: string
  username: string
  email: string
  role: 'admin' | 'operator' | 'viewer'
  otp_enabled: boolean
}

export interface TokenPair {
  access_token: string
  refresh_token: string
  expires_in: number
}

export interface ForwardRule {
  id: string
  name: string
  protocol: 'tcp' | 'udp' | 'both'
  listen_addr: string
  listen_port: number
  target_addr: string
  target_port: number
  enable_ipv6: boolean
  whitelist: string[]
  blacklist: string[]
  enabled: boolean
  schedule_on: string
  schedule_off: string
  created_at: string
  updated_at: string
}

export interface ProxyRule {
  id: string
  name: string
  domains: string[]
  target_url: string
  tls_enabled: boolean
  cert_id: string
  force_https: boolean
  http2: boolean
  websocket: boolean
  url_rewrite: string
  basic_auth_user: string
  ip_whitelist: string[]
  ip_blacklist: string[]
  ua_whitelist: string[]
  ua_blacklist: string[]
  enabled: boolean
  created_at: string
  updated_at: string
}

export interface DDNSConfig {
  id: string
  name: string
  provider: string
  domain: string
  sub_domain: string
  record_type: string
  ttl: number
  credentials: Record<string, string>
  get_ip_url: string
  get_ip_type: string
  net_interface: string
  interval: number
  enabled: boolean
  created_at: string
  updated_at: string
}

export interface STUNTunnel {
  id: string
  name: string
  protocol: string
  local_port: number
  remote_port: number
  stun_server: string
  target_addr: string
  target_port: number
  enabled: boolean
  created_at: string
  updated_at: string
}

export interface WOLDevice {
  id: string
  name: string
  mac: string
  broadcast: string
  port: number
  platform: string
  platform_key: string
  created_at: string
  updated_at: string
}

export interface CronJob {
  id: string
  name: string
  schedule: string
  type: string
  command: string
  module_id: string
  module_type: string
  enabled: boolean
  last_run: string | null
  next_run: string | null
  created_at: string
  updated_at: string
}

export interface ACMECertificate {
  id: string
  name: string
  domains: string[]
  provider: string
  dns_provider: string
  dns_config: Record<string, string>
  email: string
  auto_renew: boolean
  renew_days: number
  cert_path: string
  key_path: string
  expires_at: string | null
  status: string
  error: string
  created_at: string
  updated_at: string
}

export interface StorageMount {
  id: string
  name: string
  type: string
  source: string
  services: string[]
  ftp_port: number
  enabled: boolean
  created_at: string
  updated_at: string
}

export interface SystemStatus {
  hostname: string
  version: string
  go_version: string
  os: string
  arch: string
  cpu_count: number
  goroutines: number
  memory_mb: number
  uptime: number
}

export interface APIResponse<T = unknown> {
  success: boolean
  data?: T
  error?: string
  message?: string
}

export interface PaginatedResponse<T = unknown> {
  success: boolean
  data: T[]
  total: number
  page: number
  page_size: number
  total_pages: number
}

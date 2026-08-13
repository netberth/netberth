import { createContext, useContext, useEffect, useState, type ReactNode } from 'react'

export type Lang = 'en' | 'zh'

const zhDict: Record<string, string> = {
  // Layout
  'Dashboard': '仪表盘',
  'Port Forwarding': '端口转发',
  'Reverse Proxy': '反向代理',
  'DDNS': '动态域名 DDNS',
  'STUN Tunnel': 'STUN 隧道',
  'Wake-on-LAN': '网络唤醒',
  'Cron Jobs': '定时任务',
  'Certificates': '证书',
  'Storage': '存储',
  'Users': '用户',
  'Audit Log': '审计日志',
  'Webhooks': 'Webhook',
  'Settings': '设置',
  'System Online': '系统在线',

  // Login
  'Sign in to your admin panel': '登录管理面板',
  'Username': '用户名',
  'Password': '密码',
  'Enter username': '请输入用户名',
  'Signing in...': '登录中…',
  'Sign in': '登录',
  'Invalid credentials': '用户名或密码错误',

  // Dashboard
  'Real-time overview of all network services': '所有网络服务的实时总览',
  'STUN Tunnels': 'STUN 隧道',
  'WOL Devices': '网络唤醒设备',
  'STUN Topology': 'STUN 拓扑',
  'Live': '实时',
  'Polling': '轮询中',
  'NAT Type': 'NAT 类型',
  'Public IP': '公网 IP',
  'Mapped Port': '映射端口',
  'STUN Servers': 'STUN 服务器',
  'Type {n}': '类型 {n}',
  'Storage Mounts': '存储挂载',
  'Mounted': '已挂载',
  'Off': '已关闭',
  'Active Forward Connections': '活跃转发连接',
  'Connecting...': '连接中…',
  'Connections': '连接数',
  'In': '入',
  'Out': '出',
  'System Information': '系统信息',
  'Uptime:': '运行时间：',
  'Version:': '版本：',
  'Goroutines:': '协程数：',
  'Memory:': '内存：',
  'Unknown': '未知',
  'Open': '开放',
  'Full Cone': '全锥型',
  'Restricted Cone': '限制锥型',
  'Port Restricted': '端口限制锥型',
  'Symmetric': '对称型',
  // Forward
  'TCP/UDP port forwarding rules for IPv4 and IPv6': '面向 IPv4/IPv6 的 TCP/UDP 端口转发规则',
  'Add Rule': '添加规则',
  'Name': '名称',
  'Rule name': '规则名称',
  'Protocol': '协议',
  'Listen Addr': '监听地址',
  'Listen Port': '监听端口',
  'Target Addr': '目标地址',
  'Target Port': '目标端口',
  'IPv6': 'IPv6',
  'Enabled': '启用',
  'Save Rule': '保存规则',
  'Listen': '监听',
  'Target': '目标',
  'Status': '状态',
  'Actions': '操作',
  'Loading...': '加载中…',
  'No rules yet': '暂无规则',
  'On': '已开启',
  // Proxy
  'HTTP/HTTPS reverse proxy with URL rewrite and access control': '带 URL 重写与访问控制的 HTTP/HTTPS 反向代理',
  'Add Proxy': '添加代理',
  'Proxy name': '代理名称',
  'Domains': '域名',
  'Target URL': '目标 URL',
  'TLS': 'TLS',
  'Force HTTPS': '强制 HTTPS',
  'WebSocket': 'WebSocket',
  'Save Proxy': '保存代理',
  'No proxies yet': '暂无代理',
  'WS': 'WS',
  // DDNS
  'Dynamic DNS': '动态域名 DDNS',
  'Auto-update DNS records for dynamic IP addresses': '为动态 IP 自动更新 DNS 记录',
  'Add DDNS': '添加 DDNS',
  'Config name': '配置名称',
  'Provider': '服务商',
  'Domain': '域名',
  'Subdomain': '子域名',
  'Type': '类型',
  'A (IPv4)': 'A（IPv4）',
  'AAAA (IPv6)': 'AAAA（IPv6）',
  'Interval (s)': '间隔（秒）',
  'Save Config': '保存配置',
  'No DDNS configs': '暂无 DDNS 配置',
  'Interval': '间隔',
  // STUN
  'NAT traversal tunnels for accessing services behind NAT': '用于访问 NAT 后服务的穿透隧道',
  'Add Tunnel': '添加隧道',
  'Tunnel name': '隧道名称',
  'STUN Server': 'STUN 服务器',
  'Local Port': '本地端口',
  'Remote Port': '远端端口',
  'Save': '保存',
  'Local:Remote': '本地:远端',
  'No tunnels': '暂无隧道',
  'Active': '运行中',
  // WOL
  'Remotely wake devices on your network via magic packet': '通过网络魔术包远程唤醒设备',
  'Add Device': '添加设备',
  'Device name': '设备名称',
  'MAC Address': 'MAC 地址',
  'Broadcast': '广播地址',
  'Port': '端口',
  'Save Device': '保存设备',
  'MAC': 'MAC',
  'No devices': '暂无设备',
  'Wake': '唤醒',
  // Cron
  'Schedule commands and module operations': '调度命令与模块操作',
  'Add Job': '添加任务',
  'Job name': '任务名称',
  'Cron Expression': 'Cron 表达式',
  'Shell Command': 'Shell 命令',
  'Module Toggle': '模块开关',
  'Command': '命令',
  'Module': '模块',
  'Save Job': '保存任务',
  'Schedule': '调度',
  'Last Run': '上次运行',
  'No jobs': '暂无任务',
  'Shell': 'Shell',
  'Toggle': '开关',
  'Never': '从未',
  'Paused': '已暂停',
  // ACME
  'SSL Certificates': 'SSL 证书',
  'Automatic SSL/TLS certificate management via ACME': '通过 ACME 自动管理 SSL/TLS 证书',
  'Request Certificate': '申请证书',
  'Cert name': '证书名称',
  'Domains (comma-separated)': '域名（逗号分隔）',
  'Email': '邮箱',
  'DNS Provider': 'DNS 服务商',
  'Auto Renew': '自动续期',
  'Issue Certificate': '签发证书',
  'Expires': '到期时间',
  'No certificates': '暂无证书',
  'Renew': '续期',
  // Storage
  'Network Storage': '网络存储',
  'Mount local or remote storage and serve via FileBrowser, WebDAV, FTP': '挂载本地或远程存储，通过 FileBrowser / WebDAV / FTP 提供服务',
  'Add Mount': '添加挂载',
  'Mount name': '挂载名称',
  'Local Path': '本地路径',
  'AliyunDrive': '阿里云盘',
  'Source': '来源',
  'Services:': '服务：',
  'Services': '服务',
  'FTP Port': 'FTP 端口',
  'Save Mount': '保存挂载',
  'No mounts': '暂无挂载',
  // Users
  'User management': '用户管理',
  'You do not have permission to manage users.': '您没有权限管理用户。',
  'New password for {name} (min 8 characters):': '为 {name} 设置新密码（至少 8 个字符）：',
  'Password must be at least 8 characters': '密码至少需要 8 个字符',
  'Delete user {name}?': '确定删除用户 {name} 吗？',
  'Create and manage user accounts and roles': '创建并管理用户账户与角色',
  'Add User': '添加用户',
  'Role': '角色',
  'Initial Password': '初始密码',
  'min 8 chars': '至少 8 个字符',
  'New users must change their password on first login.': '新用户首次登录时必须修改密码。',
  'No users yet': '暂无用户',
  'you': '你',
  'Reset password': '重置密码',
  'Delete user': '删除用户',
  'Create failed — username may already exist': '创建失败——用户名可能已存在',
  // Audit
  'Security audit trail': '安全审计追踪',
  'You do not have permission to view audit logs.': '您没有权限查看审计日志。',
  'Security audit trail of all mutations': '所有变更操作的安全审计追踪',
  'Action': '操作',
  'All': '全部',
  'Resource': '资源',
  'filter by user': '按用户筛选',
  'Apply': '应用',
  'Time': '时间',
  'User': '用户',
  'Resource ID': '资源 ID',
  'Remote': '来源地址',
  'No audit events': '暂无审计事件',
  'Page {page} of {total} ({count} events)': '第 {page} / {total} 页（共 {count} 条事件）',
  'Prev': '上一页',
  'Next': '下一页',
  // Webhooks
  'Receive NetBerth events at an external URL': '在外部 URL 接收 NetBerth 事件',
  'Add Webhook': '添加 Webhook',
  'Save failed: {message}': '保存失败：{message}',
  'Delete failed: {message}': '删除失败：{message}',
  'Test delivery succeeded': '测试投递成功',
  'Test delivery failed: {message}': '测试投递失败：{message}',
  'Ops webhook': '运维 Webhook',
  'URL': 'URL',
  'Secret (HMAC-SHA256)': '密钥（HMAC-SHA256）',
  'Leave empty to keep': '留空保持不变',
  'Events (comma-separated, empty = all)': '事件（逗号分隔，留空 = 全部）',
  'Save Changes': '保存更改',
  'Save Webhook': '保存 Webhook',
  'Cancel': '取消',
  'No webhooks configured': '尚未配置 Webhook',
  'Set': '已设置',
  'Events': '事件',
  'Secret': '密钥',
  'Test webhook': '测试 Webhook',
  'Edit webhook': '编辑 Webhook',
  'Delete webhook': '删除 Webhook',
  // Settings
  'System configuration and user management': '系统配置与用户管理',
  'Change Password': '修改密码',
  'Update your account password. Minimum 8 characters.': '更新您的账户密码，至少 8 个字符。',
  'Current Password': '当前密码',
  'New Password': '新密码',
  'Confirm Password': '确认密码',
  'Passwords do not match': '两次输入的密码不一致',
  'Password changed successfully': '密码修改成功',
  'Failed to change password': '修改密码失败',
  'Update Password': '更新密码',
  'Two-Factor Authentication': '两步验证（2FA）',
  'Add an extra layer of security to your account.': '为您的账户增加一层安全保护。',
  'Status:': '状态：',
  'Disabled': '已禁用',
  'Disable 2FA': '关闭 2FA',
  'Enable 2FA': '开启 2FA',
  'Setup failed': '设置失败',
  'Secret:': '密钥：',
  'URL:': 'URL：',
  'Scan with Google Authenticator or Authy, then enter the 6-digit code:': '使用 Google Authenticator 或 Authy 扫码，然后输入 6 位验证码：',
  'Verify': '验证',
  '2FA enabled!': '2FA 已开启！',
  'Invalid code': '验证码无效',
  'About NetBerth': '关于 NetBerth',
  'NetBerth is a security-first network service management platform. Port forwarding, reverse proxy, DDNS, STUN, WOL, cron, ACME, and storage — all in one.': 'NetBerth 是安全优先的网络服务管理平台。端口转发、反向代理、DDNS、STUN、WOL、定时任务、ACME 与存储——一应俱全。',
}

const dictionaries: Record<Lang, Record<string, string>> = {
  en: {},
  zh: zhDict,
}

const STORAGE_KEY = 'nb_lang'

type I18nContextValue = {
  lang: Lang
  setLang: (lang: Lang) => void
  t: (key: string, params?: Record<string, string | number>) => string
}

const I18nContext = createContext<I18nContextValue | null>(null)

export function I18nProvider({ children }: { children: ReactNode }) {
  const [lang, setLangState] = useState<Lang>(() => {
    const saved = localStorage.getItem(STORAGE_KEY)
    return saved === 'zh' || saved === 'en' ? saved : 'en'
  })

  const setLang = (next: Lang) => {
    localStorage.setItem(STORAGE_KEY, next)
    setLangState(next)
  }

  useEffect(() => {
    document.documentElement.lang = lang
  }, [lang])

  const t = (key: string, params?: Record<string, string | number>) => {
    let text = dictionaries[lang][key] ?? key
    if (params) {
      for (const [k, v] of Object.entries(params)) {
        text = text.split(`{${k}}`).join(String(v))
      }
    }
    return text
  }

  return (
    <I18nContext.Provider value={{ lang, setLang, t }}>
      {children}
    </I18nContext.Provider>
  )
}

export function useI18n() {
  const ctx = useContext(I18nContext)
  if (!ctx) throw new Error('useI18n must be used within I18nProvider')
  return ctx
}

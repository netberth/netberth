import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { apiClient } from '@/lib/api'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { PageLayout } from '@/components/layout/PageLayout'
import { Badge } from '@/components/ui/badge'
import { ArrowLeftRight, Globe, RefreshCw, Network, Power, Clock, Shield, HardDrive } from 'lucide-react'
import { formatBytes, formatUptime } from '@/lib/utils'
import type { SystemStatus } from '@/types'

const modules = [
  { key: 'forward_rules', label: 'Port Forwarding', icon: ArrowLeftRight, color: 'text-blue-400' },
  { key: 'proxy_rules', label: 'Reverse Proxy', icon: Globe, color: 'text-emerald-400' },
  { key: 'ddns_configs', label: 'DDNS', icon: RefreshCw, color: 'text-violet-400' },
  { key: 'stun_tunnels', label: 'STUN Tunnels', icon: Network, color: 'text-amber-400' },
  { key: 'wol_devices', label: 'WOL Devices', icon: Power, color: 'text-red-400' },
  { key: 'cron_jobs', label: 'Cron Jobs', icon: Clock, color: 'text-cyan-400' },
  { key: 'acme_certificates', label: 'Certificates', icon: Shield, color: 'text-pink-400' },
  { key: 'storage_mounts', label: 'Storage', icon: HardDrive, color: 'text-orange-400' },
]

interface ModuleCounts {
  forward_rules: number
  proxy_rules: number
  ddns_configs: number
  stun_tunnels: number
  wol_devices: number
  cron_jobs: number
  acme_certificates: number
  storage_mounts: number
}

interface DashboardData {
  system: SystemStatus
  modules: ModuleCounts
  storage_mounts: { name: string; enabled: boolean; source: string }[]
}

interface WSStatus {
  system: { uptime: number; cpu_count: number; goroutines: number; memory_mb: number; version: string }
  forward: { id: string; name: string; active: boolean; connections: number; bytes_in: number; bytes_out: number }[]
  stun?: { nat_type: number; mapped_ip: string; mapped_port: number; servers: number }
}

export function Dashboard() {
  const [wsStatus, setWsStatus] = useState<WSStatus | null>(null)
  const [wsConnected, setWsConnected] = useState(false)

  const { data: status } = useQuery({
    queryKey: ['system-status'],
    queryFn: () => apiClient.get<{ data: SystemStatus }>('/system/status'),
    refetchInterval: 10_000,
  })

  const { data: dashboard } = useQuery({
    queryKey: ['dashboard'],
    queryFn: () => apiClient.get<{ data: DashboardData }>('/system/dashboard'),
    refetchInterval: 15_000,
  })

  useEffect(() => {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const ws = new WebSocket(`${protocol}//${window.location.host}/api/v1/ws`)
    ws.onopen = () => setWsConnected(true)
    ws.onclose = () => setWsConnected(false)
    ws.onmessage = (event) => {
      try {
        const msg = JSON.parse(event.data)
        if (msg.type === 'status' && msg.payload) {
          setWsStatus(typeof msg.payload === 'string' ? JSON.parse(msg.payload) : msg.payload)
        }
      } catch {}
    }
    return () => ws.close()
  }, [])

  const sys = status?.data ?? wsStatus?.system
  const mods = dashboard?.data?.modules

  const natLabels: Record<number, string> = { 0:'Unknown', 1:'Open', 2:'Full Cone', 3:'Restricted Cone', 4:'Port Restricted', 5:'Symmetric' }

  return (
    <PageLayout title="Dashboard" description="Real-time overview of all network services">
      {/* Module cards */}
      <div className="grid grid-cols-4 gap-4">
        {modules.map(({ key, label, icon: Icon, color }) => (
          <Card key={key} className="border-border">
            <CardHeader className="flex flex-row items-center justify-between pb-2">
              <CardTitle className="text-sm font-medium text-muted-foreground">{label}</CardTitle>
              <Icon className={`h-4 w-4 ${color}`} />
            </CardHeader>
            <CardContent>
              <div className="text-2xl font-bold">{mods ? (mods as unknown as Record<string, number>)[key] : '—'}</div>
            </CardContent>
          </Card>
        ))}
      </div>

      {/* STUN/NAT Topology */}
      {wsStatus?.stun && (
        <Card className="border-border border-amber-500/20 bg-amber-500/5">
          <CardHeader className="flex flex-row items-center justify-between">
            <CardTitle className="text-base flex items-center gap-2">
              <Network className="h-4 w-4 text-amber-400" /> STUN Topology
            </CardTitle>
            <Badge variant={wsConnected ? 'success' : 'warning'}>{wsConnected ? 'Live' : 'Polling'}</Badge>
          </CardHeader>
          <CardContent>
            <div className="grid grid-cols-4 gap-4 text-sm">
              <div>
                <span className="text-muted-foreground">NAT Type</span>
                <div className="mt-1 font-mono text-amber-400 font-bold text-lg">
                  {natLabels[wsStatus.stun.nat_type] ?? `Type ${wsStatus.stun.nat_type}`}
                </div>
              </div>
              <div>
                <span className="text-muted-foreground">Public IP</span>
                <div className="mt-1 font-mono text-emerald-400">{wsStatus.stun.mapped_ip || '—'}</div>
              </div>
              <div>
                <span className="text-muted-foreground">Mapped Port</span>
                <div className="mt-1 font-mono">{wsStatus.stun.mapped_port || '—'}</div>
              </div>
              <div>
                <span className="text-muted-foreground">STUN Servers</span>
                <div className="mt-1 font-mono">{wsStatus.stun.servers || '—'}</div>
              </div>
            </div>
          </CardContent>
        </Card>
      )}

      {/* Storage Mounts */}
      {dashboard?.data?.storage_mounts && dashboard.data.storage_mounts.length > 0 && (
        <Card className="border-border">
          <CardHeader className="flex flex-row items-center justify-between">
            <CardTitle className="text-base flex items-center gap-2">
              <HardDrive className="h-4 w-4 text-orange-400" /> Storage Mounts
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="grid grid-cols-3 gap-4">
              {dashboard.data.storage_mounts.map((m, i) => (
                <Card key={i} className="border-border bg-muted/30">
                  <CardContent className="pt-4">
                    <div className="flex items-center justify-between">
                      <p className="text-sm font-medium">{m.name}</p>
                      <Badge variant={m.enabled ? 'success' : 'secondary'}>{m.enabled ? 'Mounted' : 'Off'}</Badge>
                    </div>
                    <p className="mt-1 text-xs text-muted-foreground font-mono truncate">{m.source}</p>
                  </CardContent>
                </Card>
              ))}
            </div>
          </CardContent>
        </Card>
      )}

      {/* Live Forward Connections */}
      {wsStatus?.forward && wsStatus.forward.length > 0 && (
        <Card className="border-border">
          <CardHeader className="flex flex-row items-center justify-between">
            <CardTitle className="text-base">Active Forward Connections</CardTitle>
            <Badge variant={wsConnected ? 'success' : 'warning'}>{wsConnected ? 'Live' : 'Connecting...'}</Badge>
          </CardHeader>
          <CardContent>
            <div className="grid grid-cols-4 gap-4">
              {wsStatus.forward.map((f) => (
                <Card key={f.id} className="border-border bg-muted/30">
                  <CardContent className="pt-4">
                    <p className="text-sm font-medium">{f.name || f.id.slice(0, 8)}</p>
                    <div className="mt-2 space-y-1 text-xs text-muted-foreground">
                      <div className="flex justify-between"><span>Connections</span><span className="font-mono text-emerald-400">{f.connections}</span></div>
                      <div className="flex justify-between"><span>In</span><span className="font-mono">{formatBytes(f.bytes_in)}</span></div>
                      <div className="flex justify-between"><span>Out</span><span className="font-mono">{formatBytes(f.bytes_out)}</span></div>
                    </div>
                  </CardContent>
                </Card>
              ))}
            </div>
          </CardContent>
        </Card>
      )}

      {/* System Info */}
      {sys && (
        <Card className="border-border">
          <CardHeader><CardTitle className="text-base">System Information</CardTitle></CardHeader>
          <CardContent>
            <div className="grid grid-cols-4 gap-6 text-sm">
              <div><span className="text-muted-foreground">Uptime:</span> <span className="ml-2 font-mono">{formatUptime(typeof sys.uptime === 'number' ? sys.uptime : 0)}</span></div>
              <div><span className="text-muted-foreground">Version:</span> <span className="ml-2 font-mono">{String((sys as unknown as Record<string,unknown>).version ?? '—')}</span></div>
              <div><span className="text-muted-foreground">Goroutines:</span> <span className="ml-2 font-mono">{String((sys as unknown as Record<string,unknown>).goroutines ?? '—')}</span></div>
              <div><span className="text-muted-foreground">Memory:</span> <span className="ml-2 font-mono">{sys.memory_mb ?? '—'} MB</span></div>
            </div>
          </CardContent>
        </Card>
      )}
    </PageLayout>
  )
}

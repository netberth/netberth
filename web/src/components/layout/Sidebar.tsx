import { NavLink } from 'react-router-dom'
import { cn } from '@/lib/utils'
import {
  LayoutDashboard, ArrowLeftRight, Globe, RefreshCw, Network,
  Power, Clock, Shield, HardDrive, Settings, Anchor
} from 'lucide-react'

const navItems = [
  { to: '/', icon: LayoutDashboard, label: 'Dashboard' },
  { to: '/forward', icon: ArrowLeftRight, label: 'Port Forwarding' },
  { to: '/proxy', icon: Globe, label: 'Reverse Proxy' },
  { to: '/ddns', icon: RefreshCw, label: 'DDNS' },
  { to: '/stun', icon: Network, label: 'STUN Tunnel' },
  { to: '/wol', icon: Power, label: 'Wake-on-LAN' },
  { to: '/cron', icon: Clock, label: 'Cron Jobs' },
  { to: '/acme', icon: Shield, label: 'Certificates' },
  { to: '/storage', icon: HardDrive, label: 'Storage' },
  { to: '/settings', icon: Settings, label: 'Settings' },
]

export function Sidebar() {
  return (
    <aside className="fixed left-0 top-0 z-40 h-screen w-56 border-r border-border bg-card">
      <div className="flex h-14 items-center gap-2 border-b border-border px-4">
        <Anchor className="h-6 w-6 text-primary" />
        <span className="text-lg font-bold tracking-tight">NetBerth</span>
      </div>
      <nav className="space-y-1 p-3">
        {navItems.map(({ to, icon: Icon, label }) => (
          <NavLink
            key={to}
            to={to}
            end={to === '/'}
            className={({ isActive }) =>
              cn(
                'flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors',
                isActive
                  ? 'bg-primary/10 text-primary'
                  : 'text-muted-foreground hover:bg-accent hover:text-foreground'
              )
            }
          >
            <Icon className="h-4 w-4" />
            {label}
          </NavLink>
        ))}
      </nav>
      <div className="absolute bottom-0 left-0 right-0 border-t border-border p-3">
        <div className="text-xs text-muted-foreground">NetBerth v1.0.0-rc1</div>
      </div>
    </aside>
  )
}

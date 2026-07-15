import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { apiClient } from '@/lib/api'
import { PageLayout } from '@/components/layout/PageLayout'
import { Card, CardContent } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Badge } from '@/components/ui/badge'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Plus, Trash2 } from 'lucide-react'
import type { DDNSConfig } from '@/types'

const DDNS_PROVIDERS = ['aliyun', 'tencent', 'cloudflare', 'dnspod', 'godaddy', 'huawei', 'noip', 'dynv6', 'he', 'duckdns']

export function DDNS() {
  const queryClient = useQueryClient()
  const [form, setForm] = useState({ name: '', provider: 'cloudflare', domain: '', sub_domain: '@', record_type: 'A', interval: 300, enabled: false })

  const { data, isLoading } = useQuery({
    queryKey: ['ddns'],
    queryFn: () => apiClient.get<{ data: DDNSConfig[] }>('/ddns'),
  })

  const createMutation = useMutation({
    mutationFn: (cfg: Record<string, unknown>) => apiClient.post('/ddns', { ...cfg, credentials: {}, get_ip_url: '', get_ip_type: 'url', ttl: 600 }),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['ddns'] }); setForm({ name: '', provider: 'cloudflare', domain: '', sub_domain: '@', record_type: 'A', interval: 300, enabled: false }) },
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => apiClient.delete(`/ddns/${id}`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['ddns'] }),
  })

  return (
    <PageLayout title="Dynamic DNS" description="Auto-update DNS records for dynamic IP addresses" actions={
      <Button size="sm"><Plus className="mr-1 h-4 w-4" /> Add DDNS</Button>
    }>
      <Card className="border-border">
        <CardContent className="pt-6">
          <div className="grid grid-cols-5 gap-3">
            <div><Label>Name</Label><Input value={form.name} onChange={e => setForm({...form, name: e.target.value})} placeholder="Config name" /></div>
            <div>
              <Label>Provider</Label>
              <select className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                value={form.provider} onChange={e => setForm({...form, provider: e.target.value})}>
                {DDNS_PROVIDERS.map(p => <option key={p} value={p}>{p}</option>)}
              </select>
            </div>
            <div><Label>Domain</Label><Input value={form.domain} onChange={e => setForm({...form, domain: e.target.value})} placeholder="example.com" /></div>
            <div><Label>Subdomain</Label><Input value={form.sub_domain} onChange={e => setForm({...form, sub_domain: e.target.value})} placeholder="@" /></div>
            <div>
              <Label>Type</Label>
              <select className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                value={form.record_type} onChange={e => setForm({...form, record_type: e.target.value})}>
                <option value="A">A (IPv4)</option><option value="AAAA">AAAA (IPv6)</option>
              </select>
            </div>
          </div>
          <div className="mt-3 flex items-center gap-6">
            <div className="flex items-center gap-2"><Label>Interval (s)</Label><Input className="w-24" type="number" value={form.interval} onChange={e => setForm({...form, interval: +e.target.value})} /></div>
            <div className="flex items-center gap-2"><Switch checked={form.enabled} onCheckedChange={v => setForm({...form, enabled: v})} /><Label>Enabled</Label></div>
            <Button onClick={() => createMutation.mutate(form as unknown as Record<string, unknown>)} size="sm">Save Config</Button>
          </div>
        </CardContent>
      </Card>
      <Card className="border-border">
        <CardContent className="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead><TableHead>Provider</TableHead><TableHead>Domain</TableHead><TableHead>Type</TableHead><TableHead>Interval</TableHead><TableHead>Status</TableHead><TableHead className="w-[100px]">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {isLoading ? (
                <TableRow><TableCell colSpan={7} className="text-center text-muted-foreground">Loading...</TableCell></TableRow>
              ) : (data?.data ?? []).length === 0 ? (
                <TableRow><TableCell colSpan={7} className="text-center text-muted-foreground">No DDNS configs</TableCell></TableRow>
              ) : (
                (data?.data ?? []).map((cfg) => (
                  <TableRow key={cfg.id}>
                    <TableCell className="font-medium">{cfg.name}</TableCell>
                    <TableCell><Badge variant="secondary">{cfg.provider}</Badge></TableCell>
                    <TableCell className="font-mono text-xs">{cfg.sub_domain}.{cfg.domain}</TableCell>
                    <TableCell><Badge variant="outline">{cfg.record_type}</Badge></TableCell>
                    <TableCell>{cfg.interval}s</TableCell>
                    <TableCell><Switch checked={cfg.enabled} /></TableCell>
                    <TableCell>
                      <Button variant="ghost" size="icon" onClick={() => deleteMutation.mutate(cfg.id)}><Trash2 className="h-4 w-4 text-muted-foreground" /></Button>
                    </TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </PageLayout>
  )
}

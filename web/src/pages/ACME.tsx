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
import { Plus, Trash2, Shield, RefreshCw } from 'lucide-react'
import { statusColor } from '@/lib/utils'
import type { ACMECertificate } from '@/types'

export function ACME() {
  const queryClient = useQueryClient()
  const [form, setForm] = useState({ name: '', domains: '', provider: 'letsencrypt', dns_provider: 'cloudflare', email: '', auto_renew: true })

  const { data, isLoading } = useQuery({
    queryKey: ['acme'],
    queryFn: () => apiClient.get<{ data: ACMECertificate[] }>('/acme'),
  })

  const createMutation = useMutation({
    mutationFn: (c: Record<string, unknown>) => apiClient.post('/acme', {
      ...c, domains: (c.domains as string).split(',').map((d: string) => d.trim()).filter(Boolean),
      dns_config: {}, renew_days: 30,
    }),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['acme'] }); setForm({ name: '', domains: '', provider: 'letsencrypt', dns_provider: 'cloudflare', email: '', auto_renew: true }) },
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => apiClient.delete(`/acme/${id}`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['acme'] }),
  })

  const renewMutation = useMutation({
    mutationFn: (id: string) => apiClient.post(`/acme/${id}/renew`),
    onSuccess: () => {},
  })

  return (
    <PageLayout title="SSL Certificates" description="Automatic SSL/TLS certificate management via ACME" actions={
      <Button size="sm"><Plus className="mr-1 h-4 w-4" /> Request Certificate</Button>
    }>
      <Card className="border-border">
        <CardContent className="pt-6">
          <div className="grid grid-cols-3 gap-3">
            <div><Label>Name</Label><Input value={form.name} onChange={e => setForm({...form, name: e.target.value})} placeholder="Cert name" /></div>
            <div><Label>Domains (comma-separated)</Label><Input value={form.domains} onChange={e => setForm({...form, domains: e.target.value})} placeholder="example.com, *.example.com" /></div>
            <div><Label>Email</Label><Input value={form.email} onChange={e => setForm({...form, email: e.target.value})} placeholder="admin@example.com" /></div>
          </div>
          <div className="mt-3 grid grid-cols-3 gap-3">
            <div>
              <Label>Provider</Label>
              <select className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                value={form.provider} onChange={e => setForm({...form, provider: e.target.value})}>
                <option value="letsencrypt">Let's Encrypt</option><option value="zerossl">ZeroSSL</option>
              </select>
            </div>
            <div>
              <Label>DNS Provider</Label>
              <select className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                value={form.dns_provider} onChange={e => setForm({...form, dns_provider: e.target.value})}>
                <option value="cloudflare">Cloudflare</option><option value="aliyun">Aliyun</option><option value="dnspod">DNSPod</option>
              </select>
            </div>
            <div className="flex items-end pb-1 gap-6">
              <div className="flex items-center gap-2"><Switch checked={form.auto_renew} onCheckedChange={v => setForm({...form, auto_renew: v})} /><Label>Auto Renew</Label></div>
              <Button onClick={() => createMutation.mutate(form as unknown as Record<string, unknown>)} size="sm">Issue Certificate</Button>
            </div>
          </div>
        </CardContent>
      </Card>
      <Card className="border-border">
        <CardContent className="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead><TableHead>Domains</TableHead><TableHead>Provider</TableHead><TableHead>Expires</TableHead><TableHead>Status</TableHead><TableHead className="w-[120px]">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {isLoading ? (
                <TableRow><TableCell colSpan={6} className="text-center text-muted-foreground">Loading...</TableCell></TableRow>
              ) : (data?.data ?? []).length === 0 ? (
                <TableRow><TableCell colSpan={6} className="text-center text-muted-foreground">No certificates</TableCell></TableRow>
              ) : (
                (data?.data ?? []).map((c) => (
                  <TableRow key={c.id}>
                    <TableCell className="font-medium"><Shield className="mr-2 inline h-4 w-4" />{c.name}</TableCell>
                    <TableCell><div className="flex flex-wrap gap-1">{(c.domains ?? []).map(d => <Badge key={d} variant="secondary" className="text-xs">{d}</Badge>)}</div></TableCell>
                    <TableCell><Badge variant="outline">{c.provider}</Badge></TableCell>
                    <TableCell className="text-xs">{c.expires_at ? new Date(c.expires_at).toLocaleDateString() : '—'}</TableCell>
                    <TableCell><Badge className={statusColor(c.status)}>{c.status}</Badge></TableCell>
                    <TableCell>
                      <div className="flex gap-1">
                        {c.status === 'valid' && <Button variant="outline" size="icon" onClick={() => renewMutation.mutate(c.id)} title="Renew"><RefreshCw className="h-4 w-4" /></Button>}
                        <Button variant="ghost" size="icon" onClick={() => deleteMutation.mutate(c.id)}><Trash2 className="h-4 w-4 text-muted-foreground" /></Button>
                      </div>
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

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
import type { ProxyRule, PaginatedResponse } from '@/types'

export function Proxy() {
  const queryClient = useQueryClient()
  const [page] = useState(1)
  const [form, setForm] = useState({ name: '', domains: '', target_url: '', tls_enabled: false, force_https: false, websocket: false, enabled: false })

  const { data, isLoading } = useQuery({
    queryKey: ['proxy-rules', page],
    queryFn: () => apiClient.get<PaginatedResponse<ProxyRule>>(`/proxy-rules?page=${page}&page_size=20`),
  })

  const createMutation = useMutation({
    mutationFn: (rule: Record<string, unknown>) => apiClient.post('/proxy-rules', {
      ...rule,
      domains: (rule.domains as string).split(',').map((d: string) => d.trim()).filter(Boolean),
      ip_whitelist: [], ip_blacklist: [], ua_whitelist: [], ua_blacklist: [],
    }),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['proxy-rules'] }); setForm({ name: '', domains: '', target_url: '', tls_enabled: false, force_https: false, websocket: false, enabled: false }) },
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => apiClient.delete(`/proxy-rules/${id}`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['proxy-rules'] }),
  })

  const toggleMutation = useMutation({
    mutationFn: ({ id, ...rest }: ProxyRule) => apiClient.put(`/proxy-rules/${id}`, { ...rest, enabled: !rest.enabled }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['proxy-rules'] }),
  })

  return (
    <PageLayout title="Reverse Proxy" description="HTTP/HTTPS reverse proxy with URL rewrite and access control" actions={
      <Button onClick={() => {}} size="sm"><Plus className="mr-1 h-4 w-4" /> Add Proxy</Button>
    }>
      <Card className="border-border">
        <CardContent className="pt-6">
          <div className="grid grid-cols-4 gap-3">
            <div><Label>Name</Label><Input value={form.name} onChange={e => setForm({...form, name: e.target.value})} placeholder="Proxy name" /></div>
            <div><Label>Domains</Label><Input value={form.domains} onChange={e => setForm({...form, domains: e.target.value})} placeholder="example.com, *.example.com" /></div>
            <div><Label>Target URL</Label><Input value={form.target_url} onChange={e => setForm({...form, target_url: e.target.value})} placeholder="http://localhost:3000" /></div>
          </div>
          <div className="mt-3 flex items-center gap-6">
            <div className="flex items-center gap-2"><Switch checked={form.tls_enabled} onCheckedChange={v => setForm({...form, tls_enabled: v})} /><Label>TLS</Label></div>
            <div className="flex items-center gap-2"><Switch checked={form.force_https} onCheckedChange={v => setForm({...form, force_https: v})} /><Label>Force HTTPS</Label></div>
            <div className="flex items-center gap-2"><Switch checked={form.websocket} onCheckedChange={v => setForm({...form, websocket: v})} /><Label>WebSocket</Label></div>
            <div className="flex items-center gap-2"><Switch checked={form.enabled} onCheckedChange={v => setForm({...form, enabled: v})} /><Label>Enabled</Label></div>
            <Button onClick={() => createMutation.mutate(form as unknown as Record<string, unknown>)} size="sm">Save Proxy</Button>
          </div>
        </CardContent>
      </Card>
      <Card className="border-border">
        <CardContent className="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead><TableHead>Domains</TableHead><TableHead>Target</TableHead><TableHead>TLS</TableHead><TableHead>WS</TableHead><TableHead>Status</TableHead><TableHead className="w-[100px]">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {isLoading ? (
                <TableRow><TableCell colSpan={7} className="text-center text-muted-foreground">Loading...</TableCell></TableRow>
              ) : data?.data?.length === 0 ? (
                <TableRow><TableCell colSpan={7} className="text-center text-muted-foreground">No proxies yet</TableCell></TableRow>
              ) : (
                data?.data?.map((rule) => (
                  <TableRow key={rule.id}>
                    <TableCell className="font-medium">{rule.name}</TableCell>
                    <TableCell><div className="flex flex-wrap gap-1">{rule.domains?.map(d => <Badge key={d} variant="secondary" className="text-xs">{d}</Badge>)}</div></TableCell>
                    <TableCell className="font-mono text-xs">{rule.target_url}</TableCell>
                    <TableCell>{rule.tls_enabled ? <Badge variant="success">On</Badge> : <Badge variant="secondary">Off</Badge>}</TableCell>
                    <TableCell>{rule.websocket ? <Badge variant="success">On</Badge> : <Badge variant="secondary">Off</Badge>}</TableCell>
                    <TableCell><Switch checked={rule.enabled} onCheckedChange={() => toggleMutation.mutate(rule)} /></TableCell>
                    <TableCell>
                      <Button variant="ghost" size="icon" onClick={() => deleteMutation.mutate(rule.id)}><Trash2 className="h-4 w-4 text-muted-foreground" /></Button>
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

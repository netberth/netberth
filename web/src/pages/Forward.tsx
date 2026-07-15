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
import { Plus, Trash2, Edit3 } from 'lucide-react'
import type { ForwardRule, PaginatedResponse } from '@/types'

export function Forward() {
  const queryClient = useQueryClient()
  const [page] = useState(1)
  const [editing, setEditing] = useState<ForwardRule | null>(null)
  const [form, setForm] = useState<{ name: string; protocol: 'tcp'|'udp'|'both'; listen_addr: string; listen_port: number; target_addr: string; target_port: number; enable_ipv6: boolean; enabled: boolean }>({ name: '', protocol: 'tcp', listen_addr: '', listen_port: 0, target_addr: '', target_port: 0, enable_ipv6: true, enabled: false })

  const { data, isLoading } = useQuery({
    queryKey: ['forward-rules', page],
    queryFn: () => apiClient.get<PaginatedResponse<ForwardRule>>(`/forward-rules?page=${page}&page_size=20`),
  })

  const createMutation = useMutation({
    mutationFn: (rule: Partial<ForwardRule>) => apiClient.post('/forward-rules', rule),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['forward-rules'] }); resetForm() },
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => apiClient.delete(`/forward-rules/${id}`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['forward-rules'] }),
  })

  const toggleMutation = useMutation({
    mutationFn: ({ id, ...rest }: ForwardRule) => apiClient.put(`/forward-rules/${id}`, { ...rest, enabled: !rest.enabled }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['forward-rules'] }),
  })

  function resetForm() {
    setEditing(null)
    setForm({ name: '', protocol: 'tcp', listen_addr: '', listen_port: 0, target_addr: '', target_port: 0, enable_ipv6: true, enabled: false })
  }

  return (
    <PageLayout title="Port Forwarding" description="TCP/UDP port forwarding rules for IPv4 and IPv6" actions={
      <Button onClick={() => { resetForm() }} size="sm"><Plus className="mr-1 h-4 w-4" /> Add Rule</Button>
    }>
      {/* Form */}
      <Card className="border-border">
        <CardContent className="pt-6">
          <div className="grid grid-cols-6 gap-3">
            <div><Label>Name</Label><Input value={form.name} onChange={e => setForm({...form, name: e.target.value})} placeholder="Rule name" /></div>
            <div>
              <Label>Protocol</Label>
              <select className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                value={form.protocol} onChange={e => setForm({...form, protocol: e.target.value as 'tcp'|'udp'|'both'})}>
                <option value="tcp">TCP</option><option value="udp">UDP</option><option value="both">TCP+UDP</option>
              </select>
            </div>
            <div><Label>Listen Addr</Label><Input value={form.listen_addr} onChange={e => setForm({...form, listen_addr: e.target.value})} placeholder="0.0.0.0" /></div>
            <div><Label>Listen Port</Label><Input type="number" value={form.listen_port || ''} onChange={e => setForm({...form, listen_port: +e.target.value})} placeholder="8080" /></div>
            <div><Label>Target Addr</Label><Input value={form.target_addr} onChange={e => setForm({...form, target_addr: e.target.value})} placeholder="192.168.1.100" /></div>
            <div><Label>Target Port</Label><Input type="number" value={form.target_port || ''} onChange={e => setForm({...form, target_port: +e.target.value})} placeholder="80" /></div>
          </div>
          <div className="mt-3 flex items-center gap-6">
            <div className="flex items-center gap-2"><Switch checked={form.enable_ipv6} onCheckedChange={v => setForm({...form, enable_ipv6: v})} /><Label>IPv6</Label></div>
            <div className="flex items-center gap-2"><Switch checked={form.enabled} onCheckedChange={v => setForm({...form, enabled: v})} /><Label>Enabled</Label></div>
            <Button onClick={() => createMutation.mutate(form)} size="sm">Save Rule</Button>
          </div>
        </CardContent>
      </Card>
      {/* Table */}
      <Card className="border-border">
        <CardContent className="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead><TableHead>Protocol</TableHead><TableHead>Listen</TableHead><TableHead>Target</TableHead><TableHead>IPv6</TableHead><TableHead>Status</TableHead><TableHead className="w-[100px]">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {isLoading ? (
                <TableRow><TableCell colSpan={7} className="text-center text-muted-foreground">Loading...</TableCell></TableRow>
              ) : data?.data?.length === 0 ? (
                <TableRow><TableCell colSpan={7} className="text-center text-muted-foreground">No rules yet</TableCell></TableRow>
              ) : (
                data?.data?.map((rule) => (
                  <TableRow key={rule.id}>
                    <TableCell className="font-medium">{rule.name}</TableCell>
                    <TableCell><Badge variant="secondary">{rule.protocol.toUpperCase()}</Badge></TableCell>
                    <TableCell className="font-mono text-xs">{rule.listen_addr}:{rule.listen_port}</TableCell>
                    <TableCell className="font-mono text-xs">{rule.target_addr}:{rule.target_port}</TableCell>
                    <TableCell>{rule.enable_ipv6 ? <Badge variant="success">On</Badge> : <Badge variant="secondary">Off</Badge>}</TableCell>
                    <TableCell>
                      <Switch checked={rule.enabled} onCheckedChange={() => toggleMutation.mutate(rule)} />
                    </TableCell>
                    <TableCell>
                      <div className="flex gap-1">
                        <Button variant="ghost" size="icon" onClick={() => deleteMutation.mutate(rule.id)}><Trash2 className="h-4 w-4 text-muted-foreground" /></Button>
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

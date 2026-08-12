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
import { Plus, Trash2, Send, Pencil } from 'lucide-react'
import type { WebhookEndpoint } from '@/types'

const emptyForm = { name: '', url: '', secret: '', events: '', enabled: true }

export function Webhooks() {
  const queryClient = useQueryClient()
  const [form, setForm] = useState(emptyForm)
  const [editingId, setEditingId] = useState<string | null>(null)
  const [notice, setNotice] = useState<string | null>(null)

  const { data, isLoading } = useQuery({
    queryKey: ['webhooks'],
    queryFn: () => apiClient.get<{ data: WebhookEndpoint[] }>('/webhooks'),
  })

  const reset = () => { setForm(emptyForm); setEditingId(null); setNotice(null) }

  const saveMutation = useMutation({
    mutationFn: (payload: Record<string, unknown>) =>
      editingId
        ? apiClient.put(`/webhooks/${editingId}`, payload)
        : apiClient.post('/webhooks', payload),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['webhooks'] })
      reset()
    },
    onError: (e: Error) => setNotice(`Save failed: ${e.message}`),
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => apiClient.delete(`/webhooks/${id}`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['webhooks'] }),
    onError: (e: Error) => setNotice(`Delete failed: ${e.message}`),
  })

  const testMutation = useMutation({
    mutationFn: (id: string) => apiClient.post(`/webhooks/${id}/test`),
    onSuccess: () => setNotice('Test delivery succeeded'),
    onError: (e: Error) => setNotice(`Test delivery failed: ${e.message}`),
  })

  const startEdit = (w: WebhookEndpoint) => {
    setEditingId(w.id)
    setForm({ name: w.name, url: w.url, secret: '', events: w.events.join(','), enabled: w.enabled })
    setNotice(null)
  }

  const submit = () => {
    const events = form.events.split(',').map(s => s.trim()).filter(Boolean)
    saveMutation.mutate({
      name: form.name,
      url: form.url,
      secret: form.secret,
      events,
      enabled: form.enabled,
    })
  }

  return (
    <PageLayout title="Webhooks" description="Receive NetBerth events at an external URL" actions={
      <Button size="sm" onClick={reset}><Plus className="mr-1 h-4 w-4" /> Add Webhook</Button>
    }>
      {notice && <div className="mb-3 rounded-md border border-border bg-card px-3 py-2 text-sm">{notice}</div>}
      <Card className="border-border">
        <CardContent className="pt-6">
          <div className="grid grid-cols-4 gap-3">
            <div><Label>Name</Label><Input value={form.name} onChange={e => setForm({...form, name: e.target.value})} placeholder="Ops webhook" /></div>
            <div className="col-span-2"><Label>URL</Label><Input value={form.url} onChange={e => setForm({...form, url: e.target.value})} placeholder="https://hooks.example.com/ingest" /></div>
            <div><Label>Secret (HMAC-SHA256)</Label><Input type="password" value={form.secret} onChange={e => setForm({...form, secret: e.target.value})} placeholder={editingId ? 'Leave empty to keep' : ''} /></div>
            <div className="col-span-3"><Label>Events (comma-separated, empty = all)</Label><Input value={form.events} onChange={e => setForm({...form, events: e.target.value})} placeholder="forward:created, proxy:deleted, cron:updated" /></div>
            <div className="flex items-end gap-4 pb-1">
              <div className="flex items-center gap-2"><Switch checked={form.enabled} onCheckedChange={v => setForm({...form, enabled: v})} /><Label>Enabled</Label></div>
              <Button onClick={submit} size="sm" disabled={saveMutation.isPending}>{editingId ? 'Save Changes' : 'Save Webhook'}</Button>
              {editingId && <Button onClick={reset} size="sm" variant="ghost">Cancel</Button>}
            </div>
          </div>
        </CardContent>
      </Card>
      <Card className="border-border">
        <CardContent className="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead><TableHead>URL</TableHead><TableHead>Events</TableHead><TableHead>Secret</TableHead><TableHead>Status</TableHead><TableHead className="w-[140px]"></TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {isLoading ? (
                <TableRow><TableCell colSpan={6} className="text-center text-muted-foreground">Loading...</TableCell></TableRow>
              ) : (data?.data ?? []).length === 0 ? (
                <TableRow><TableCell colSpan={6} className="text-center text-muted-foreground">No webhooks configured</TableCell></TableRow>
              ) : (
                (data?.data ?? []).map((w) => (
                  <TableRow key={w.id}>
                    <TableCell className="font-medium">{w.name}</TableCell>
                    <TableCell className="max-w-[260px] truncate font-mono text-xs">{w.url}</TableCell>
                    <TableCell className="max-w-[240px] truncate text-xs">{w.events.length === 0 ? <Badge variant="secondary">All</Badge> : w.events.join(', ')}</TableCell>
                    <TableCell>{w.has_secret ? <Badge variant="success">Set</Badge> : <span className="text-xs text-muted-foreground">—</span>}</TableCell>
                    <TableCell>{w.enabled ? <Badge variant="success">Active</Badge> : <Badge variant="secondary">Paused</Badge>}</TableCell>
                    <TableCell>
                      <div className="flex items-center gap-1">
                        <Button variant="ghost" size="icon" onClick={() => testMutation.mutate(w.id)} aria-label="Test webhook"><Send className="h-4 w-4 text-muted-foreground" /></Button>
                        <Button variant="ghost" size="icon" onClick={() => startEdit(w)} aria-label="Edit webhook"><Pencil className="h-4 w-4 text-muted-foreground" /></Button>
                        <Button variant="ghost" size="icon" onClick={() => deleteMutation.mutate(w.id)} aria-label="Delete webhook"><Trash2 className="h-4 w-4 text-muted-foreground" /></Button>
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

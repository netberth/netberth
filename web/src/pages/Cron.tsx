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
import { Plus, Trash2, Clock } from 'lucide-react'
import type { CronJob } from '@/types'

export function Cron() {
  const queryClient = useQueryClient()
  const [form, setForm] = useState({ name: '', schedule: '*/5 * * * *', type: 'command', command: '', enabled: true })

  const { data, isLoading } = useQuery({
    queryKey: ['cron'],
    queryFn: () => apiClient.get<{ data: CronJob[] }>('/cron'),
  })

  const createMutation = useMutation({
    mutationFn: (j: Record<string, unknown>) => apiClient.post('/cron', j),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['cron'] }); setForm({ name: '', schedule: '*/5 * * * *', type: 'command', command: '', enabled: true }) },
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => apiClient.delete(`/cron/${id}`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['cron'] }),
  })

  return (
    <PageLayout title="Cron Jobs" description="Schedule commands and module operations" actions={
      <Button size="sm"><Plus className="mr-1 h-4 w-4" /> Add Job</Button>
    }>
      <Card className="border-border">
        <CardContent className="pt-6">
          <div className="grid grid-cols-4 gap-3">
            <div><Label>Name</Label><Input value={form.name} onChange={e => setForm({...form, name: e.target.value})} placeholder="Job name" /></div>
            <div><Label>Cron Expression</Label><Input value={form.schedule} onChange={e => setForm({...form, schedule: e.target.value})} placeholder="*/5 * * * *" /></div>
            <div>
              <Label>Type</Label>
              <select className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                value={form.type} onChange={e => setForm({...form, type: e.target.value})}>
                <option value="command">Shell Command</option><option value="module_toggle">Module Toggle</option>
              </select>
            </div>
            <div><Label>{form.type === 'command' ? 'Command' : 'Module'}</Label><Input value={form.command} onChange={e => setForm({...form, command: e.target.value})} placeholder={form.type === 'command' ? 'curl http://...' : ''} /></div>
          </div>
          <div className="mt-3 flex items-center gap-6">
            <div className="flex items-center gap-2"><Switch checked={form.enabled} onCheckedChange={v => setForm({...form, enabled: v})} /><Label>Enabled</Label></div>
            <Button onClick={() => createMutation.mutate(form as unknown as Record<string, unknown>)} size="sm">Save Job</Button>
          </div>
        </CardContent>
      </Card>
      <Card className="border-border">
        <CardContent className="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead><TableHead>Schedule</TableHead><TableHead>Type</TableHead><TableHead>Command</TableHead><TableHead>Last Run</TableHead><TableHead>Status</TableHead><TableHead className="w-[80px]"></TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {isLoading ? (
                <TableRow><TableCell colSpan={7} className="text-center text-muted-foreground">Loading...</TableCell></TableRow>
              ) : (data?.data ?? []).length === 0 ? (
                <TableRow><TableCell colSpan={7} className="text-center text-muted-foreground">No jobs</TableCell></TableRow>
              ) : (
                (data?.data ?? []).map((j) => (
                  <TableRow key={j.id}>
                    <TableCell className="font-medium">{j.name}</TableCell>
                    <TableCell className="font-mono text-xs"><Clock className="mr-1 inline h-3 w-3" />{j.schedule}</TableCell>
                    <TableCell><Badge variant="secondary">{j.type === 'command' ? 'Shell' : 'Toggle'}</Badge></TableCell>
                    <TableCell className="font-mono text-xs max-w-[200px] truncate">{j.command || '—'}</TableCell>
                    <TableCell className="text-xs text-muted-foreground">{j.last_run ? new Date(j.last_run).toLocaleString() : 'Never'}</TableCell>
                    <TableCell>{j.enabled ? <Badge variant="success">Active</Badge> : <Badge variant="secondary">Paused</Badge>}</TableCell>
                    <TableCell><Button variant="ghost" size="icon" onClick={() => deleteMutation.mutate(j.id)}><Trash2 className="h-4 w-4 text-muted-foreground" /></Button></TableCell>
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

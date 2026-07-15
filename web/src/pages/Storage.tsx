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
import { Plus, Trash2, HardDrive } from 'lucide-react'
import type { StorageMount } from '@/types'

export function Storage() {
  const queryClient = useQueryClient()
  const [form, setForm] = useState({ name: '', type: 'local', source: '', services: ['filebrowser'], ftp_port: 2121, enabled: false, username: '', password: '' })

  const { data, isLoading } = useQuery({
    queryKey: ['storage'],
    queryFn: () => apiClient.get<{ data: StorageMount[] }>('/storage'),
  })

  const createMutation = useMutation({
    mutationFn: (m: Record<string, unknown>) => apiClient.post('/storage', m),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['storage'] }); setForm({ name: '', type: 'local', source: '', services: ['filebrowser'], ftp_port: 2121, enabled: false, username: '', password: '' }) },
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => apiClient.delete(`/storage/${id}`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['storage'] }),
  })

  const toggleService = (svc: string) => {
    const current = form.services
    const next = current.includes(svc) ? current.filter(s => s !== svc) : [...current, svc]
    setForm({...form, services: next})
  }

  return (
    <PageLayout title="Network Storage" description="Mount local or remote storage and serve via FileBrowser, WebDAV, FTP" actions={
      <Button size="sm"><Plus className="mr-1 h-4 w-4" /> Add Mount</Button>
    }>
      <Card className="border-border">
        <CardContent className="pt-6">
          <div className="grid grid-cols-3 gap-3">
            <div><Label>Name</Label><Input value={form.name} onChange={e => setForm({...form, name: e.target.value})} placeholder="Mount name" /></div>
            <div>
              <Label>Type</Label>
              <select className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                value={form.type} onChange={e => setForm({...form, type: e.target.value})}>
                <option value="local">Local Path</option><option value="webdav">WebDAV</option><option value="aliyundrive">AliyunDrive</option>
              </select>
            </div>
            <div><Label>Source</Label><Input value={form.source} onChange={e => setForm({...form, source: e.target.value})} placeholder={form.type === 'local' ? '/mnt/data' : 'https://...'} /></div>
          </div>
          {form.type !== 'local' && (
            <div className="mt-3 grid grid-cols-2 gap-3">
              <div><Label>Username</Label><Input value={form.username} onChange={e => setForm({...form, username: e.target.value})} /></div>
              <div><Label>Password</Label><Input type="password" value={form.password} onChange={e => setForm({...form, password: e.target.value})} /></div>
            </div>
          )}
          <div className="mt-3 flex flex-wrap items-center gap-4">
            <div className="flex items-center gap-2 text-sm">
              <Label>Services:</Label>
              {['filebrowser', 'webdav', 'ftp'].map(svc => (
                <Badge key={svc} variant={form.services.includes(svc) ? 'default' : 'outline'}
                  className="cursor-pointer" onClick={() => toggleService(svc)}>{svc}</Badge>
              ))}
            </div>
            <div className="flex items-center gap-2"><Label>FTP Port</Label><Input className="w-24" type="number" value={form.ftp_port} onChange={e => setForm({...form, ftp_port: +e.target.value})} /></div>
            <div className="flex items-center gap-2"><Switch checked={form.enabled} onCheckedChange={v => setForm({...form, enabled: v})} /><Label>Enabled</Label></div>
            <Button onClick={() => createMutation.mutate(form as unknown as Record<string, unknown>)} size="sm">Save Mount</Button>
          </div>
        </CardContent>
      </Card>
      <Card className="border-border">
        <CardContent className="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead><TableHead>Type</TableHead><TableHead>Source</TableHead><TableHead>Services</TableHead><TableHead>Status</TableHead><TableHead className="w-[80px]"></TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {isLoading ? (
                <TableRow><TableCell colSpan={6} className="text-center text-muted-foreground">Loading...</TableCell></TableRow>
              ) : (data?.data ?? []).length === 0 ? (
                <TableRow><TableCell colSpan={6} className="text-center text-muted-foreground">No mounts</TableCell></TableRow>
              ) : (
                (data?.data ?? []).map((m) => (
                  <TableRow key={m.id}>
                    <TableCell className="font-medium"><HardDrive className="mr-2 inline h-4 w-4" />{m.name}</TableCell>
                    <TableCell><Badge variant="outline">{m.type}</Badge></TableCell>
                    <TableCell className="font-mono text-xs max-w-[200px] truncate">{m.source}</TableCell>
                    <TableCell><div className="flex gap-1">{(m.services ?? []).map(s => <Badge key={s} variant="secondary" className="text-xs">{s}</Badge>)}</div></TableCell>
                    <TableCell>{m.enabled ? <Badge variant="success">Mounted</Badge> : <Badge variant="secondary">Off</Badge>}</TableCell>
                    <TableCell><Button variant="ghost" size="icon" onClick={() => deleteMutation.mutate(m.id)}><Trash2 className="h-4 w-4 text-muted-foreground" /></Button></TableCell>
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

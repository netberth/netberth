import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { apiClient } from '@/lib/api'
import { PageLayout } from '@/components/layout/PageLayout'
import { Card, CardContent } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Plus, Trash2, Power } from 'lucide-react'
import type { WOLDevice } from '@/types'

export function WOL() {
  const queryClient = useQueryClient()
  const [form, setForm] = useState({ name: '', mac: '', broadcast: '255.255.255.255', port: 9 })

  const { data, isLoading } = useQuery({
    queryKey: ['wol'],
    queryFn: () => apiClient.get<{ data: WOLDevice[] }>('/wol'),
  })

  const createMutation = useMutation({
    mutationFn: (d: Record<string, unknown>) => apiClient.post('/wol', d),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['wol'] }); setForm({ name: '', mac: '', broadcast: '255.255.255.255', port: 9 }) },
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => apiClient.delete(`/wol/${id}`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['wol'] }),
  })

  const wakeMutation = useMutation({
    mutationFn: (id: string) => apiClient.post(`/wol/${id}/wake`),
    onSuccess: () => {},
  })

  return (
    <PageLayout title="Wake-on-LAN" description="Remotely wake devices on your network via magic packet" actions={
      <Button size="sm"><Plus className="mr-1 h-4 w-4" /> Add Device</Button>
    }>
      <Card className="border-border">
        <CardContent className="pt-6">
          <div className="grid grid-cols-4 gap-3">
            <div><Label>Name</Label><Input value={form.name} onChange={e => setForm({...form, name: e.target.value})} placeholder="Device name" /></div>
            <div><Label>MAC Address</Label><Input value={form.mac} onChange={e => setForm({...form, mac: e.target.value})} placeholder="AA:BB:CC:DD:EE:FF" /></div>
            <div><Label>Broadcast</Label><Input value={form.broadcast} onChange={e => setForm({...form, broadcast: e.target.value})} /></div>
            <div><Label>Port</Label><Input type="number" value={form.port} onChange={e => setForm({...form, port: +e.target.value})} /></div>
          </div>
          <div className="mt-3">
            <Button onClick={() => createMutation.mutate(form as unknown as Record<string, unknown>)} size="sm">Save Device</Button>
          </div>
        </CardContent>
      </Card>
      <Card className="border-border">
        <CardContent className="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead><TableHead>MAC</TableHead><TableHead>Broadcast</TableHead><TableHead>Port</TableHead><TableHead className="w-[120px]">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {isLoading ? (
                <TableRow><TableCell colSpan={5} className="text-center text-muted-foreground">Loading...</TableCell></TableRow>
              ) : (data?.data ?? []).length === 0 ? (
                <TableRow><TableCell colSpan={5} className="text-center text-muted-foreground">No devices</TableCell></TableRow>
              ) : (
                (data?.data ?? []).map((d) => (
                  <TableRow key={d.id}>
                    <TableCell className="font-medium">{d.name}</TableCell>
                    <TableCell className="font-mono text-xs">{d.mac}</TableCell>
                    <TableCell className="font-mono text-xs">{d.broadcast}</TableCell>
                    <TableCell>{d.port}</TableCell>
                    <TableCell>
                      <div className="flex gap-1">
                        <Button variant="outline" size="icon" onClick={() => wakeMutation.mutate(d.id)} title="Wake"><Power className="h-4 w-4 text-emerald-400" /></Button>
                        <Button variant="ghost" size="icon" onClick={() => deleteMutation.mutate(d.id)}><Trash2 className="h-4 w-4 text-muted-foreground" /></Button>
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

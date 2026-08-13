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
import type { STUNTunnel } from '@/types'
import { useI18n } from '@/i18n'

export function STUN() {
  const queryClient = useQueryClient()
  const { t } = useI18n()
  const [form, setForm] = useState({ name: '', protocol: 'tcp', local_port: 0, remote_port: 0, stun_server: 'stun.l.google.com:19302', target_addr: '', target_port: 0, enabled: false })

  const { data, isLoading } = useQuery({
    queryKey: ['stun'],
    queryFn: () => apiClient.get<{ data: STUNTunnel[] }>('/stun'),
  })

  const createMutation = useMutation({
    mutationFn: (t: Record<string, unknown>) => apiClient.post('/stun', t),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['stun'] }); setForm({ name: '', protocol: 'tcp', local_port: 0, remote_port: 0, stun_server: 'stun.l.google.com:19302', target_addr: '', target_port: 0, enabled: false }) },
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => apiClient.delete(`/stun/${id}`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['stun'] }),
  })

  return (
    <PageLayout title={t('STUN Tunnel')} description={t('NAT traversal tunnels for accessing services behind NAT')} actions={
      <Button size="sm"><Plus className="mr-1 h-4 w-4" /> {t('Add Tunnel')}</Button>
    }>
      <Card className="border-border">
        <CardContent className="pt-6">
          <div className="grid grid-cols-4 gap-3">
            <div><Label>{t('Name')}</Label><Input value={form.name} onChange={e => setForm({...form, name: e.target.value})} placeholder={t('Tunnel name')} /></div>
            <div>
              <Label>{t('Protocol')}</Label>
              <select className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                value={form.protocol} onChange={e => setForm({...form, protocol: e.target.value})}>
                <option value="tcp">TCP</option><option value="udp">UDP</option>
              </select>
            </div>
            <div><Label>{t('STUN Server')}</Label><Input value={form.stun_server} onChange={e => setForm({...form, stun_server: e.target.value})} /></div>
            <div className="grid grid-cols-2 gap-2">
              <div><Label>{t('Local Port')}</Label><Input type="number" value={form.local_port || ''} onChange={e => setForm({...form, local_port: +e.target.value})} /></div>
              <div><Label>{t('Remote Port')}</Label><Input type="number" value={form.remote_port || ''} onChange={e => setForm({...form, remote_port: +e.target.value})} /></div>
            </div>
          </div>
          <div className="mt-3 grid grid-cols-3 gap-3">
            <div><Label>{t('Target Addr')}</Label><Input value={form.target_addr} onChange={e => setForm({...form, target_addr: e.target.value})} placeholder="192.168.1.100" /></div>
            <div><Label>{t('Target Port')}</Label><Input type="number" value={form.target_port || ''} onChange={e => setForm({...form, target_port: +e.target.value})} /></div>
            <div className="flex items-end gap-6 pb-1">
              <div className="flex items-center gap-2"><Switch checked={form.enabled} onCheckedChange={v => setForm({...form, enabled: v})} /><Label>{t('Enabled')}</Label></div>
              <Button onClick={() => createMutation.mutate(form as unknown as Record<string, unknown>)} size="sm">{t('Save')}</Button>
            </div>
          </div>
        </CardContent>
      </Card>
      <Card className="border-border">
        <CardContent className="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('Name')}</TableHead><TableHead>{t('Protocol')}</TableHead><TableHead>{t('STUN Server')}</TableHead><TableHead>{t('Local:Remote')}</TableHead><TableHead>{t('Target')}</TableHead><TableHead>{t('Status')}</TableHead><TableHead className="w-[80px]"></TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {isLoading ? (
                <TableRow><TableCell colSpan={7} className="text-center text-muted-foreground">{t('Loading...')}</TableCell></TableRow>
              ) : (data?.data ?? []).length === 0 ? (
                <TableRow><TableCell colSpan={7} className="text-center text-muted-foreground">{t('No tunnels')}</TableCell></TableRow>
              ) : (
                (data?.data ?? []).map((tun) => (
                  <TableRow key={tun.id}>
                    <TableCell className="font-medium">{tun.name}</TableCell>
                    <TableCell><Badge variant="secondary">{tun.protocol.toUpperCase()}</Badge></TableCell>
                    <TableCell className="font-mono text-xs">{tun.stun_server}</TableCell>
                    <TableCell className="font-mono text-xs">{tun.local_port}:{tun.remote_port}</TableCell>
                    <TableCell className="font-mono text-xs">{tun.target_addr}:{tun.target_port}</TableCell>
                    <TableCell>{tun.enabled ? <Badge variant="success">{t('Active')}</Badge> : <Badge variant="secondary">{t('Off')}</Badge>}</TableCell>
                    <TableCell><Button variant="ghost" size="icon" onClick={() => deleteMutation.mutate(tun.id)}><Trash2 className="h-4 w-4 text-muted-foreground" /></Button></TableCell>
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

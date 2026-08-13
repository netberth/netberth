import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { apiClient } from '@/lib/api'
import { useAuth } from '@/stores/auth'
import { PageLayout } from '@/components/layout/PageLayout'
import { Card, CardContent } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { ChevronLeft, ChevronRight } from 'lucide-react'
import type { AuditEvent, PaginatedResponse } from '@/types'
import { useI18n } from '@/i18n'

const ACTIONS = ['created', 'updated', 'deleted']
const RESOURCES = [
  'forward_rule', 'proxy_rule', 'ddns_config', 'stun_tunnel',
  'wol_device', 'cron_job', 'acme_certificate', 'storage_mount', 'user',
]

export function Audit() {
  const { user } = useAuth()
  const { t } = useI18n()
  const [page, setPage] = useState(1)
  const [action, setAction] = useState('')
  const [resourceType, setResourceType] = useState('')
  const [username, setUsername] = useState('')
  const [appliedUsername, setAppliedUsername] = useState('')

  const { data, isLoading } = useQuery({
    queryKey: ['audit', page, action, resourceType, appliedUsername],
    queryFn: () => apiClient.get<PaginatedResponse<AuditEvent>>(
      `/audit?page=${page}&page_size=20&action=${encodeURIComponent(action)}&resource_type=${encodeURIComponent(resourceType)}&username=${encodeURIComponent(appliedUsername)}`
    ),
  })

  if (user?.role !== 'admin') {
    return (
      <PageLayout title={t('Audit Log')} description={t('Security audit trail')}>
        <p className="text-sm text-muted-foreground">{t('You do not have permission to view audit logs.')}</p>
      </PageLayout>
    )
  }

  const totalPages = data?.total_pages ?? 1

  return (
    <PageLayout title={t('Audit Log')} description={t('Security audit trail of all mutations')}>
      <Card className="border-border">
        <CardContent className="pt-6">
          <div className="grid grid-cols-4 gap-3">
            <div>
              <Label>{t('Action')}</Label>
              <select
                className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                value={action}
                onChange={e => { setAction(e.target.value); setPage(1) }}
              >
                <option value="">{t('All')}</option>
                {ACTIONS.map(a => <option key={a} value={a}>{a}</option>)}
              </select>
            </div>
            <div>
              <Label>{t('Resource')}</Label>
              <select
                className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                value={resourceType}
                onChange={e => { setResourceType(e.target.value); setPage(1) }}
              >
                <option value="">{t('All')}</option>
                {RESOURCES.map(r => <option key={r} value={r}>{r}</option>)}
              </select>
            </div>
            <div>
              <Label>{t('Username')}</Label>
              <Input
                value={username}
                onChange={e => setUsername(e.target.value)}
                onKeyDown={e => { if (e.key === 'Enter') { setAppliedUsername(username); setPage(1) } }}
                placeholder={t('filter by user')}
              />
            </div>
            <div className="flex items-end">
              <Button size="sm" onClick={() => { setAppliedUsername(username); setPage(1) }}>{t('Apply')}</Button>
            </div>
          </div>
        </CardContent>
      </Card>

      <Card className="border-border">
        <CardContent className="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('Time')}</TableHead>
                <TableHead>{t('User')}</TableHead>
                <TableHead>{t('Action')}</TableHead>
                <TableHead>{t('Resource')}</TableHead>
                <TableHead>{t('Resource ID')}</TableHead>
                <TableHead>{t('Remote')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {isLoading ? (
                <TableRow><TableCell colSpan={6} className="text-center text-muted-foreground">{t('Loading...')}</TableCell></TableRow>
              ) : data?.data?.length === 0 ? (
                <TableRow><TableCell colSpan={6} className="text-center text-muted-foreground">{t('No audit events')}</TableCell></TableRow>
              ) : (
                data?.data?.map(e => (
                  <TableRow key={e.id}>
                    <TableCell className="whitespace-nowrap font-mono text-xs">{e.created_at}</TableCell>
                    <TableCell>{e.username || '-'}</TableCell>
                    <TableCell><Badge variant={e.action === 'deleted' ? 'destructive' : e.action === 'updated' ? 'secondary' : 'success'}>{e.action}</Badge></TableCell>
                    <TableCell><Badge variant="secondary">{e.resource_type}</Badge></TableCell>
                    <TableCell className="max-w-[160px] truncate font-mono text-xs" title={e.resource_id}>{e.resource_id || '-'}</TableCell>
                    <TableCell className="font-mono text-xs">{e.remote_addr}</TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </CardContent>
      </Card>

      <div className="flex items-center justify-between pt-3 text-sm text-muted-foreground">
        <span>{t('Page {page} of {total} ({count} events)', { page: data?.page ?? 1, total: totalPages, count: data?.total ?? 0 })}</span>
        <div className="flex gap-2">
          <Button variant="outline" size="sm" disabled={page <= 1} onClick={() => setPage(p => Math.max(1, p - 1))}>
            <ChevronLeft className="mr-1 h-4 w-4" /> {t('Prev')}
          </Button>
          <Button variant="outline" size="sm" disabled={page >= totalPages} onClick={() => setPage(p => p + 1)}>
            {t('Next')} <ChevronRight className="ml-1 h-4 w-4" />
          </Button>
        </div>
      </div>
    </PageLayout>
  )
}

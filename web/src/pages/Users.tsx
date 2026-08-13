import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { apiClient } from '@/lib/api'
import { useAuth } from '@/stores/auth'
import { PageLayout } from '@/components/layout/PageLayout'
import { Card, CardContent } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Badge } from '@/components/ui/badge'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Plus, Trash2, KeyRound } from 'lucide-react'
import type { User } from '@/types'
import { useI18n } from '@/i18n'

type Role = 'admin' | 'operator' | 'viewer'
const ROLES: Role[] = ['admin', 'operator', 'viewer']

interface UserForm {
  username: string
  email: string
  role: Role
  password: string
}

export function Users() {
  const { user: currentUser } = useAuth()
  const { t } = useI18n()
  const queryClient = useQueryClient()
  const [form, setForm] = useState<UserForm>({ username: '', email: '', role: 'viewer', password: '' })
  const [msg, setMsg] = useState('')

  const { data, isLoading } = useQuery({
    queryKey: ['users'],
    queryFn: () => apiClient.get<{ data: User[] }>('/users'),
  })

  const invalidate = () => queryClient.invalidateQueries({ queryKey: ['users'] })

  const createMutation = useMutation({
    mutationFn: (body: UserForm) => apiClient.post('/users', body),
    onSuccess: () => {
      setForm({ username: '', email: '', role: 'viewer', password: '' })
      setMsg('')
      invalidate()
    },
    onError: () => setMsg(t('Create failed — username may already exist')),
  })

  const updateMutation = useMutation({
    mutationFn: ({ id, patch }: { id: string; patch: Partial<User> }) =>
      apiClient.put(`/users/${id}`, patch),
    onSuccess: invalidate,
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => apiClient.delete(`/users/${id}`),
    onSuccess: invalidate,
  })

  const resetMutation = useMutation({
    mutationFn: ({ id, password }: { id: string; password: string }) =>
      apiClient.post(`/users/${id}/reset-password`, { new_password: password }),
    onSuccess: invalidate,
  })

  if (currentUser?.role !== 'admin') {
    return (
      <PageLayout title={t('Users')} description={t('User management')}>
        <p className="text-sm text-muted-foreground">{t('You do not have permission to manage users.')}</p>
      </PageLayout>
    )
  }

  const handleReset = (u: User) => {
    const pw = window.prompt(t('New password for {name} (min 8 characters):', { name: u.username }))
    if (!pw) return
    if (pw.length < 8) {
      window.alert(t('Password must be at least 8 characters'))
      return
    }
    resetMutation.mutate({ id: u.id, password: pw })
  }

  const handleDelete = (u: User) => {
    if (!window.confirm(t('Delete user {name}?', { name: u.username }))) return
    deleteMutation.mutate(u.id)
  }

  return (
    <PageLayout
      title={t('Users')}
      description={t('Create and manage user accounts and roles')}
      actions={
        <Button size="sm" onClick={() => createMutation.mutate(form)}>
          <Plus className="mr-1 h-4 w-4" /> {t('Add User')}
        </Button>
      }
    >
      <Card className="border-border">
        <CardContent className="pt-6">
          <div className="grid grid-cols-4 gap-3">
            <div>
              <Label>{t('Username')}</Label>
              <Input value={form.username} onChange={e => setForm({ ...form, username: e.target.value })} placeholder="username" />
            </div>
            <div>
              <Label>{t('Email')}</Label>
              <Input value={form.email} onChange={e => setForm({ ...form, email: e.target.value })} placeholder="user@example.com" />
            </div>
            <div>
              <Label>{t('Role')}</Label>
              <select
                className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                value={form.role}
                onChange={e => setForm({ ...form, role: e.target.value as Role })}
              >
                {ROLES.map(r => <option key={r} value={r}>{r}</option>)}
              </select>
            </div>
            <div>
              <Label>{t('Initial Password')}</Label>
              <Input type="password" value={form.password} onChange={e => setForm({ ...form, password: e.target.value })} placeholder={t('min 8 chars')} />
            </div>
          </div>
          {msg && <p className="mt-2 text-sm text-destructive">{msg}</p>}
          <p className="mt-2 text-xs text-muted-foreground">{t('New users must change their password on first login.')}</p>
        </CardContent>
      </Card>

      <Card className="border-border">
        <CardContent className="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('Username')}</TableHead>
                <TableHead>{t('Email')}</TableHead>
                <TableHead>{t('Role')}</TableHead>
                <TableHead>{t('Enabled')}</TableHead>
                <TableHead className="w-[140px]">{t('Actions')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {isLoading ? (
                <TableRow><TableCell colSpan={5} className="text-center text-muted-foreground">{t('Loading...')}</TableCell></TableRow>
              ) : data?.data?.length === 0 ? (
                <TableRow><TableCell colSpan={5} className="text-center text-muted-foreground">{t('No users yet')}</TableCell></TableRow>
              ) : (
                data?.data?.map(u => (
                  <TableRow key={u.id}>
                    <TableCell className="font-medium">
                      {u.username}
                      {u.id === currentUser?.id && <Badge className="ml-2" variant="secondary">{t('you')}</Badge>}
                    </TableCell>
                    <TableCell>{u.email}</TableCell>
                    <TableCell>
                      <select
                        className="h-8 rounded-md border border-input bg-background px-2 text-sm"
                        value={u.role}
                        onChange={e => updateMutation.mutate({ id: u.id, patch: { role: e.target.value as Role } })}
                      >
                        {ROLES.map(r => <option key={r} value={r}>{r}</option>)}
                      </select>
                    </TableCell>
                    <TableCell>
                      <Switch
                        checked={u.enabled}
                        disabled={u.id === currentUser?.id}
                        onCheckedChange={v => updateMutation.mutate({ id: u.id, patch: { enabled: v } })}
                      />
                    </TableCell>
                    <TableCell>
                      <div className="flex gap-1">
                        <Button variant="ghost" size="icon" title={t('Reset password')} onClick={() => handleReset(u)}>
                          <KeyRound className="h-4 w-4 text-muted-foreground" />
                        </Button>
                        {u.id !== currentUser?.id && (
                          <Button variant="ghost" size="icon" title={t('Delete user')} onClick={() => handleDelete(u)}>
                            <Trash2 className="h-4 w-4 text-muted-foreground" />
                          </Button>
                        )}
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

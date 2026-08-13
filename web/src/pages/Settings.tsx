import { useAuth } from '@/stores/auth'
import { PageLayout } from '@/components/layout/PageLayout'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import { Switch } from '@/components/ui/switch'
import { Shield, Key, Bell, Globe } from 'lucide-react'
import { useState } from 'react'
import { apiClient } from '@/lib/api'
import { useI18n } from '@/i18n'

export function Settings() {
  const { user, fetchUser } = useAuth()
  const { t } = useI18n()
  const [passwordForm, setPasswordForm] = useState({ old_password: '', new_password: '', confirm: '' })
  const [passwordMsg, setPasswordMsg] = useState('')
  const [passwordOk, setPasswordOk] = useState(false)
  const [otpSecret, setOtpSecret] = useState('')
  const [otpQrUrl, setOtpQrUrl] = useState('')
  const [otpCode, setOtpCode] = useState('')
  const [otpMsg, setOtpMsg] = useState('')
  const [otpOk, setOtpOk] = useState(false)

  const changePassword = async () => {
    if (passwordForm.new_password !== passwordForm.confirm) {
      setPasswordMsg(t('Passwords do not match'))
      setPasswordOk(false)
      return
    }
    if (passwordForm.new_password.length < 8) {
      setPasswordMsg(t('Password must be at least 8 characters'))
      setPasswordOk(false)
      return
    }
    try {
      await apiClient.post('/auth/change-password', {
        old_password: passwordForm.old_password,
        new_password: passwordForm.new_password,
      })
      setPasswordMsg(t('Password changed successfully'))
      setPasswordOk(true)
      setPasswordForm({ old_password: '', new_password: '', confirm: '' })
    } catch {
      setPasswordMsg(t('Failed to change password'))
      setPasswordOk(false)
    }
  }

  return (
    <PageLayout title={t('Settings')} description={t('System configuration and user management')}>
      <div className="grid gap-6">
        <Card className="border-border">
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base"><Key className="h-4 w-4" /> {t('Change Password')}</CardTitle>
            <CardDescription>{t('Update your account password. Minimum 8 characters.')}</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="max-w-md space-y-3">
              <div><Label>{t('Current Password')}</Label><Input type="password" value={passwordForm.old_password} onChange={e => setPasswordForm({...passwordForm, old_password: e.target.value})} /></div>
              <div><Label>{t('New Password')}</Label><Input type="password" value={passwordForm.new_password} onChange={e => setPasswordForm({...passwordForm, new_password: e.target.value})} /></div>
              <div><Label>{t('Confirm Password')}</Label><Input type="password" value={passwordForm.confirm} onChange={e => setPasswordForm({...passwordForm, confirm: e.target.value})} /></div>
              {passwordMsg && <p className={`text-sm ${passwordOk ? 'text-emerald-400' : 'text-destructive'}`}>{passwordMsg}</p>}
              <Button onClick={changePassword}>{t('Update Password')}</Button>
            </div>
          </CardContent>
        </Card>

        <Card className="border-border">
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base"><Shield className="h-4 w-4" /> {t('Two-Factor Authentication')}</CardTitle>
            <CardDescription>{t('Add an extra layer of security to your account.')}</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="space-y-3">
              <div className="flex items-center gap-4">
                <p className="text-sm">{t('Status:')} {user?.otp_enabled ? <Badge variant="success">{t('Enabled')}</Badge> : <Badge variant="secondary">{t('Disabled')}</Badge>}</p>
                {user?.otp_enabled ? (
                  <Button variant="outline" size="sm" onClick={async () => {
                    await apiClient.post('/auth/2fa/disable')
                    fetchUser()
                  }}>{t('Disable 2FA')}</Button>
                ) : (
                  <Button variant="outline" size="sm" onClick={async () => {
                    try {
                      const res = await apiClient.get<{data: {secret: string, qr_code: string}}>('/auth/2fa/setup')
                      if (res.data) {
                        setOtpSecret(res.data.secret)
                        setOtpQrUrl(res.data.qr_code)
                        setOtpMsg('')
                        setOtpOk(false)
                      }
                    } catch { setOtpMsg(t('Setup failed')); setOtpOk(false) }
                  }}>{t('Enable 2FA')}</Button>
                )}
              </div>
              {otpSecret && !user?.otp_enabled && (
                <div className="space-y-2 rounded border border-border p-3">
                  <p className="text-xs text-muted-foreground">{t('Secret:')} <span className="font-mono text-foreground">{otpSecret}</span></p>
                  {otpQrUrl && <p className="text-xs text-muted-foreground">{t('URL:')} <span className="font-mono text-foreground break-all">{otpQrUrl}</span></p>}
                  <p className="text-xs text-muted-foreground">{t('Scan with Google Authenticator or Authy, then enter the 6-digit code:')}</p>
                  <div className="flex gap-2">
                    <Input className="w-32" placeholder="000000" maxLength={6} value={otpCode} onChange={e => setOtpCode(e.target.value)} />
                    <Button size="sm" onClick={async () => {
                      try {
                        await apiClient.post('/auth/2fa/enable', {code: otpCode})
                        setOtpMsg(t('2FA enabled!'))
                        setOtpOk(true)
                        setOtpSecret(''); setOtpQrUrl(''); setOtpCode('')
                        fetchUser()
                      } catch { setOtpMsg(t('Invalid code')); setOtpOk(false) }
                    }}>{t('Verify')}</Button>
                  </div>
                  {otpMsg && <p className={`text-xs ${otpOk ? 'text-emerald-400' : 'text-destructive'}`}>{otpMsg}</p>}
                </div>
              )}
            </div>
          </CardContent>
        </Card>

        <Card className="border-border">
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base"><Globe className="h-4 w-4" /> {t('About NetBerth')}</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="space-y-2 text-sm text-muted-foreground">
              <p>Version: <span className="text-foreground font-mono">{__APP_VERSION__}</span></p>
              <p>{t('NetBerth is a security-first network service management platform. Port forwarding, reverse proxy, DDNS, STUN, WOL, cron, ACME, and storage — all in one.')}</p>
            </div>
          </CardContent>
        </Card>
      </div>
    </PageLayout>
  )
}

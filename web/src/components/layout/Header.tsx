import { useAuth } from '@/stores/auth'
import { useNavigate } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { LogOut, User } from 'lucide-react'
import { useI18n, type Lang } from '@/i18n'

export function Header() {
  const { user, logout } = useAuth()
  const navigate = useNavigate()
  const { lang, setLang, t } = useI18n()

  const handleLogout = () => {
    logout()
    navigate('/login')
  }

  return (
    <header className="flex h-14 items-center justify-between border-b border-border bg-card px-6">
      <div className="flex items-center gap-2">
        <div className="h-2 w-2 rounded-full bg-emerald-400 animate-pulse" />
        <span className="text-sm text-muted-foreground">{t('System Online')}</span>
      </div>
      <div className="flex items-center gap-3">
        <Button
          variant="ghost"
          size="sm"
          onClick={() => setLang(lang === 'en' ? 'zh' : 'en')}
          className="text-xs"
        >
          {lang === 'en' ? '中文' : 'EN'}
        </Button>
        <div className="flex items-center gap-2 text-sm">
          <User className="h-4 w-4 text-muted-foreground" />
          <span className="text-foreground">{user?.username}</span>
          <span className="rounded bg-primary/10 px-1.5 py-0.5 text-xs text-primary">{user?.role}</span>
        </div>
        <Button variant="ghost" size="icon" onClick={handleLogout}>
          <LogOut className="h-4 w-4" />
        </Button>
      </div>
    </header>
  )
}

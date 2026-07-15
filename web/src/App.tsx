import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { useAuth } from '@/stores/auth'
import { useEffect, type ReactNode } from 'react'
import { Sidebar } from '@/components/layout/Sidebar'
import { Header } from '@/components/layout/Header'
import { Login } from '@/pages/Login'
import { Dashboard } from '@/pages/Dashboard'
import { Forward } from '@/pages/Forward'
import { Proxy } from '@/pages/Proxy'
import { DDNS } from '@/pages/DDNS'
import { STUN } from '@/pages/STUN'
import { WOL } from '@/pages/WOL'
import { Cron } from '@/pages/Cron'
import { ACME } from '@/pages/ACME'
import { Storage } from '@/pages/Storage'
import { Settings } from '@/pages/Settings'

const queryClient = new QueryClient({
  defaultOptions: { queries: { staleTime: 30_000, retry: 1 } },
})

function AppLayout({ children }: { children: ReactNode }) {
  return (
    <div className="min-h-screen">
      <Sidebar />
      <div className="ml-56">
        <Header />
        <main>{children}</main>
      </div>
    </div>
  )
}

function ProtectedRoute({ children }: { children: ReactNode }) {
  const { user, loading } = useAuth()
  if (loading) return <div className="flex h-screen items-center justify-center"><div className="h-8 w-8 animate-spin rounded-full border-2 border-primary border-t-transparent" /></div>
  if (!user) return <Navigate to="/login" replace />
  return <AppLayout>{children}</AppLayout>
}

export default function App() {
  const { fetchUser } = useAuth()

  useEffect(() => {
    if (localStorage.getItem('access_token')) {
      fetchUser()
    }
  }, [fetchUser])

  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <Routes>
          <Route path="/login" element={<Login />} />
          <Route path="/" element={<ProtectedRoute><Dashboard /></ProtectedRoute>} />
          <Route path="/forward" element={<ProtectedRoute><Forward /></ProtectedRoute>} />
          <Route path="/proxy" element={<ProtectedRoute><Proxy /></ProtectedRoute>} />
          <Route path="/ddns" element={<ProtectedRoute><DDNS /></ProtectedRoute>} />
          <Route path="/stun" element={<ProtectedRoute><STUN /></ProtectedRoute>} />
          <Route path="/wol" element={<ProtectedRoute><WOL /></ProtectedRoute>} />
          <Route path="/cron" element={<ProtectedRoute><Cron /></ProtectedRoute>} />
          <Route path="/acme" element={<ProtectedRoute><ACME /></ProtectedRoute>} />
          <Route path="/storage" element={<ProtectedRoute><Storage /></ProtectedRoute>} />
          <Route path="/settings" element={<ProtectedRoute><Settings /></ProtectedRoute>} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </BrowserRouter>
    </QueryClientProvider>
  )
}

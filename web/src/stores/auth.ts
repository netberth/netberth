import { create } from 'zustand'
import type { User } from '@/types'
import { apiClient, setTokens, clearTokens } from '@/lib/api'

interface AuthState {
  user: User | null
  loading: boolean
  login: (username: string, password: string) => Promise<boolean>
  logout: () => void
  fetchUser: () => Promise<void>
}

export const useAuth = create<AuthState>((set) => ({
  user: null,
  loading: !!localStorage.getItem('access_token'),
  login: async (username, password) => {
    const res = await apiClient.post<{ data: { tokens: { access_token: string; refresh_token: string }; user: User } }>('/auth/login', { username, password })
    if (res.data) {
      setTokens(res.data.tokens.access_token, res.data.tokens.refresh_token)
      set({ user: res.data.user })
      return true
    }
    return false
  },
  logout: () => {
    clearTokens()
    set({ user: null })
  },
  fetchUser: async () => {
    try {
      const res = await apiClient.get<{ data: User }>('/auth/me')
      if (res.data) { set({ user: res.data, loading: false }) }
    } catch {
      set({ user: null, loading: false })
    }
  },
}))

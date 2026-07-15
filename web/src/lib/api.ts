const API_BASE = '/api/v1'

let accessToken: string | null = localStorage.getItem('access_token')
let refreshToken: string | null = localStorage.getItem('refresh_token')

export function setTokens(access: string, refresh: string) {
  accessToken = access
  refreshToken = refresh
  localStorage.setItem('access_token', access)
  localStorage.setItem('refresh_token', refresh)
}

export function clearTokens() {
  accessToken = null
  refreshToken = null
  localStorage.removeItem('access_token')
  localStorage.removeItem('refresh_token')
}

async function refreshAccessToken(): Promise<boolean> {
  if (!refreshToken) return false
  try {
    const res = await fetch(`${API_BASE}/auth/refresh`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ refresh_token: refreshToken }),
    })
    if (!res.ok) return false
    const json = await res.json()
    if (json.data) {
      setTokens(json.data.access_token, json.data.refresh_token)
      return true
    }
    return false
  } catch {
    return false
  }
}

export async function api<T = unknown>(
  path: string,
  options: RequestInit = {}
): Promise<T> {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(options.headers as Record<string, string> || {}),
  }
  if (accessToken) {
    headers['Authorization'] = `Bearer ${accessToken}`
  }
  const res = await fetch(`${API_BASE}${path}`, { ...options, headers })

  // 403: force password change or permission denied
  if (res.status === 403) {
    try {
      const textData = await res.clone().text()
      let isPasswordChangeRequired = textData.includes('password change required') || textData.includes('/settings')

      try {
        const body = JSON.parse(textData)
        if (body.redirect === '/settings' || body.error?.includes('password change required') || body.message?.includes('password change required')) {
          isPasswordChangeRequired = true
        }
      } catch { /* text response, not JSON */ }

      if (isPasswordChangeRequired) {
        alert('First login detected. You must change your initial password in Settings before accessing this feature.')
        window.location.href = '/settings'
        throw new Error('Password change required')
      }
    } catch (err) {
      console.error('403 interceptor error:', err)
    }
  }

  // 401: token refresh
  if (res.status === 401 && refreshToken) {
    const refreshed = await refreshAccessToken()
    if (refreshed) {
      headers['Authorization'] = `Bearer ${accessToken}`
      const retry = await fetch(`${API_BASE}${path}`, { ...options, headers })
      return retry.json()
    }
    clearTokens()
    window.location.href = '/login'
    throw new Error('Session expired')
  }
  return res.json()
}

export const apiClient = {
  get: <T = unknown>(path: string) => api<T>(path),
  post: <T = unknown>(path: string, body?: unknown) =>
    api<T>(path, { method: 'POST', body: body ? JSON.stringify(body) : undefined }),
  put: <T = unknown>(path: string, body?: unknown) =>
    api<T>(path, { method: 'PUT', body: body ? JSON.stringify(body) : undefined }),
  delete: <T = unknown>(path: string) => api<T>(path, { method: 'DELETE' }),
}

const BASE_URL = import.meta.env.VITE_API_URL ?? ''

if (!BASE_URL) {
  console.error('[OnyxChat] VITE_API_URL is not set — API calls will fail.')
}

let token: string | null = sessionStorage.getItem('token')

export function setToken(t: string | null) {
  token = t
  if (t) sessionStorage.setItem('token', t)
  else sessionStorage.removeItem('token')
}

export function getToken() {
  return token
}

// Lazily imported to avoid a circular dep (auth.ts → client.ts → auth.ts).
// Only resolved at call time, after both modules are fully loaded.
async function tryRefresh(): Promise<string | null> {
  const { refresh } = await import('./auth')
  return refresh()
}

async function request<T>(method: string, path: string, body?: unknown, isRetry = false): Promise<T> {
  if (!BASE_URL) throw new Error('VITE_API_URL is not configured')

  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
  }

  if (token) headers['Authorization'] = `Bearer ${token}`

  const res = await fetch(`${BASE_URL}${path}`, {
    method,
    headers,
    body: body ? JSON.stringify(body) : undefined,
  })

  if (res.status === 401 && !isRetry && path !== '/api/v1/refresh' && path !== '/api/v1/login') {
    const newToken = await tryRefresh()
    if (newToken) {
      // Retry once with the new access token
      return request<T>(method, path, body, true)
    }
    // Refresh failed — clear session and surface the 401
    setToken(null)
    localStorage.removeItem('user')
  }

  if (!res.ok) {
    const text = await res.text()
    throw new Error(text || res.statusText)
  }

  const ct = res.headers.get('content-type') || ''
  if (!ct.includes('application/json')) return null as T
  return res.json() as Promise<T>
}

export const api = {
  get: <T>(path: string) => request<T>('GET', path),
  post: <T>(path: string, body: unknown) => request<T>('POST', path, body),
  patch: <T>(path: string, body: unknown) => request<T>('PATCH', path, body),
  delete: <T>(path: string, body?: unknown) => request<T>('DELETE', path, body),
}

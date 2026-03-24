import { api, getToken } from './client'
import type { GetKeyResponse } from '../types'

const BASE_URL = import.meta.env.VITE_API_URL

if (!BASE_URL) {
  throw new Error('VITE_API_URL is not set')
}

export async function uploadPublicKey(publicKey: string): Promise<void> {
  const res = await fetch(`${BASE_URL}/api/v1/keys`, {
    method: 'PUT',
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${getToken() ?? ''}`,
    },
    body: JSON.stringify({ publicKey }),
  })

  if (!res.ok) {
    throw new Error(await res.text())
  }
}

export async function fetchPublicKey(username: string): Promise<string | null> {
  try {
    const data = await api.get<GetKeyResponse>(`/api/v1/keys/${encodeURIComponent(username)}`)
    return data.publicKey
  } catch {
    return null
  }
}
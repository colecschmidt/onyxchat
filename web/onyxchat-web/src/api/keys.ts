// api/keys.ts — new file
// Talks to the two new backend endpoints: PUT /api/v1/keys, GET /api/v1/keys/:username

import { api, getToken } from './client'
import type { GetKeyResponse } from '../types'

/**
 * Upload your own ECDH public key to the server.
 * Call once after login/register.
 */
export async function uploadPublicKey(publicKey: string): Promise<void> {
  await fetch(
    (import.meta.env.VITE_API_URL ?? window.location.origin) + '/api/v1/keys',
    {
      method: 'PUT',
      headers: {
        'Content-Type': 'application/json',
        // client.ts stores the token in sessionStorage under 'token'
        Authorization: `Bearer ${getToken() ?? ''}`,
      },
      body: JSON.stringify({ publicKey }),
    },
  )
}

/**
 * Fetch any user's ECDH public key.
 * Returns null if the user hasn't uploaded a key yet (no E2E possible).
 */
export async function fetchPublicKey(username: string): Promise<string | null> {
  try {
    const data = await api.get<GetKeyResponse>(`/api/v1/keys/${encodeURIComponent(username)}`)
    return data.publicKey
  } catch {
    return null
  }
}
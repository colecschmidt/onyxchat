import { describe, it, expect } from 'vitest'
import { deriveSharedKey, encryptMessage, decryptMessage } from './crypto'

describe('E2E crypto', () => {
  it('encrypt → decrypt round trip', async () => {
    // Simulate two users generating keypairs
    const alice = await crypto.subtle.generateKey(
      { name: 'ECDH', namedCurve: 'P-256' }, true, ['deriveKey']
    )
    const bob = await crypto.subtle.generateKey(
      { name: 'ECDH', namedCurve: 'P-256' }, true, ['deriveKey']
    )

    // Export bob's public key as base64 (what the server would return)
    const bobPubRaw = await crypto.subtle.exportKey('spki', bob.publicKey)
    const bobPubB64 = btoa(String.fromCharCode(...new Uint8Array(bobPubRaw)))

    const alicePubRaw = await crypto.subtle.exportKey('spki', alice.publicKey)
    const alicePubB64 = btoa(String.fromCharCode(...new Uint8Array(alicePubRaw)))

    // Both sides derive shared key
    const aliceShared = await deriveSharedKey(alice.privateKey, bobPubB64)
    const bobShared   = await deriveSharedKey(bob.privateKey, alicePubB64)

    // Alice encrypts
    const plaintext = 'hello from alice'
    const { body, iv } = await encryptMessage(aliceShared, plaintext)

    // Ciphertext should not be plaintext
    expect(body).not.toBe(plaintext)

    // Bob decrypts
    const decrypted = await decryptMessage(bobShared, body, iv)
    expect(decrypted).toBe(plaintext)
  })

  it('wrong key fails to decrypt', async () => {
    const alice   = await crypto.subtle.generateKey({ name: 'ECDH', namedCurve: 'P-256' }, true, ['deriveKey'])
    const bob     = await crypto.subtle.generateKey({ name: 'ECDH', namedCurve: 'P-256' }, true, ['deriveKey'])
    const mallory = await crypto.subtle.generateKey({ name: 'ECDH', namedCurve: 'P-256' }, true, ['deriveKey'])

    const bobPubRaw     = await crypto.subtle.exportKey('spki', bob.publicKey)
    const bobPubB64     = btoa(String.fromCharCode(...new Uint8Array(bobPubRaw)))
    const malloryPubRaw = await crypto.subtle.exportKey('spki', mallory.publicKey)
    const malloryPubB64 = btoa(String.fromCharCode(...new Uint8Array(malloryPubRaw)))

    const aliceShared   = await deriveSharedKey(alice.privateKey, bobPubB64)
    const malloryShared = await deriveSharedKey(mallory.privateKey, malloryPubB64)

    const { body, iv } = await encryptMessage(aliceShared, 'secret')
    await expect(decryptMessage(malloryShared, body, iv)).rejects.toThrow()
  })
})
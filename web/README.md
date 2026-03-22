# OnyxChat — Web Client

A real-time messaging web app built with React, TypeScript, and the browser's native Web Crypto API. Pairs with the [OnyxChat Go backend](https://github.com/splitcell01/secure-messenger-server).

## Overview

OnyxChat implements end-to-end encrypted messaging in the browser without any third-party crypto libraries. Messages are encrypted with ECDH P-256 + AES-256-GCM before leaving the client — the server stores and relays ciphertext only and has no access to plaintext message content.

## Features

- **End-to-end encryption** — ECDH key agreement + AES-256-GCM per-message encryption via Web Crypto API
- **Real-time messaging** — WebSocket connection with presence tracking (online/offline)
- **Typing indicators** — live "is typing…" feedback
- **Optimistic UI** — messages appear instantly, confirmed or rolled back on server response
- **Encrypted history** — past messages fetched from the server are decrypted client-side on load
- **Graceful fallback** — falls back to plaintext if a peer hasn't uploaded an E2E key yet

## E2E Encryption Design

```
Alice                          Server                         Bob
  |                              |                              |
  |-- ECDH public key (SPKI) --> |                              |
  |                              | <-- ECDH public key (SPKI) --|
  |                              |                              |
  | <-- Bob's public key --------|                              |
  |                              |                              |
  | deriveSharedKey(             |       deriveSharedKey(       |
  |   alice.privateKey,          |         bob.privateKey,      |
  |   bob.publicKey              |         alice.publicKey      |
  | ) → AES-256-GCM key          |       ) → same AES-256-GCM key
  |                              |                              |
  |-- encrypt(plaintext) ------> | -- ciphertext + IV --------> |
  |   ciphertext + IV            |                              |-- decrypt(ciphertext)
  |                              |                              |   plaintext
```

**Key points:**

- Each user generates an ECDH P-256 keypair on first login using `crypto.subtle.generateKey`
- The private key is stored as a **non-extractable** `CryptoKey` in IndexedDB — it never exists as raw bytes in JS memory
- The public key is uploaded to the server as a base64 SPKI string
- Both sides independently derive the same AES-256-GCM shared key via ECDH — the key itself is never transmitted
- Each message gets a fresh random 12-byte IV; ciphertext and IV are sent to the server together
- Shared keys are cached in memory per peer to avoid re-deriving on every message

## Tech Stack

- **React 19** with TypeScript
- **Vite** — build tooling and dev server
- **Web Crypto API** — all crypto primitives (no external crypto dependencies)
- **IndexedDB** — private key persistence across sessions
- **WebSockets** — real-time messaging and presence
- **Vitest** — unit tests for the crypto layer

## Project Structure

```
src/
├── api/
│   ├── auth.ts          # Register, login
│   ├── client.ts        # Base fetch wrapper with auth headers
│   ├── keys.ts          # Upload/fetch ECDH public keys
│   └── messages.ts      # Send and fetch message history
├── components/
│   ├── AuthScreen.tsx   # Login / register UI
│   ├── ChatPanel.tsx    # Message thread, input, encryption badge
│   └── Sidebar.tsx      # Contact list with online presence
├── context/
│   ├── AuthContext.tsx  # Auth state, login/logout/register
│   └── ChatContext.tsx  # Messages, encryption, WebSocket coordination
├── hooks/
│   └── useWebSocket.ts  # WebSocket lifecycle and message dispatch
├── lib/
│   ├── crypto.ts        # ECDH + AES-GCM implementation
│   └── crypto.test.ts   # Round-trip and wrong-key rejection tests
└── types/
    └── index.ts         # Shared types (Message, Contact, WS events)
```

## Getting Started

### Prerequisites

- Node.js 18+
- The [OnyxChat backend](https://github.com/splitcell01/secure-messenger-server) running locally

### Install and run

```bash
npm install
npm run dev
```

The app will be available at `http://localhost:5173`.

By default it expects the backend at `http://localhost:8080`. To point it elsewhere:

```bash
VITE_API_URL=https://your-backend.example.com npm run dev
```

### Run tests

```bash
npx vitest run
```

The crypto tests cover the full encrypt → decrypt round trip and verify that decryption with a wrong key throws.

### Build for production

```bash
npm run build
```

## Related Projects

- **Go Backend:** [secure-messenger-server](https://github.com/splitcell01/secure-messenger-server)

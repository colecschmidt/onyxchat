# OnyxChat Backend (Go)

Backend service for **OnyxChat**, a real-time encrypted messaging platform built with Go, PostgreSQL, Redis, and WebSockets.

Handles **authentication**, **REST APIs**, **end-to-end encrypted messaging**, **realtime WebSocket connections**, and **distributed tracing** across the server and notification service.

---

## Part of the OnyxChat Platform

| Component | Role |
|-----------|------|
| `server/onyxchat-server` | This — Go API server |
| `server/notification-service` | Go notification service (service-to-service auth, Redis pub/sub) |
| `web/onyxchat-web` | React / PWA frontend |
| `desktop/onyxchat-desktop` | Java 21 / JavaFX desktop client |
| `iac/onyxchat-iac` | Terraform / AWS infrastructure |

---

## Architecture

```text
  Cloudflare DNS (onyxchat.dev)
       │
       ├── onyxchat.dev  ──► Cloudflare Pages (React PWA)
       │
       └── api.onyxchat.dev ──► AWS ALB
                                  │  /internal/* → 403 (listener rule, priority 1)
                                  │  /* → forward
                             ┌────┴────┐
                             ▼         ▼
                      ECS Fargate  ECS Fargate
                      (Go server)  (Go server)
                             │
             ┌───────────────┼──────────────────┐
             ▼               ▼                  ▼
      RDS PostgreSQL   ElastiCache Redis    SSM Parameter
      (messages,       (pub/sub fanout,     Store (secrets)
       users, keys)     presence, tickets)
                             │
                             ▼
                    notification-service
                    (ECS Fargate)
                    - subscribes message.created
                    - checks presence
                    - fetches push token via
                      /internal/* + X-Service-Secret
                    - delivers with retry + backoff
```

### WebSocket ticket flow

```text
Client                          Server                    Redis
  │                               │                         │
  ├─ POST /api/v1/ws/ticket ──────►                         │
  │   (Authorization: Bearer JWT) │                         │
  │                               ├─ SET ws:ticket:<id> ────►
  │                               │   TTL=30s               │
  ◄── { ticket: "<id>" } ─────────┤                         │
  │                               │                         │
  ├─ WS /api/v1/ws?ticket=<id> ───►                         │
  │                               ├─ GETDEL ws:ticket:<id> ─►
  ◄══ WebSocket established ══════┤                         │
```

Tickets are one-time use and expire in 30 seconds. The JWT never appears in WebSocket URLs or server logs.

---

## API Reference

### Auth

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `POST` | `/api/v1/register` | — | Register with invite code |
| `POST` | `/api/v1/login` | — | Login, receive JWT + refresh token |
| `POST` | `/api/v1/refresh` | — | Rotate access token |
| `POST` | `/api/v1/logout` | JWT | Invalidate session |

### Messaging

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `POST` | `/api/v1/messages` | JWT | Send a message |
| `GET` | `/api/v1/messages?peer=<username>&sinceId=<id>` | JWT | Fetch messages newer than sinceId |
| `GET` | `/api/v1/messages?peer=<username>&beforeId=<id>` | JWT | Fetch messages older than beforeId (scroll-up pagination) |
| `POST` | `/api/v1/messages/read` | JWT | Mark messages from a peer as read (fires `message_read` WS event) |
| `DELETE` | `/api/v1/messages/{id}` | JWT | Soft-delete a message (sender only; fires `message_deleted` WS event) |

### Users & Contacts

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/api/v1/users` | JWT | List users |
| `PATCH` | `/api/v1/users/me/password` | JWT | Change password |
| `PUT` | `/api/v1/users/me/push-token` | JWT | Register FCM/APNs push token |
| `DELETE` | `/api/v1/account` | JWT | GDPR account deletion |
| `GET` | `/api/v1/contacts` | JWT | List contacts |
| `POST` | `/api/v1/contacts` | JWT | Add contact |
| `DELETE` | `/api/v1/contacts/{username}` | JWT | Remove contact |

### E2E Encryption Keys

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `PUT` | `/api/v1/keys` | JWT | Upload ECDH public key |
| `GET` | `/api/v1/keys/{username}` | JWT | Fetch a user's public key |

### WebSocket

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `POST` | `/api/v1/ws/ticket` | JWT | Get one-time WS ticket |
| `GET` | `/api/v1/ws?ticket=<id>&sinceId=<id>` | ticket | Open WebSocket connection |

### Admin

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/api/v1/admin/invites` | JWT + admin | List invite codes |
| `POST` | `/api/v1/admin/invites` | JWT + admin | Create invite code |
| `POST` | `/api/v1/admin/invites/{code}/reset` | JWT + admin | Reset used invite |

### Internal (service-to-service only)

Blocked at the ALB in production (`/internal/*` → 403, listener rule priority 1). Protected by `X-Service-Secret` header in application code as a second layer.

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/internal/users/{id}/push-token` | `X-Service-Secret` | Fetch a user's push token (called by notification service) |

### Health

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health/live` | Liveness probe |
| `GET` | `/health/ready` | Readiness probe (checks DB) |
| `GET` | `/metrics` | Prometheus metrics |

---

## WebSocket Events

Events pushed to connected clients over the WebSocket:

| Type | Direction | Payload | Description |
|------|-----------|---------|-------------|
| `message` | server → client | `{type, message}` | New message received |
| `message_read` | server → client | `{type, byUserId, messageIds[]}` | Recipient read your messages |
| `message_deleted` | server → client | `{type, messageId}` | A message was deleted |
| `typing` | bidirectional | `{type, from/to, isTyping}` | Typing indicator |
| `presence` | server → client | `{type, userId, username, status}` | Contact came online/offline |
| `key_changed` | server → client | `{type, username}` | Peer uploaded a new public key |

---

## End-to-End Encryption

Messages are encrypted client-side using **ECDH P-256 + AES-256-GCM** via the browser's Web Crypto API.

- Each user generates a keypair on first login, stored in IndexedDB (private key is non-extractable)
- The public key is uploaded to the server after login
- Before sending, the client derives a shared AES key via ECDH and encrypts the message body
- The server stores and relays ciphertext — it never sees plaintext

---

## Service-to-Service Auth

The notification service authenticates calls to `/internal/*` using a shared secret header (`X-Service-Secret`). Both services read the secret from `INTERNAL_SERVICE_SECRET` in the environment. In production the secret is stored in SSM Parameter Store; the `/internal/*` path is blocked at the ALB as a first line of defense.

---

## Distributed Tracing

Both services export traces to Tempo via OTLP gRPC. Trace context is propagated across the service boundary through the Redis `message.created` payload using W3C `traceparent`, so the full chain — HTTP request → Redis publish → notification delivery — appears as one trace in Grafana.

---

## Key Design Decisions

**Soft delete** — deleted messages retain their row with `deleted_at` set and `body` cleared. Clients receive a `message_deleted` WS event in real time; offline clients reconstruct the deleted state on history reload without a separate API call.

**Read receipts** — `POST /messages/read` bulk-marks all unread messages from a peer in one `UPDATE ... RETURNING id` query. The returned IDs are pushed to the sender as a `message_read` WS event.

**Cursor-based pagination** — `sinceId` for polling new messages, `beforeId` for scrolling up through history. Both use the message primary key as cursor — stable under concurrent inserts, no skipped rows.

**Invite-only registration** — all registrations require a single-use invite code with optional expiry.

**Stateless backend** — no in-memory session state. JWT auth enables horizontal scaling across multiple ECS tasks.

**Idempotent sends** — clients generate a `clientMessageId` per message. The server uses `ON CONFLICT DO NOTHING` to deduplicate retries.

**Redis pub/sub for multi-instance fanout** — each ECS task subscribes to Redis channels. Messages and presence events published by any instance reach WebSocket clients on any instance.

---

## Local Development

### Prerequisites

- Go 1.25+
- Docker + Docker Compose

### Start the full stack

```bash
cp .env.dev.example .env.dev   # fill in values
ln -s .env.dev .env            # compose variable substitution
docker compose up -d
```

### Run tests

```bash
go test ./...
```

### Environment variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `SM_ENV` | no | `dev` | Set to `prod` to enable prod guards |
| `SM_SERVER_ADDR` | no | `:8080` | Listen address |
| `SM_DB_DSN` | yes | — | PostgreSQL DSN |
| `SM_REDIS_ADDR` | no | `redis:6379` | Redis address |
| `SM_REDIS_AUTH_TOKEN` | prod only | — | Redis auth token |
| `JWT_SECRET` | prod only | insecure dev default | JWT signing secret |
| `SM_ALLOWED_ORIGINS` | prod only | — | Comma-separated CORS origins |
| `SM_ADMIN_USERNAME` | prod only | `admin` | Username with admin privileges |
| `INTERNAL_SERVICE_SECRET` | prod only | insecure dev default | Shared secret for `/internal/*` endpoints |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | no | — | OTLP gRPC endpoint for distributed tracing |

---

## Deployment

Fully automated via GitHub Actions on push to `main`:

1. Tests run with race detector
2. Docker image built and pushed to ECR
3. ECS service updated — rolling deploy with automatic rollback on health check failure

Infrastructure managed in `iac/onyxchat-iac` via Terraform.

---

## Status

| Feature | Status |
|---------|--------|
| Invite-only registration | ✅ |
| JWT authentication + refresh tokens | ✅ |
| REST messaging API | ✅ |
| Read receipts | ✅ |
| Message deletion (soft delete) | ✅ |
| Cursor-based pagination | ✅ |
| WebSocket realtime | ✅ |
| Typing indicators | ✅ |
| Presence (online/offline) | ✅ |
| E2EE (ECDH P-256 + AES-256-GCM) | ✅ |
| Redis pub/sub multi-instance fanout | ✅ |
| Secure WS ticket auth | ✅ |
| Contacts | ✅ |
| GDPR account deletion | ✅ |
| Distributed tracing (OTel → Tempo) | ✅ |
| Prometheus metrics + Grafana | ✅ |
| Loki structured logging | ✅ |
| Notification service | ✅ |
| Service-to-service auth | ✅ |
| Push notifications (FCM/APNs) | 🟡 stub wired, backend ready |
| 2FA | 🟡 planned |

---

## License

MIT

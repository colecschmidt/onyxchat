# OnyxChat Notification Service

Standalone Go service that delivers offline push notifications for OnyxChat. Runs alongside the main API server as a separate process/container.

---

## What it does

1. Subscribes to the `message.created` Redis channel
2. Checks the distributed presence store — if the recipient is already online (active WebSocket), the notification is skipped
3. Fetches the recipient's push token from the main server via an authenticated internal API call
4. Delivers the notification with exponential backoff retry (3 attempts: 1s → 2s → 4s)

---

## Service-to-service auth

The notification service calls `GET /internal/users/{id}/push-token` on the main server. Every request carries an `X-Service-Secret` header. The main server validates the header before returning the token.

Both services read the secret from `INTERNAL_SERVICE_SECRET`. In production the value lives in SSM Parameter Store. The `/internal/*` path is also blocked at the ALB (priority 1 listener rule) so it is never reachable from the public internet — the header check is a second layer.

---

## Distributed tracing

The main server injects a W3C `traceparent` into every `message.created` Redis payload before publishing. This service extracts it and starts a child span, so the full chain — `POST /messages` → Redis publish → notification delivery — appears as a single trace in Grafana/Tempo across both services.

---

## Metrics

Exposed on `:9091/metrics`, scraped by the existing Prometheus instance.

| Metric | Type | Description |
|--------|------|-------------|
| `notifications_sent_total` | Counter | Successfully delivered notifications |
| `notifications_skipped_online_total` | Counter | Skipped — recipient was online |
| `notification_delivery_errors_total` | Counter | Failed after all retries |
| `notification_delivery_duration_seconds` | Histogram | End-to-end delivery time including retries |

---

## Swapping in a real push backend

The stub in `internal/notifier/delivery.go` → `deliver()` is the only function to change. The retry contract, metrics, and tracing stay the same regardless of backend (FCM, APNs, web push).

```go
func deliver(ctx context.Context, ev MessageCreatedEvent, pushToken string) error {
    // Replace with: fcm.Send(ctx, pushToken, ev.MessageID)
    return nil
}
```

---

## Environment variables

| Variable | Default | Description |
|----------|---------|-------------|
| `SM_REDIS_ADDR` | `redis:6379` | Redis address |
| `ONYXCHAT_SERVER_URL` | `http://server:8080` | Main server base URL for internal API calls |
| `INTERNAL_SERVICE_SECRET` | insecure dev default | Shared secret for `X-Service-Secret` header |
| `NOTIF_METRICS_ADDR` | `:9091` | Prometheus metrics listen address |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | — | OTLP gRPC endpoint for distributed tracing |

---

## Running locally

```bash
cd server/onyxchat-server
docker compose up --build notification-service -d
docker compose logs notification-service -f
```

---

## License

MIT

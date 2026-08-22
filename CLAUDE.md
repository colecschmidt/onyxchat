# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository overview

OnyxChat is a monorepo for a production, end-to-end encrypted messaging platform (live at onyxchat.dev). It has five independently-deployed components:

```
server/onyxchat-server/       Go API server — auth, REST, WebSocket, E2EE key exchange
server/notification-service/  Go service — offline push delivery via Redis pub/sub
web/onyxchat-web/              React 19 + TypeScript PWA (Vite)
desktop/onyxchat-desktop/      Java 21 / JavaFX desktop client (Maven) — early stage, not production-grade
iac/onyxchat-iac/               Terraform for AWS (ECS Fargate, RDS, ElastiCache, ALB, SSM, ECR)
```

Each has its own CI workflow in `.github/workflows/` and only builds/deploys when its path changes (`server/**`, `web/**`, `desktop/**`).

## Commands

### server/onyxchat-server (Go 1.25)

Run all commands from `server/onyxchat-server/`.

```bash
docker compose up -d --build     # full local stack: Postgres + Redis + server (make dev / make up)
make health                      # curl /health/ready
make logs                        # tail compose logs

go test ./...                    # all tests
go test ./... -run TestName      # single test
go test ./internal/http/...      # one package
go test ./internal/store/...     # store package tests need a live Postgres — auto-skip if SM_DB_DSN
                                  # is unset and localhost:5432 isn't reachable

gofmt -l .                       # CI fails on any file gofmt would reformat (no -race in CI)
golangci-lint run ./...          # make lint
go mod tidy                      # make tidy
```

Local env: `cp .env.dev.example .env.dev && ln -s .env.dev .env` (compose reads `.env` for variable substitution). `SM_DB_DSN` is the only required var outside of `SM_ENV=prod`; everything else has an insecure dev default that `main.go` logs a warning for.

### server/notification-service (Go 1.25)

```bash
cd server/onyxchat-server && docker compose up --build notification-service -d
go test ./...
```

Runs alongside the main server as a separate process, authenticating to it via `INTERNAL_SERVICE_SECRET`. Metrics on `:9091/metrics`. The only thing to change to wire up a real push backend is `deliver()` in `internal/notifier/delivery.go`.

### web/onyxchat-web (React 19 + TS + Vite)

```bash
npm install
npm run dev
npm run build      # tsc -b && vite build — type errors fail the build
npm run lint
npm test           # vitest run
npm run test:watch
```

### desktop/onyxchat-desktop (Java 21, Maven)

```bash
mvn -B -ntp clean package
```

### iac/onyxchat-iac (Terraform)

Standard `terraform plan`/`apply` from that directory. Deploys are otherwise driven entirely by the GitHub Actions workflows on push to `main` — there's no manual deploy step for server/web.

## Architecture

### Request flow (onyxchat-server)

`cmd/server/main.go` wires dependencies and starts the process: opens the DB (`store.MustOpen`), runs migrations, connects Redis, starts two Redis-subscriber goroutines (`StartMessageSubscriber`, `StartPresenceSubscriber`, each with a reconnect-with-backoff loop), then builds the router via `internal/http.NewRouter` and starts `http.Server`.

`NewRouter` (`internal/http/router.go`) is the map of the whole HTTP surface — read it first when tracing a route. Middleware order matters: `RequestID` → `CORSMiddleware` → `AccessLogAndMetrics` wrap every route; protected routes additionally get `AuthMiddleware` (JWT) and `PerUserRateLimit`; admin routes add `AdminOnly`; `/internal/*` gets `InternalServiceAuth` (checked twice — once at the ALB via a listener rule that 403s `/internal/*` from the public internet, once in-app via the `X-Service-Secret` header). The whole router is wrapped in `otelhttp.NewHandler` for tracing.

Handlers depend on narrow interfaces (`userStorer`, `messageStorer` in `stores_interfaces.go`), not the concrete `*store.UserStore`/`*store.MessageStore` — this is what lets `internal/http` tests fake the store without a real DB, while `internal/store` tests run against real Postgres (see Commands above).

### WebSocket + realtime fanout

Auth never touches the WS URL. Flow: `POST /api/v1/ws/ticket` (JWT-authed) stores a one-time ticket in Redis with a 30s TTL → client opens `GET /api/v1/ws?ticket=...` → server does `GETDEL` on the ticket → connection upgrades. See `ws_ticket.go` / `ws_auth_middleware.go` / `ws.go`.

The backend is stateless and horizontally scaled (2 ECS tasks minimum): a `Hub` holds only local WebSocket connections per task. Cross-instance delivery goes through Redis pub/sub — `SendMessageHandler` etc. publish to `message.created`/`message.deleted` channels via `EventPublisher` (`RedisPublisher`), and `StartMessageSubscriber`/`StartPresenceSubscriber` (started in `main.go`) receive on every task and push to that task's local `Hub`. Presence is similarly tracked in Redis (`PresenceStore`), not in-process, so "is this user online" is consistent across tasks.

Trace context is propagated across this boundary manually: the publishing side injects a W3C `traceparent` into the Redis payload; `notification-service` extracts it to keep `POST /messages → Redis publish → notification delivery` as one trace in Tempo, even though it crosses a process/service boundary that OTel's HTTP instrumentation doesn't see.

### End-to-end encryption

Crypto is entirely client-side (`web/onyxchat-web/src/lib/crypto.ts`) using the Web Crypto API — ECDH P-256 to derive a shared AES-256-GCM key per conversation, private keys stored as non-extractable `CryptoKey` objects in IndexedDB. The server (`key_handlers.go`) only stores and serves public keys (`PUT/GET /api/v1/keys`); message bodies it stores/relays are ciphertext it cannot decrypt. Don't add server-side plaintext handling of message bodies — that would break the security model, not just add a feature.

### Migrations

`internal/store/schema.go` embeds `migrations/*.sql` via `go:embed` and runs them through `golang-migrate` on every startup (`RunMigrations`, called from `main.go`) — idempotent, no separate migration step in CI/CD. New migrations go in `internal/store/migrations/` following the existing numbered naming.

### Observability

Structured logging is zap; `internal/http/sentry.go` tees a `sentryCore` onto the logger that forwards every `Error`+ level entry to Sentry via `CaptureException`/`CaptureMessage` — there's no route or message filtering beyond that, so a new `log.Error(...)` call anywhere in a hot path becomes a Sentry event per request. If a given error-level log line is an expected condition rather than a real error (e.g. a health-check dependency being down), attach `SkipSentry()` (defined in `sentry.go`) to its fields to keep it in normal logs/metrics without forwarding it to Sentry. `AccessLogAndMetrics` (`observability_middleware.go`) is the one place that logs every request; it already does this for 5xx responses on `/health*` routes.

`/health/ready` (`health_handlers.go`) only checks that a DB connection is obtainable within `readyCheckTimeout` (2s) — it does not check Redis or migration status. It's polled independently by both the ALB target group health check and the Dockerfile's `HEALTHCHECK` directive, on different intervals/timeouts, and shares the same connection pool (`store.MustOpen`, sized in `internal/store/db.go`) as all other DB-backed handlers — pool size there is deliberately kept small relative to `max_capacity` in `iac/onyxchat-iac/ecr.tf` and RDS's connection ceiling in `iac/onyxchat-iac/rds.tf`, since ECS can run up to 2x the desired task count during a rolling deploy. Keep these in sync if either changes.

Prometheus metrics are defined in `observability_middleware.go` (HTTP) and scattered near what they measure (`ObserveDBQuery`, `ObserveRedisOp` helpers) — `/metrics` on the main server, `:9091/metrics` on notification-service.

### Infrastructure

Terraform in `iac/onyxchat-iac/` is the source of truth for ECS task sizing, autoscaling (CPU-only target tracking, `ecr.tf`), the ALB + target group health check config (`main.tf`), and RDS (`rds.tf`, single `db.t4g.micro`, no read replica). Autoscaling does not currently react to DB connection pressure, only CPU — worth checking `rds.tf`/`ecr.tf` together before changing pool sizes or task counts, since they're coupled through Postgres's `max_connections`.

# BridgeWatch

BridgeWatch coordinates the closeout, load testing, handover acceptance and opening-readiness work for a major bridge project. It is a Go HTTP service backed by SQLite and designed around auditable transactions, optimistic concurrency and recoverable background jobs.

## Capabilities

- Revocable server sessions with expiry and four business roles.
- Work-package closeout, supervision inspections and finding resolution.
- Load-test plans, cases, sensor channels, readings and asynchronous evaluation.
- Handover dossiers whose review depends on accepted work, closed findings, passed inspections and passed load runs.
- Derived opening readiness, request idempotency, durable audit events and leased jobs.

The schema is migrated automatically and uses WAL mode, foreign keys, unique constraints and version columns. Closing and reopening the process preserves all business state and recovers expired work leases.

## Run

```sh
cp .env.example .env
GOTOOLCHAIN=local CGO_ENABLED=0 go run ./cmd/server
```

`BRIDGEWATCH_BOOTSTRAP_PASSWORD` is required at startup and must contain at least 12 characters. Bootstrap accounts use the local domain and represent owner, contractor, supervision and commissioning roles.

Probe `GET /healthz` for liveness and `GET /readyz` for database-backed readiness. Login with `POST /v1/auth/login`, then send `Authorization: Bearer <token>` to protected endpoints.

## HTTP workflows

- Projects and closeout: create/list projects, create work packages, transition work, schedule and complete inspections, resolve findings.
- Load commissioning: create and approve plans, start runs, append sensor readings and enqueue durable evaluation.
- Handover: create a dossier, receive each required document, submit for review, record a supervision decision and query opening readiness.

All protected reads and writes are scoped to the authenticated organization. Mutations use role checks, optimistic versions where lifecycle conflicts matter, and stable JSON errors carrying a request ID.

## Verify

```sh
go test ./... -count=1
go test -race ./... -count=1
go vet ./...
CGO_ENABLED=0 go build ./...
```

The root Dockerfile is architecture-neutral. Select the target with Docker's `--platform` flag; the same entrypoint and SQLite persistence are used on amd64 and arm64.

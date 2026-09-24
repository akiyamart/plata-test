# quoteservice

Async FX quote service for the Plata Go Engineer assignment.

User requests a quote update, gets an `update_id` immediately; a background worker fetches the rate from [exchangerate.dev](https://exchangerate.dev) and stores it in PostgreSQL.

## Architecture

```text
cmd/quoteservice/                 composition root
internal/
  api/
    router/                       route wiring
    handler/{quote,update}/       HTTP handlers, DTOs, use-case contracts
    middleware/                   request ID, logging, recovery, CORS
    response/                     JSON and domain error mapping
    apictx/                       request context values
  domain/                         pair, quote, update
  service/                        use cases, worker, repository/provider ports
  repository/postgres/            pool, migrations, persistence adapters
  exchangerate/                   exchangerate.dev adapter
  config/                         environment config
  metrics/                        process counters
  clock/                          system clock adapter
```

Supported pairs: whitelist currencies `USD`, `EUR`, `MXN` (any distinct pair among them).

## Run with Docker

```bash
cp .env.example .env
docker compose up --build
```

API: http://localhost:8080  
Swagger UI: http://localhost:8081

## Run locally

```bash
# start Postgres (example)
docker compose up -d postgres

export DATABASE_URL=postgres://quotes:quotes@localhost:5432/quotes?sslmode=disable
export HTTP_ADDR=:8080
go run ./cmd/quoteservice
```

## API examples

Request an update:

```bash
curl -s -X POST http://localhost:8080/v1/quotes/updates \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: client-retry-1' \
  -H 'X-Request-Id: my-client-corr-1' \
  -d '{"pair":"EUR/MXN"}'
# {"update_id":"..."}  (+ response header X-Request-Id)
```

`X-Request-Id` is accepted or generated, echoed on the response, stored on the job as `correlation_id`, and appears in HTTP / worker / FX logs (not as `/metrics` labels). Repeating the same `Idempotency-Key` (or JSON `idempotency_key`) returns the same `update_id` and does not enqueue a second job (first correlation_id wins). Reusing a key with a different pair returns `409`.

OpenAPI spec: [`api/openapi.yaml`](api/openapi.yaml).

Poll by update id:

```bash
curl -s http://localhost:8080/v1/quotes/updates/<update_id>
# {"update_id":"...","pair":"EUR/MXN","status":"completed","rate":"...","observed_at":"..."}
```

Latest quote for a pair (`EUR-MXN` in the path):

```bash
curl -s http://localhost:8080/v1/quotes/EUR-MXN
```

Health:

```bash
curl -s http://localhost:8080/healthz
curl -s http://localhost:8080/readyz
```

Failed updates return a stable `error` code (`fx_unavailable`, `fx_timeout`, `max_attempts`, `invalid_quote`), not the upstream body.

Optional: set `EXCHANGERATE_API_KEY` for authenticated quota on exchangerate.dev (anonymous access works for light use).

## Multi-instance

Several app processes may share one Postgres. Migrations take a Postgres advisory lock on startup. Job claiming uses `FOR UPDATE SKIP LOCKED`; completes/fails require `status = 'processing'` (lost lease → skip, no overwrite).

Ops notes when scaling replicas to N:

| Setting | Note |
| --- | --- |
| `FX_RPS` / `FX_BURST` | Per process. Cap total upstream QPS ≈ `FX_RPS × N`; set each replica to `global_budget / N`. |
| `DB_MAX_CONNS` | Per process. Keep `N × DB_MAX_CONNS` under Postgres `max_connections`. |
| `PROCESSING_LEASE_MS` | Must stay greater than `JOB_TIMEOUT_MS` (plus margin) so a live job is not reclaimed mid-flight. |
| `MAX_PENDING` | Soft backpressure (check-then-insert); under load the queue can briefly exceed the limit. |
| `GET /metrics` | Process-local counters; scrape every pod and sum if you need cluster totals. |

## Assignment extras

| Extra from TZ | Status |
| --- | --- |
| Unit tests | Yes (`go test ./...`) |
| Docker | Yes (`Dockerfile` + `docker-compose.yml`) |
| Idempotent quote update | Yes (`Idempotency-Key` header or JSON `idempotency_key`) |
| OpenAPI / Swagger | Yes ([`api/openapi.yaml`](api/openapi.yaml)) |

Postgres integration tests (claim/reclaim, upsert guard) run when `DATABASE_URL` is set:

```bash
go test ./...
DATABASE_URL=postgres://quotes:quotes@localhost:5432/quotes?sslmode=disable go test ./internal/app/infrastructure/repository/...
```

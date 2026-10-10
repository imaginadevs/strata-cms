<p align="center">
  <img src="assets/branding/logo-stack-lockup.svg" alt="Strata" width="300" />
</p>


<p align="center">
  A high-performance, <strong>open-source</strong> (MIT) headless CMS written in Go with a React admin interface.<br />
  Built for multi-tenant, white-label deployments with an AI-first, event-driven architecture.
</p>


<p align="center">
  Made with Strata · <a href="https://www.imaginacolombia.com/">Imagina Colombia</a>
</p>

# Strata

## License

This project is open source under the [MIT License](LICENSE).

## Infrastructure & deployment

Self-hosting is fully supported. If you need managed infrastructure, production deployment, or hosting support, visit [Imagina Colombia](https://www.imaginacolombia.com/) or contact [ventas@imaginacolombia.com](mailto:ventas@imaginacolombia.com).

## Stack

| Layer | Technology |
|-------|-----------|
| Backend | Go 1.25, Chi router, pgx v5 |
| API | REST + GraphQL (gqlgen) |
| Realtime | SSE (Server-Sent Events) |
| Events | In-memory bus (dev) · Kafka via Watermill (prod) |
| Database | PostgreSQL 16 only |
| Migrations | Goose (embedded SQL) |
| Auth | JWT + Logto SSO (OIDC) |
| File storage | S3-compatible (per-tenant) |
| Frontend | Vite + React 18, TanStack Router/Query, Zustand |
| Styling | Tailwind CSS v3, Radix UI, Framer Motion |

## Quick start

```bash
# 1 — clone and install
git clone <repo-url> strata && cd strata
make setup          # copies .env.example → .env and installs deps

# 2 — edit credentials
nano .env

# 3 — start postgres
docker-compose up -d postgres

# 4 — run both services (hot-reload)
make dev            # backend :8080  ·  frontend :5173
```

Open [http://localhost:5173](http://localhost:5173).

## Commands

| Command | Description |
|---------|-------------|
| `make dev` | Start backend + frontend (parallel) |
| `make build` | Production build (frontend embedded in Go binary) |
| `make docker-build` | Build Docker image |
| `make docker-up` | Start full stack via docker-compose |
| `make migrate-up` | Apply pending migrations |
| `make migrate-create name=add_posts` | Create a new migration file |
| `make test` | Run all tests |
| `make lint` | Lint backend + frontend |

## Architecture

See [docs/Architecture.md](docs/Architecture.md) for the full architectural reference — keep this file updated and pass it to Claude when working on the codebase.

## Project layout

```
strata/
├── backend/                  Go service
│   ├── cmd/server/           Entry point
│   ├── internal/
│   │   ├── core/             Domain + ports (no external deps)
│   │   │   ├── domain/       Entities (base, tenant, user, collection…)
│   │   │   ├── ports/        Interfaces (inbound + outbound)
│   │   │   └── events/       Event bus interface + in-memory impl
│   │   ├── adapters/         Implementations
│   │   │   ├── http/         REST handlers, GraphQL, SSE, middleware
│   │   │   ├── events/       Kafka bus adapter (Watermill)
│   │   │   └── storage/      S3-compatible file adapter
│   │   └── infrastructure/   Config, DB pool, migrations, logger, server
├── frontend/                 React SPA
│   └── src/
│       ├── components/       Layout + UI primitives (crystal design)
│       ├── pages/            Route-level components
│       ├── routes/           TanStack Router tree
│       ├── store/            Zustand stores (auth, tenant)
│       ├── lib/              API client, GraphQL client, SSE client
│       └── types/            TypeScript interfaces
├── docs/
│   ├── Architecture.md       Architectural reference (keep updated)
│   └── implementation/       Step-by-step build guides (01–12)
├── Dockerfile                Multi-stage: Node → Go → distroless
├── docker-compose.yml        Local dev + production compose
└── Makefile                  Developer commands
```

## Collection types

| Type | Description |
|------|-------------|
| `base` | Standard user-defined collection |
| `auth` | Includes built-in `username`, `password_hash`, `status`, `last_login` system fields and integrates with the auth engine |

## Super admin

Super admins can navigate across all tenants. Access is restricted to a static IP whitelist configured **only** in `.env` (`SUPER_ADMIN_ALLOWED_IPS`). This list is never stored in the database and super admin accounts are hidden from regular user queries.

## Events

Every model operation emits a typed event on the bus. Services and external integrations subscribe to these events to extend behaviour without modifying core logic.

```
items.before.create  →  guard hooks (return error to abort)
items.after.create   →  side effects (notifications, search indexing…)
schema.collection.*  →  schema change hooks
auth.login / logout  →  audit, session management
```

Switch from in-memory to Kafka by setting `ENABLE_KAFKA=true` and `KAFKA_BROKERS=...` in `.env`.

## Realtime (SSE)

Authenticated clients connect to `/api/events?token=<jwt>`. The server pushes typed JSON events as operations occur. The frontend `sseClient` (src/lib/sse.ts) auto-reconnects with exponential backoff.

## i18n

Tenants have a `default_language`. Fields marked `translatable: true` can have per-language values. The admin UI shows a language selector to create/edit translations — each translation is the same record presented in a different language context, not a duplicated record.

## File storage

Each tenant configures its own S3-compatible bucket (AWS S3, Cloudflare R2, MinIO) through Settings → File Storage. Credentials are encrypted before being stored in `tenants.settings`. The storage adapter is resolved per-tenant at request time.

## Implementation guide

Step-by-step build guides are in [docs/implementation/](docs/implementation/).

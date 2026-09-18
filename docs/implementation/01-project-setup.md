# 01 — Project Setup

## Overview

This guide walks through everything needed to get the CMS running on a development machine from a fresh clone. It covers prerequisites, environment configuration, the `make setup` / `make dev` workflow, and a directory-by-directory tour of the monorepo. All subsequent guides assume you have completed these steps.

## Prerequisites

| Tool | Version | Install |
|------|---------|---------|
| Go | 1.22+ | https://go.dev/dl/ |
| Node.js | 20 LTS | https://nodejs.org/ |
| Docker + Compose | latest | https://docs.docker.com/get-docker/ |
| PostgreSQL | 15+ | via Docker (recommended) or native |
| make | any | already on macOS/Linux |
| golangci-lint | v1.59+ | `go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest` |

Verify before continuing:

```bash
go version          # go version go1.22.x linux/amd64
node --version      # v20.x.x
docker --version    # Docker version 26.x.x
psql --version      # psql (PostgreSQL) 15.x
```

## Step-by-Step

### 1. Clone the repository

```bash
git clone https://github.com/imaginacolombia/cms.git
cd cms
```

### 2. Start infrastructure with Docker Compose

The provided `docker-compose.yml` starts PostgreSQL (and optionally Kafka). For local development you only need PostgreSQL:

```bash
# Start PostgreSQL only
docker compose up -d postgres

# Verify it is running
docker compose ps
# NAME         STATUS
# cms-postgres running
```

If you want Kafka for event streaming (covered in guide 09):

```bash
docker compose up -d postgres zookeeper kafka
```

Wait ~10 seconds for Kafka to be ready before starting the backend with `ENABLE_KAFKA=true`.

### 3. Run first-time setup

```bash
make setup
```

This command:
- Copies `.env.example` → `.env` (skips if `.env` already exists).
- Runs `npm install` inside `frontend/`.
- Runs `go mod download` inside `backend/`.

Expected output:

```
✓ Setup complete. Edit .env then run: make dev
```

### 4. Edit .env

Open `.env` in your editor. At minimum set these values:

```dotenv
# Database — matches docker-compose defaults
DB_HOST=localhost
DB_PORT=5432
DB_NAME=cms
DB_USER=cms
DB_PASSWORD=cms_secret

# Auth — change this before going anywhere near production
JWT_SECRET=change-me-to-something-32-chars-minimum

# Super admin IP whitelist (loopback only for local dev)
SUPER_ADMIN_ALLOWED_IPS=127.0.0.1,::1

# Features (defaults are fine for local dev)
ENABLE_KAFKA=false
ENABLE_SSO=false
ENABLE_GRAPHQL=true
ENABLE_REST=true
```

The full list of supported variables is documented in `.env.example`. The config loader (`backend/internal/infrastructure/config/config.go`) uses Viper, which reads `.env` first and then falls back to actual environment variables, so CI/production can inject values without an `.env` file.

### 5. Create the PostgreSQL database (first time only)

If using the Docker Compose PostgreSQL, the `cms` database and user are created automatically from the `POSTGRES_*` environment variables set in `docker-compose.yml`. Skip to step 6.

If using a native PostgreSQL installation:

```bash
psql -U postgres <<'SQL'
CREATE ROLE cms WITH LOGIN PASSWORD 'cms_secret';
CREATE DATABASE cms OWNER cms;
GRANT ALL PRIVILEGES ON DATABASE cms TO cms;
SQL
```

### 6. Run database migrations

Migrations run automatically when the backend starts (via `database.Migrate()` in `main.go`). You can also run them manually:

```bash
make migrate-up
```

This executes Goose against the embedded SQL files in `backend/internal/infrastructure/database/migrations/`. The migration state is tracked in the `cms_migrations` table (not the default `goose_db_version`).

To roll back the last migration:

```bash
make migrate-down
```

### 7. Start the development server

```bash
make dev
```

This runs `make dev-backend` and `make dev-frontend` in parallel (`make -j2`).

- Backend: `http://localhost:8080` (Go, hot-reload not included — use `air` if you want it)
- Frontend: `http://localhost:5173` (Vite HMR)

The React app proxies `/api` to port 8080 via Vite's `server.proxy` config in `frontend/vite.config.ts`.

You should see:

```
▶ Starting backend...
▶ Starting frontend...
{"level":"info","msg":"server listening","addr":"0.0.0.0:8080"}
  VITE v5.x.x  ready in 432 ms
  ➜  Local:   http://localhost:5173/
```

### 8. Verify the health endpoint

```bash
curl http://localhost:8080/api/health
# {"status":"ok","version":"0.1.0","db":"ok"}
```

### 9. (Optional) Install air for backend hot-reload

```bash
go install github.com/air-verse/air@latest

# From the backend directory:
cd backend && air
```

### 10. Run tests

```bash
make test
# Runs: go test ./... -race -cover  (backend)
#       npm run test --if-present   (frontend)
```

## Directory Tour

```
cms/
├── Makefile                   # All developer commands
├── .env.example               # Environment variable template
├── docker-compose.yml         # Local infrastructure
├── Dockerfile                 # Multi-stage production build
│
├── backend/
│   ├── cmd/
│   │   ├── server/main.go     # Application entry point
│   │   └── migrate/main.go    # Standalone migration runner
│   │
│   ├── internal/
│   │   ├── core/              # Pure domain logic — no external deps
│   │   │   ├── domain/        # Entity types (base.go + per-domain packages)
│   │   │   ├── events/        # Event bus interface + InMemoryBus
│   │   │   └── ports/
│   │   │       └── outbound/  # Repository + storage interfaces (ports)
│   │   │
│   │   ├── adapters/          # Implementations of ports
│   │   │   ├── http/          # Chi router, middleware, REST handlers
│   │   │   ├── events/        # KafkaBus (Watermill)
│   │   │   └── storage/       # S3Adapter + Factory
│   │   │
│   │   └── infrastructure/    # Framework-level concerns
│   │       ├── config/        # Viper config loader
│   │       ├── database/      # pgxpool connect + Goose migrations
│   │       ├── logger/        # Zap logger wrapper
│   │       ├── server/        # http.Server lifecycle
│   │       └── static/        # go:embed for frontend dist/
│   │
│   └── go.mod                 # module github.com/imaginacolombia/cms
│
├── frontend/
│   ├── src/
│   │   ├── main.tsx           # React entry point
│   │   ├── routes/            # TanStack Router tree
│   │   ├── pages/             # Page components
│   │   ├── components/        # Shared UI components (layout, ui/)
│   │   ├── store/             # Zustand stores (auth, tenant)
│   │   ├── lib/               # API client, SSE, GraphQL helpers
│   │   ├── types/             # Shared TypeScript types
│   │   └── styles/            # Tailwind globals
│   ├── vite.config.ts
│   ├── tailwind.config.js
│   └── package.json
│
└── docs/
    ├── Architecture.md
    └── implementation/        # ← You are here
```

### Key architectural layers (backend)

| Layer | Package path | Allowed dependencies |
|-------|-------------|----------------------|
| Domain | `internal/core/domain/` | Standard library only |
| Ports | `internal/core/ports/outbound/` | Domain types only |
| Events | `internal/core/events/` | Standard library only |
| Adapters | `internal/adapters/` | Core + infrastructure |
| Infrastructure | `internal/infrastructure/` | Third-party packages |

The rule: **inner layers never import outer layers**. All external dependencies (database, Kafka, S3) are hidden behind interfaces defined in `internal/core/ports/outbound/`.

## Key Files

- `/home/jd/dev/full/cms/Makefile` — all developer commands
- `/home/jd/dev/full/cms/.env.example` — environment variable reference
- `/home/jd/dev/full/cms/docker-compose.yml` — local infrastructure
- `/home/jd/dev/full/cms/backend/cmd/server/main.go` — application entry point
- `/home/jd/dev/full/cms/backend/internal/infrastructure/config/config.go` — config loader
- `/home/jd/dev/full/cms/backend/internal/infrastructure/database/postgres.go` — DB connect + migrations

## Next Steps

Proceed to [02 — Backend Foundation](./02-backend-foundation.md) to understand the hexagonal architecture and learn how to add new domain entities.

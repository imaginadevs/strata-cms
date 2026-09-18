# General production image for the whole CMS: builds the frontend, embeds it into
# the Go binary, and ships a single distroless container that serves the SPA and
# /api on the same origin (:8080). No nginx, no separate frontend service.
#
#   Build from the cms/ root:  docker build -t cms .
#   Run:                       docker run -p 8080:8080 --env-file .env cms
#   (runtime config — DB, JWT_SECRET, S3, … — comes from the container env)

# ── Stage 1: Frontend build (pnpm) ─────────────────────────────────────────────
FROM node:24-alpine AS frontend-builder
WORKDIR /build/frontend

# Corepack activates the pnpm version pinned in frontend/package.json
# ("packageManager"). Refresh it first so it can verify recent pnpm releases.
RUN npm install -g corepack@latest && corepack enable

# Install deps first for layer caching. pnpm-workspace.yaml carries the
# supply-chain settings and the allowBuilds allow-list (esbuild needs its
# postinstall to link its native binary — Vite depends on it).
COPY frontend/package.json frontend/pnpm-lock.yaml frontend/pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile

# Build (tsc --noEmit then vite build → /build/frontend/dist).
COPY frontend/ ./
RUN pnpm build

# ── Stage 2: Backend build ─────────────────────────────────────────────────────
FROM golang:1.25-alpine AS backend-builder
WORKDIR /build/backend

# Cache dependencies first
COPY backend/go.mod backend/go.sum ./
RUN go mod download

COPY backend/ ./

# Embed the compiled frontend into the binary
COPY --from=frontend-builder /build/frontend/dist ./internal/infrastructure/static/dist

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w -extldflags=-static" \
    -o /app/server \
    ./cmd/server/main.go

# ── Stage 3: Runtime (distroless — no shell, minimal attack surface) ───────────
FROM gcr.io/distroless/static-debian12 AS runtime

COPY --from=backend-builder /app/server /server

# Non-root user (distroless uses uid 65532 by default)
USER nonroot:nonroot

EXPOSE 8080

ENTRYPOINT ["/server"]

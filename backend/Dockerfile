# Backend-only image for the IM CMS Go API.
# Build context is the backend/ directory:
#   docker build -t im-cms-backend .
# (The repo-root Dockerfile builds the API *with* the admin studio embedded;
#  this one ships the API alone, for deploying the backend as its own service.)

# ── Stage 1: Build ─────────────────────────────────────────────────────────────
FROM golang:1.25-alpine AS builder
WORKDIR /build

# Cross-compilation targets provided by BuildKit (default to the build host's).
ARG TARGETOS=linux
ARG TARGETARCH=amd64

# Cache dependencies first so source edits don't re-download modules.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

# Static binary (CGO off) — required for the distroless runtime below.
# Build caches are mounted so incremental rebuilds are fast.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w -extldflags=-static" \
    -o /out/server \
    ./cmd/server

# ── Stage 2: Runtime (distroless — no shell, minimal attack surface) ───────────
FROM gcr.io/distroless/static-debian12:nonroot AS runtime

COPY --from=builder /out/server /server

# distroless:nonroot runs as uid 65532 (never root).
USER nonroot:nonroot

EXPOSE 8080

# The binary self-probes via the --health flag (GET /api/health on APP_PORT).
# distroless has no shell, so exec the binary directly (no sh -c).
HEALTHCHECK --interval=10s --timeout=5s --start-period=15s --retries=5 \
    CMD ["/server", "--health"]

ENTRYPOINT ["/server"]

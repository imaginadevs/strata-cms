# 12 — Deployment

## Overview

The CMS is packaged as a single static binary that serves both the API and the embedded React SPA. The three-stage Dockerfile builds the frontend, then the backend, then produces a minimal scratch image. Production uses docker-compose with a PostgreSQL container; Kafka can be enabled independently. This guide covers the full production setup: Docker build, environment checklist, nginx reverse proxy, health checks, database backups, and zero-downtime binary updates.

## Prerequisites

- Guide 01 completed (project structure understood).
- Docker 26+ and docker-compose v2 installed on the production server.
- A PostgreSQL 15+ instance (managed service recommended for production).
- Domain name pointing to the server's IP.

## Step-by-Step

### 1. Three-stage Dockerfile (production)

The existing `Dockerfile` at the repo root uses this pattern:

```dockerfile
# Stage 1: Build frontend
FROM node:20-alpine AS frontend-builder
WORKDIR /app/frontend
COPY frontend/package*.json ./
RUN npm ci --frozen-lockfile
COPY frontend/ ./
RUN npm run build
# Output: /app/frontend/dist/

# Stage 2: Build backend binary
FROM golang:1.22-alpine AS backend-builder
WORKDIR /app/backend

# Download modules first (separate layer for cache efficiency)
COPY backend/go.mod backend/go.sum ./
RUN go mod download

# Copy frontend dist into the expected embed location
COPY --from=frontend-builder /app/frontend/dist \
     /app/backend/internal/infrastructure/static/dist/

# Copy backend source
COPY backend/ ./

# Build static binary (CGO_ENABLED=0 for scratch compatibility)
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" \
    -o /app/bin/server ./cmd/server/main.go

# Stage 3: Minimal runtime image
FROM scratch
COPY --from=backend-builder /app/bin/server /server
# Include CA certs for outbound HTTPS (S3, Logto, Kafka)
COPY --from=backend-builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD ["/server", "--health"]

ENTRYPOINT ["/server"]
```

Build the production image:

```bash
make docker-build
# Equivalent to: docker build -t cms:latest .

# Tag for a registry
docker tag cms:latest ghcr.io/imaginacolombia/cms:v1.0.0
docker push ghcr.io/imaginacolombia/cms:v1.0.0
```

### 2. docker-compose production setup

Create `docker-compose.prod.yml`:

```yaml
version: "3.9"

services:
  cms:
    image: ghcr.io/imaginacolombia/cms:v1.0.0
    restart: unless-stopped
    ports:
      - "127.0.0.1:8080:8080"   # Only expose to localhost; nginx proxies externally
    env_file:
      - .env.production
    depends_on:
      postgres:
        condition: service_healthy
    healthcheck:
      test: ["/server", "--health"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 15s
    logging:
      driver: "json-file"
      options:
        max-size: "100m"
        max-file: "5"

  postgres:
    image: postgres:15-alpine
    restart: unless-stopped
    environment:
      POSTGRES_DB: cms
      POSTGRES_USER: cms
      POSTGRES_PASSWORD: ${DB_PASSWORD}
    volumes:
      - postgres_data:/var/lib/postgresql/data
      - ./scripts/postgres-init.sql:/docker-entrypoint-initdb.d/init.sql:ro
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U cms"]
      interval: 10s
      timeout: 5s
      retries: 5
    ports:
      - "127.0.0.1:5432:5432"   # Bind to localhost only

  # Optional: Kafka (only if ENABLE_KAFKA=true)
  zookeeper:
    image: confluentinc/cp-zookeeper:7.6.0
    restart: unless-stopped
    environment:
      ZOOKEEPER_CLIENT_PORT: 2181
    profiles: ["kafka"]

  kafka:
    image: confluentinc/cp-kafka:7.6.0
    restart: unless-stopped
    depends_on: [zookeeper]
    environment:
      KAFKA_BROKER_ID: 1
      KAFKA_ZOOKEEPER_CONNECT: zookeeper:2181
      KAFKA_ADVERTISED_LISTENERS: PLAINTEXT://kafka:9092
      KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR: 1
      KAFKA_AUTO_CREATE_TOPICS_ENABLE: "true"
    profiles: ["kafka"]

volumes:
  postgres_data:
```

Start without Kafka:

```bash
docker compose -f docker-compose.prod.yml up -d
```

Start with Kafka:

```bash
docker compose -f docker-compose.prod.yml --profile kafka up -d
```

### 3. Environment variables checklist

Create `.env.production` on the server (never commit this file):

```dotenv
# ── Application ─────────────────────────────────────────────────────────────
APP_ENV=production
APP_PORT=8080
APP_HOST=0.0.0.0

# ── Database ─────────────────────────────────────────────────────────────────
# Use a managed PostgreSQL URL in production (e.g. AWS RDS, Supabase, Neon)
DB_HOST=your-postgres-host.rds.amazonaws.com
DB_PORT=5432
DB_NAME=cms
DB_USER=cms
DB_PASSWORD=<strong-random-password>
DB_MAX_CONNS=25
DB_MIN_CONNS=5

# ── Auth ─────────────────────────────────────────────────────────────────────
# CRITICAL: Generate with: openssl rand -hex 32
JWT_SECRET=<64-char-hex-secret>
JWT_EXPIRY=24h
REFRESH_TOKEN_EXPIRY=168h

# ── Super admin IP whitelist ───────────────────────────────────────────────
# Empty = super admin login disabled (safe default)
# Set to your office/VPN CIDR in production
SUPER_ADMIN_ALLOWED_IPS=203.0.113.0/24

# ── SSO — Logto ───────────────────────────────────────────────────────────────
ENABLE_SSO=false
# LOGTO_ENDPOINT=https://your-tenant.logto.app
# LOGTO_APP_ID=your-app-id
# LOGTO_APP_SECRET=your-app-secret
# LOGTO_REDIRECT_URI=https://your-domain.com/api/auth/sso/callback

# ── Kafka ─────────────────────────────────────────────────────────────────────
ENABLE_KAFKA=false
# KAFKA_BROKERS=kafka:9092
# KAFKA_TOPIC_PREFIX=cms

# ── Features ─────────────────────────────────────────────────────────────────
ENABLE_GRAPHQL=true
ENABLE_REST=true

# ── Storage encryption key ────────────────────────────────────────────────────
# Generate with: openssl rand -hex 32
STORAGE_ENCRYPTION_KEY=<64-char-hex-key>
```

Security checklist before deploying:

```
[ ] JWT_SECRET is at least 32 random bytes
[ ] DB_PASSWORD is not the default "cms_secret"
[ ] SUPER_ADMIN_ALLOWED_IPS is set correctly (or empty to disable)
[ ] .env.production is chmod 600 (readable only by root/deploy user)
[ ] .env.production is NOT in the Docker image (env_file, not COPY)
[ ] PostgreSQL port 5432 is NOT exposed to the public internet
[ ] All secrets are rotated from development defaults
```

### 4. Health check endpoint

The health endpoint is mounted by `rest.MountHealth()` and returns:

```bash
curl https://your-domain.com/api/health
# {"status":"ok","version":"0.1.0","db":"ok","uptime":"2h15m"}
```

The binary also supports `--health` flag for Docker's HEALTHCHECK:

```go
// From backend/cmd/server/main.go
if len(os.Args) > 1 && os.Args[1] == "--health" {
    resp, err := http.Get("http://localhost:" + config.Load().Server.Port + "/api/health")
    if err != nil || resp.StatusCode != 200 {
        os.Exit(1)
    }
    os.Exit(0)
}
```

### 5. Nginx reverse proxy configuration

Install nginx on the host. Create `/etc/nginx/sites-available/cms`:

```nginx
# Rate limiting zones
limit_req_zone $binary_remote_addr zone=api:10m rate=60r/m;
limit_req_zone $binary_remote_addr zone=auth:10m rate=10r/m;

upstream cms_backend {
    server 127.0.0.1:8080;
    keepalive 32;
}

server {
    listen 80;
    server_name your-domain.com www.your-domain.com;
    return 301 https://$server_name$request_uri;
}

server {
    listen 443 ssl http2;
    server_name your-domain.com www.your-domain.com;

    # TLS — use certbot/Let's Encrypt
    ssl_certificate     /etc/letsencrypt/live/your-domain.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/your-domain.com/privkey.pem;
    ssl_protocols       TLSv1.2 TLSv1.3;
    ssl_ciphers         ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256;
    ssl_prefer_server_ciphers off;

    # Security headers
    add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;
    add_header X-Content-Type-Options nosniff always;
    add_header X-Frame-Options DENY always;
    add_header Referrer-Policy strict-origin-when-cross-origin always;

    # SSE endpoint — disable buffering
    location /api/events {
        proxy_pass         http://cms_backend;
        proxy_http_version 1.1;
        proxy_set_header   Connection "";
        proxy_buffering    off;
        proxy_cache        off;
        proxy_read_timeout 3600s;   # Keep SSE connections open for 1 hour
        proxy_set_header   Host $host;
        proxy_set_header   X-Real-IP $remote_addr;
        proxy_set_header   X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header   X-Forwarded-Proto $scheme;
        limit_req zone=api burst=20 nodelay;
    }

    # Auth endpoints — stricter rate limit
    location ~ ^/api/auth/(login|refresh|register) {
        proxy_pass         http://cms_backend;
        proxy_http_version 1.1;
        proxy_set_header   Connection "keep-alive";
        proxy_set_header   Host $host;
        proxy_set_header   X-Real-IP $remote_addr;
        proxy_set_header   X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header   X-Forwarded-Proto $scheme;
        limit_req zone=auth burst=5 nodelay;
    }

    # All other API routes
    location /api/ {
        proxy_pass         http://cms_backend;
        proxy_http_version 1.1;
        proxy_set_header   Connection "keep-alive";
        proxy_set_header   Host $host;
        proxy_set_header   X-Real-IP $remote_addr;
        proxy_set_header   X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header   X-Forwarded-Proto $scheme;
        proxy_read_timeout 60s;
        limit_req zone=api burst=30 nodelay;
    }

    # SPA — let the Go binary serve the embedded React app
    location / {
        proxy_pass         http://cms_backend;
        proxy_http_version 1.1;
        proxy_set_header   Connection "keep-alive";
        proxy_set_header   Host $host;
        proxy_set_header   X-Real-IP $remote_addr;
        proxy_set_header   X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header   X-Forwarded-Proto $scheme;
        # Cache static assets aggressively
        location ~* \.(js|css|woff2?|png|jpg|svg|ico)$ {
            proxy_pass http://cms_backend;
            proxy_cache_valid 200 365d;
            add_header Cache-Control "public, max-age=31536000, immutable";
        }
    }
}
```

Enable and reload:

```bash
ln -s /etc/nginx/sites-available/cms /etc/nginx/sites-enabled/
nginx -t && systemctl reload nginx

# Install Let's Encrypt certificate
certbot --nginx -d your-domain.com -d www.your-domain.com
```

### 6. Enabling Kafka

```bash
# Edit .env.production
ENABLE_KAFKA=true
KAFKA_BROKERS=kafka:9092
KAFKA_TOPIC_PREFIX=cms

# Restart with Kafka profile
docker compose -f docker-compose.prod.yml --profile kafka up -d
```

Verify Kafka is receiving events:

```bash
# List topics (after first event is published)
docker compose exec kafka kafka-topics --bootstrap-server localhost:9092 --list
# cms.items.after.create
# cms.items.after.update
# cms.auth.login
```

### 7. Enabling SSO (Logto)

1. Create an application in your Logto tenant console.
2. Set the redirect URI to `https://your-domain.com/api/auth/sso/callback`.
3. Update `.env.production`:

```dotenv
ENABLE_SSO=true
LOGTO_ENDPOINT=https://your-tenant.logto.app
LOGTO_APP_ID=abc123def456
LOGTO_APP_SECRET=<secret-from-logto-console>
LOGTO_REDIRECT_URI=https://your-domain.com/api/auth/sso/callback
```

4. Restart the CMS container:

```bash
docker compose -f docker-compose.prod.yml up -d cms
```

The SSO login button appears in the frontend login page when `ENABLE_SSO=true`.

### 8. Database backup strategy

#### Automated daily backups with pg_dump

Create `/etc/cron.daily/cms-backup`:

```bash
#!/bin/bash
set -e

BACKUP_DIR=/var/backups/cms-postgres
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
DB_URL="postgresql://cms:${DB_PASSWORD}@localhost:5432/cms"

mkdir -p "$BACKUP_DIR"

# Dump compressed SQL
docker compose -f /opt/cms/docker-compose.prod.yml exec -T postgres \
    pg_dump -U cms cms | gzip > "$BACKUP_DIR/cms_$TIMESTAMP.sql.gz"

# Keep only last 7 days
find "$BACKUP_DIR" -name "*.sql.gz" -mtime +7 -delete

# Upload to S3 (optional)
# aws s3 cp "$BACKUP_DIR/cms_$TIMESTAMP.sql.gz" s3://your-backup-bucket/postgres/
```

```bash
chmod +x /etc/cron.daily/cms-backup
```

#### Restore from backup

```bash
gunzip -c /var/backups/cms-postgres/cms_20240101_020000.sql.gz | \
    docker compose -f docker-compose.prod.yml exec -T postgres \
    psql -U cms cms
```

#### Point-in-time recovery (managed databases)

For AWS RDS or Supabase, enable automated backups and point-in-time recovery in the console — no additional configuration required in this project.

### 9. Zero-downtime update (binary rolling update)

Since the CMS runs as a Docker container and stores no local state (all state is in PostgreSQL + S3), updates are blue-green by default:

```bash
# 1. Pull the new image
docker pull ghcr.io/imaginacolombia/cms:v1.1.0

# 2. Update the image tag in docker-compose.prod.yml
sed -i 's/cms:v1.0.0/cms:v1.1.0/' docker-compose.prod.yml

# 3. Rolling update (docker compose recreates with zero-downtime for single replica)
docker compose -f docker-compose.prod.yml up -d --no-deps cms

# Docker will:
# a. Start the new container (migrations run automatically on startup)
# b. Wait for the new container to pass the HEALTHCHECK
# c. Stop the old container
# Nginx upstream keeps-alive mean in-flight requests complete before the old container stops.
```

For true blue-green with two replicas, use a load balancer that supports health-check-based draining (nginx upstream with health_check module, or a cloud load balancer).

### 10. Monitoring and observability

The backend uses structured JSON logging via Zap. In production, ship logs to your observability stack:

```bash
# View live logs
docker compose -f docker-compose.prod.yml logs -f cms

# Stream to Grafana Loki (with docker logging driver)
# See: https://grafana.com/docs/loki/latest/send-data/docker-driver/
```

Key metrics to monitor:

| Metric | Source | Alert threshold |
|--------|--------|----------------|
| `GET /api/health` | nginx access log | Non-200 for 2+ minutes |
| `pg_pool_acquire_count` | Viper config | — |
| Container restart count | docker stats | > 2 restarts in 10 min |
| Disk usage | host | > 80% |
| Response time p99 | nginx log | > 2s |
| S3 upload errors | Go logs | Any |

Prometheus + Grafana setup (optional):

```bash
# Add to docker-compose.prod.yml
  prometheus:
    image: prom/prometheus:latest
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml
  
  grafana:
    image: grafana/grafana:latest
    ports:
      - "127.0.0.1:3000:3000"
```

### 11. Production deployment runbook

```bash
# ── First deployment ──────────────────────────────────────────────────────
git clone https://github.com/imaginacolombia/cms.git /opt/cms
cd /opt/cms

# Copy and edit production env
cp .env.example .env.production
nano .env.production   # fill in all values per the checklist above

# Build or pull image
make docker-build
# OR: docker pull ghcr.io/imaginacolombia/cms:v1.0.0

# Start services
docker compose -f docker-compose.prod.yml up -d

# Verify
curl https://your-domain.com/api/health
# {"status":"ok","db":"ok"}

# ── Subsequent deployments ────────────────────────────────────────────────
cd /opt/cms
git pull
make docker-build
docker compose -f docker-compose.prod.yml up -d --no-deps cms

# ── Rollback ──────────────────────────────────────────────────────────────
# Edit docker-compose.prod.yml to previous image tag, then:
docker compose -f docker-compose.prod.yml up -d --no-deps cms
```

## Key Files

- `/home/jd/dev/full/cms/Dockerfile` — three-stage production build
- `/home/jd/dev/full/cms/docker-compose.yml` — development services
- `/home/jd/dev/full/cms/.env.example` — environment variable template
- `/home/jd/dev/full/cms/Makefile` — build commands
- `/home/jd/dev/full/cms/backend/cmd/server/main.go` — `--health` flag + graceful shutdown
- `/home/jd/dev/full/cms/backend/internal/adapters/http/rest/health.go` — health endpoint
- `/home/jd/dev/full/cms/backend/internal/infrastructure/config/config.go` — all config fields

## Summary: Feature Flag Quick Reference

| Feature | Enable | Config |
|---------|--------|--------|
| REST API | `ENABLE_REST=true` (default) | — |
| GraphQL | `ENABLE_GRAPHQL=true` (default) | — |
| Kafka events | `ENABLE_KAFKA=true` | `KAFKA_BROKERS`, `KAFKA_TOPIC_PREFIX` |
| Logto SSO | `ENABLE_SSO=true` | `LOGTO_ENDPOINT`, `LOGTO_APP_ID`, `LOGTO_APP_SECRET`, `LOGTO_REDIRECT_URI` |
| Super admin | `SUPER_ADMIN_ALLOWED_IPS` non-empty | IP/CIDR whitelist |
| S3 storage | Per tenant via Settings API | Stored encrypted in `tenants.settings` |

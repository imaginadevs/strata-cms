.PHONY: dev dev-backend dev-frontend build docker-build docker-up docker-down \
        migrate-up migrate-down migrate-create lint test clean setup

# ── Development ────────────────────────────────────────────────────────────────
dev:
	@make -j2 dev-backend dev-frontend

dev-backend:
	@echo "▶ Starting backend..."
	@cd backend && go run ./cmd/server/main.go

dev-frontend:
	@echo "▶ Starting frontend..."
	@cd frontend && npm run dev

# ── Build ──────────────────────────────────────────────────────────────────────
build: build-frontend build-backend

build-frontend:
	@echo "▶ Building frontend..."
	@cd frontend && npm ci && npm run build

build-backend: build-frontend
	@echo "▶ Embedding frontend into backend..."
	@rm -rf backend/internal/infrastructure/static/dist
	@mkdir -p backend/internal/infrastructure/static/dist
	@cp -r frontend/dist/. backend/internal/infrastructure/static/dist/
	@echo "▶ Building backend..."
	@mkdir -p bin
	@cd backend && CGO_ENABLED=0 go build -ldflags="-s -w" -o ../bin/server ./cmd/server/main.go

# ── Docker ─────────────────────────────────────────────────────────────────────
docker-build:
	@docker build -t cms:latest .

docker-up:
	@docker-compose up -d

docker-down:
	@docker-compose down

docker-logs:
	@docker-compose logs -f

# ── Database migrations (via goose inside backend) ─────────────────────────────
migrate-up:
	@cd backend && go run ./cmd/migrate/main.go up

migrate-down:
	@cd backend && go run ./cmd/migrate/main.go down

migrate-create:
	@cd backend && go run ./cmd/migrate/main.go create $(name) sql

# ── Code quality ───────────────────────────────────────────────────────────────
lint:
	@cd backend && golangci-lint run ./...
	@cd frontend && npm run lint

test:
	@cd backend && go test ./... -race -cover
	@cd frontend && npm run test --if-present

# ── Setup (first time) ─────────────────────────────────────────────────────────
setup:
	@cp -n .env.example .env || true
	@cd frontend && npm install
	@cd backend && go mod download
	@echo "✓ Setup complete. Edit .env then run: make dev"

clean:
	@rm -rf bin/ frontend/dist/ backend/tmp/
	@echo "✓ Cleaned"

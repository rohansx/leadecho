.PHONY: help up down logs migrate-up migrate-down migrate-status migrate-new install build clean check web-dev web-build web-preview web-check api-dev api-build api-test seed sqlc fmt lint db-up db-down db-reset db-connect redis-connect

# ──────────────────────────────────────────────
# leadecho — monorepo makefile
# ──────────────────────────────────────────────

# Load .env if present
ifneq (,$(wildcard ./.env))
    include .env
    export
endif

GO    := go
PNPM  := pnpm
GOOSE := $(shell $(GO) env GOPATH)/bin/goose
SQLC  := $(shell $(GO) env GOPATH)/bin/sqlc
API_PID := /tmp/leadecho-api.pid
WEB_PID := /tmp/leadecho-web.pid

help: ## Show this help
	@grep -hE '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[33m%-20s\033[0m %s\n", $$1, $$2}'

# ──────────────────────────────────────────────
# Quick commands
# ──────────────────────────────────────────────

up: db-up ## Start DB + API + frontend (backgrounded, use `make logs` to watch)
	@$(MAKE) migrate-up 2>/dev/null || true
	@mkdir -p /tmp/leadecho-logs
	@echo "Building API..."
	@cd backend && $(GO) build -o bin/leadecho ./cmd/api
	@echo "Starting API server..."
	@bash -c 'cd backend && exec ./bin/leadecho > /tmp/leadecho-logs/api.log 2>&1' & echo $$! > $(API_PID)
	@echo "Starting frontend dev server..."
	@bash -c 'cd dashboard && exec $(PNPM) dev > /tmp/leadecho-logs/web.log 2>&1' & echo $$! > $(WEB_PID)
	@sleep 2
	@printf '\n'
	@printf '  \033[33m✓\033[0m Postgres → localhost:5433\n'
	@printf '  \033[33m✓\033[0m Redis    → localhost:6380\n'
	@printf '  \033[33m✓\033[0m API      → http://localhost:8090\n'
	@printf '  \033[33m✓\033[0m Web      → http://localhost:3100\n'
	@printf '\n'
	@printf '  Run \033[33mmake logs\033[0m to tail output\n'
	@printf '  Run \033[33mmake down\033[0m to stop\n'

down: ## Stop all running servers + Docker
	@if [ -f $(API_PID) ]; then \
		pid=$$(cat $(API_PID)); \
		kill $$pid 2>/dev/null && echo "API stopped (pid $$pid)." || echo "API not running."; \
		rm -f $(API_PID); \
	fi
	@if [ -f $(WEB_PID) ]; then \
		pid=$$(cat $(WEB_PID)); \
		kill $$pid 2>/dev/null && echo "Web stopped (pid $$pid)." || echo "Web not running."; \
		rm -f $(WEB_PID); \
	fi
	@# Fallback: kill anything still on our ports (orphaned processes)
	@lsof -ti :8090 2>/dev/null | xargs -r kill 2>/dev/null && echo "Killed orphan on :8090" || true
	@lsof -ti :3100 2>/dev/null | xargs -r kill 2>/dev/null && echo "Killed orphan on :3100" || true
	@docker compose down 2>/dev/null || true
	@printf 'All servers stopped.\n'

logs: ## Tail logs from API + frontend
	@tail -f /tmp/leadecho-logs/api.log /tmp/leadecho-logs/web.log

logs-api: ## Tail API logs only
	@tail -f /tmp/leadecho-logs/api.log

logs-web: ## Tail frontend logs only
	@tail -f /tmp/leadecho-logs/web.log

# ──────────────────────────────────────────────
# Database
# ──────────────────────────────────────────────

db-up: ## Start Docker services (Postgres + Redis)
	@docker compose up -d
	@printf '  Waiting for PostgreSQL...'
	@for i in 1 2 3 4 5 6 7 8 9 10; do \
		if docker exec leadecho-postgres pg_isready -U leadecho -q 2>/dev/null; then \
			printf ' \033[32mready\033[0m\n'; \
			break; \
		fi; \
		printf '.'; \
		sleep 1; \
	done

db-down: ## Stop Docker services (keeps data)
	@docker compose down
	@echo "DB stopped."

db-reset: ## Destroy and recreate database (WARNING: deletes all data)
	@echo "Destroying containers and volumes..."
	@docker compose down -v
	@$(MAKE) db-up
	@echo "Running migrations..."
	@$(MAKE) migrate-up
	@echo "Seeding dev data..."
	@$(MAKE) seed

db-connect: ## Connect to PostgreSQL via psql
	psql "$(DATABASE_URL)"

redis-connect: ## Connect to Redis via redis-cli
	redis-cli -p 6380

# ──────────────────────────────────────────────
# Database migrations (goose)
# ──────────────────────────────────────────────

migrate-up: ## Run all pending migrations
	cd backend && $(GOOSE) -dir migrations postgres "$(DATABASE_URL)" up

migrate-down: ## Roll back last migration
	cd backend && $(GOOSE) -dir migrations postgres "$(DATABASE_URL)" down

migrate-status: ## Show migration status
	cd backend && $(GOOSE) -dir migrations postgres "$(DATABASE_URL)" status

migrate-new: ## Create new migration (usage: make migrate-new name=add_column)
	cd backend && $(GOOSE) -dir migrations create $(name) sql

# ──────────────────────────────────────────────
# Install / Build
# ──────────────────────────────────────────────

install: ## Install all dependencies
	cd dashboard && $(PNPM) install
	cd backend && $(GO) mod tidy
	@echo "Installing Go tools (goose, sqlc, golangci-lint)..."
	$(GO) install github.com/pressly/goose/v3/cmd/goose@latest
	$(GO) install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
	$(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

build: web-build api-build ## Production build (frontend + backend)

clean: ## Remove build artifacts and logs
	rm -rf backend/bin dashboard/dist dashboard/node_modules/.vite /tmp/leadecho-logs
	@echo "Cleaned."

check: web-check api-test ## Type check + test everything

# ──────────────────────────────────────────────
# Frontend (dashboard/)
# ──────────────────────────────────────────────

web-dev: ## Start Vite dashboard dev server (foreground)
	cd dashboard && $(PNPM) dev

web-build: ## Build dashboard for production
	cd dashboard && $(PNPM) build

web-preview: ## Preview production build locally
	cd dashboard && $(PNPM) preview

web-check: ## Type check frontend
	cd dashboard && $(PNPM) tsc -b

# ──────────────────────────────────────────────
# Backend (backend/)
# ──────────────────────────────────────────────

api-dev: ## Start Go API server (foreground)
	cd backend && $(GO) run ./cmd/api

api-build: ## Build Go binary
	cd backend && CGO_ENABLED=0 $(GO) build -o bin/leadecho ./cmd/api

api-test: ## Run Go tests
	cd backend && $(GO) test -race -count=1 ./...

sqlc: ## Generate Go code from SQL queries
	cd backend && $(SQLC) generate

seed: ## Run seed migration (dev data)
	@echo "Seed data is applied via migrations (00002_seed_dev_data.sql)."
	@echo "Run 'make migrate-up' to apply."

# ──────────────────────────────────────────────
# Utilities
# ──────────────────────────────────────────────

fmt: ## Format all code
	cd backend && $(GO) fmt ./...

lint: ## Lint all code
	cd backend && golangci-lint run ./... 2>/dev/null || true
	cd dashboard && $(PNPM) tsc -b

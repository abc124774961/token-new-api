FRONTEND_CLASSIC_DIR = ./web/classic
BACKEND_DIR = .

.PHONY: all build-frontend build-frontend-classic build-all-frontends start-backend dev dev-api dev-up dev-down pro-up pro-down dev-web dev-web-classic

all: build-all-frontends start-backend

build-frontend: build-frontend-classic

build-frontend-classic:
	@echo "Building classic frontend..."
	@cd $(FRONTEND_CLASSIC_DIR) && bun install && VITE_REACT_APP_VERSION=$(cat ../../VERSION) bun run build

build-all-frontends: build-frontend-classic

start-backend:
	@echo "Starting backend dev server..."
	@cd $(BACKEND_DIR) && go run main.go &

dev-api:
	@echo "Starting local backend container (dev)..."
	@mkdir -p data logs
	@docker compose --env-file .env.dev -f docker-compose.dev.yml up -d --build

dev-up: dev-api

dev-down:
	@echo "Stopping local full-stack container (dev)..."
	@docker compose --env-file .env.dev -f docker-compose.dev.yml down

pro-up:
	@echo "Starting production container set (pro)..."
	@mkdir -p data logs
	@docker compose --env-file .env.pro -f docker-compose.pro.yml up -d --build

pro-down:
	@echo "Stopping production container set (pro)..."
	@docker compose --env-file .env.pro -f docker-compose.pro.yml down

dev-web: dev-web-classic

dev-web-classic:
	@echo "Starting classic frontend dev server..."
	@APP_PORT_VALUE=$$(awk -F= '/^APP_PORT=/{print $$2}' .env.dev 2>/dev/null | tail -n 1); \
	FRONTEND_PORT_VALUE=$$(awk -F= '/^FRONTEND_PORT=/{print $$2}' .env.dev 2>/dev/null | tail -n 1); \
	cd $(FRONTEND_CLASSIC_DIR) && bun install && DEV_PROXY_TARGET=$${DEV_PROXY_TARGET:-http://localhost:$${APP_PORT:-$${APP_PORT_VALUE:-3001}}} bun run dev -- --host 0.0.0.0 --port $${FRONTEND_PORT:-$${FRONTEND_PORT_VALUE:-3000}}

dev: dev-api dev-web-classic

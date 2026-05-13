FRONTEND_DIR = ./web/default
FRONTEND_CLASSIC_DIR = ./web/classic
BACKEND_DIR = .

.PHONY: all build-frontend build-frontend-classic build-all-frontends start-backend dev dev-api dev-up dev-down pro-up pro-down dev-web dev-web-classic

all: build-all-frontends start-backend

build-frontend:
	@echo "Building default frontend..."
	@cd $(FRONTEND_DIR) && bun install && DISABLE_ESLINT_PLUGIN='true' VITE_REACT_APP_VERSION=$(cat ../../VERSION) bun run build

build-frontend-classic:
	@echo "Building classic frontend..."
	@cd $(FRONTEND_CLASSIC_DIR) && bun install && VITE_REACT_APP_VERSION=$(cat ../../VERSION) bun run build

build-all-frontends: build-frontend build-frontend-classic

start-backend:
	@echo "Starting backend dev server..."
	@cd $(BACKEND_DIR) && go run main.go &

dev-api:
	@echo "Starting local full-stack container (dev)..."
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

dev-web:
	@echo "Starting frontend dev server..."
	@cd $(FRONTEND_DIR) && bun install && bun run dev

dev-web-classic:
	@echo "Starting classic frontend dev server..."
	@cd $(FRONTEND_CLASSIC_DIR) && bun install && bun run dev

dev: dev-api

.PHONY: up down logs ps setup sqlc proto help

COMPOSE ?= docker compose -f infra/docker-compose.yml --env-file .env

up: setup       ## Build and start the full stack (UI at http://localhost:8080)
	$(COMPOSE) up --build -d
	@echo "Quillit is starting — open http://localhost:8080"

down:           ## Stop and remove all containers
	$(COMPOSE) down

logs:           ## Tail logs from all services
	$(COMPOSE) logs -f

ps:             ## Show service status
	$(COMPOSE) ps

setup:          ## Create .env from .env.example if missing; warn on placeholder secrets
	@test -f .env || (cp .env.example .env && echo "Created .env from .env.example — edit it to set real secrets")
	@grep -q '^JWT_SECRET=replace-with' .env && \
		echo "WARNING: JWT_SECRET is still the placeholder. Generate one with: openssl rand -hex 32" || true
	@grep -q '^MINIO_PASSWORD=replace-with' .env && \
		echo "WARNING: MINIO_PASSWORD is still the placeholder." || true

sqlc:           ## Regenerate sqlc code from sqlc.yaml (svc, auth, content queries)
	go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 generate

proto:          ## Regenerate Go/connect-go stubs under gen/ from proto/*.proto
	cd proto && go run github.com/bufbuild/buf/cmd/buf@v1.72.0 generate

help:           ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*##' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*##"}; {printf "  \033[36m%-8s\033[0m %s\n", $$1, $$2}'

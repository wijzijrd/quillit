.PHONY: dev build build-linux setup check-env swagger help

setup:          ## Copy .env.example → .env (skips if already exists)
	@test -f .env && echo ".env already exists, skipping" \
		|| (cp .env.example .env && echo "Created .env — edit JWT_SECRET and AUTH_SERVICE_URL before running")

check-env:
	@test -f .env || (echo "ERROR: .env missing. Run 'make setup' first." && exit 1)
	@grep -q '^JWT_SECRET=' .env || (echo "ERROR: JWT_SECRET not set in .env" && exit 1)
	@grep '^JWT_SECRET=' .env | grep -q 'replace-with' \
		&& (echo "ERROR: JWT_SECRET is still the placeholder. Generate one with: openssl rand -hex 32" && exit 1) || true
	@grep '^JWT_SECRET=' .env | grep -q '[\$$`]' \
		&& (echo "ERROR: JWT_SECRET contains shell-special characters (\$$ or backtick). Use: openssl rand -hex 32" && exit 1) || true

dev: check-env  ## Run dev server (loads .env automatically)
	@export $$(cat .env | xargs) && go run .

swagger:        ## Regenerate Swagger docs
	swag init --parseDependency --parseInternal

build:          ## Compile binary for current OS/arch (local dev)
	go build -o bin/quillit-svc .

build-linux:    ## Cross-compile binary for Linux (run before docker compose up --build)
	CGO_ENABLED=0 GOOS=linux go build -o bin/quillit-svc .

help:           ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*##' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*##"}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

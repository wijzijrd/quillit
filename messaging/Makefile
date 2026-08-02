.PHONY: dev build build-linux setup check-env swagger help

setup:          ## Copy .env.example → .env (skips if already exists)
	@test -f .env && echo ".env already exists, skipping" \
		|| (cp .env.example .env && echo "Created .env — edit MESSAGING_SECRET before running")

check-env:
	@test -f .env || (echo "ERROR: .env missing. Run 'make setup' first." && exit 1)

dev: check-env  ## Run with hot-reload (requires Air: go install github.com/air-verse/air@latest)
	@export $$(cat .env | xargs) && go run .

swagger:        ## Regenerate Swagger docs
	swag init --parseDependency --parseInternal

build:          ## Compile binary for current OS/arch (local dev)
	go build -o bin/quillit-messaging-svc .

build-linux:    ## Cross-compile binary for Linux (run before docker compose up --build)
	CGO_ENABLED=0 GOOS=linux go build -o bin/quillit-messaging-svc .

help:           ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*##' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*##"}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

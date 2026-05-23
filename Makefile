.PHONY: dev build setup help

setup:          ## Copy .env.example → .env (skips if already exists)
	@test -f .env && echo ".env already exists, skipping" \
		|| (cp .env.example .env && echo "Created .env")

dev:            ## Start Vite dev server on :5173
	npm run dev

build:          ## Build for production
	npm run build

help:           ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*##' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*##"}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

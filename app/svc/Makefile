BINARY := quillit-svc
SETUP_HINT := JWT_SECRET and AUTH_SERVICE_URL

.PHONY: check-env

check-env:
	@test -f .env || (echo "ERROR: .env missing. Run 'make setup' first." && exit 1)
	@grep -q '^JWT_SECRET=' .env || (echo "ERROR: JWT_SECRET not set in .env" && exit 1)
	@grep '^JWT_SECRET=' .env | grep -q 'replace-with' \
		&& (echo "ERROR: JWT_SECRET is still the placeholder. Generate one with: openssl rand -hex 32" && exit 1) || true
	@grep '^JWT_SECRET=' .env | grep -q '[\$$`]' \
		&& (echo "ERROR: JWT_SECRET contains shell-special characters (\$$ or backtick). Use: openssl rand -hex 32" && exit 1) || true

include ../Makefile.common

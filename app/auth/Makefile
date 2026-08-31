BINARY := quillit-auth-svc
SETUP_HINT := JWT_SECRET
DB_PATH ?= ./quillit-auth.db

.PHONY: check-env

check-env:
	@test -f .env || (echo "ERROR: .env missing. Run 'make setup' first." && exit 1)

include ../Makefile.common

BINARY := quillit-content-svc
SETUP_HINT := (none required — PORT and DB_PATH both have defaults)

# content's DB file is named differently from svc's default (see .env.example).
DB_PATH ?= ./quillit-content.db

.PHONY: check-env

check-env:
	@test -f .env || (echo "ERROR: .env missing. Run 'make setup' first." && exit 1)

include ../Makefile.common

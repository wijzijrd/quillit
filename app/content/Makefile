BINARY := quillit-content-svc
SETUP_HINT := (none required — PORT and DB_PATH both have defaults)

.PHONY: check-env

check-env:
	@test -f .env || (echo "ERROR: .env missing. Run 'make setup' first." && exit 1)

include ../Makefile.common

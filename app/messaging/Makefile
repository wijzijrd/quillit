BINARY := quillit-messaging-svc
SETUP_HINT := MESSAGING_SECRET

.PHONY: check-env

check-env:
	@test -f .env || (echo "ERROR: .env missing. Run 'make setup' first." && exit 1)

include ../Makefile.common

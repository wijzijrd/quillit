#!/usr/bin/env bash
set -euo pipefail

# ── Quillit backup ────────────────────────────────────────────────────────────
# Backs up SQLite databases (auth + main) and MinIO blob data.
# Run from the repo root. Safe to run while services are live (SQLite WAL mode).

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DATE=$(date +%Y-%m-%d_%H-%M-%S)
BACKUP_DIR="${REPO_DIR}/backups/${DATE}"

GREEN='\033[0;32m'; BOLD='\033[1m'; NC='\033[0m'
info() { echo -e "${GREEN}[+]${NC} $*"; }

mkdir -p "$BACKUP_DIR"
cd "$REPO_DIR"

COMPOSE_CMD="./compose.sh"
if [[ ! -x "$COMPOSE_CMD" ]]; then
    COMPOSE_CMD="docker compose"
fi

# ── SQLite: svc ───────────────────────────────────────────────────────────────
info "Backing up quillit.db..."
$COMPOSE_CMD exec -T svc sh -c \
    'sqlite3 /data/quillit.db ".backup /data/quillit.db.bak"' 2>/dev/null \
    || $COMPOSE_CMD exec -T svc cp /data/quillit.db /data/quillit.db.bak

SVC_CONTAINER=$($COMPOSE_CMD ps -q svc)
docker cp "${SVC_CONTAINER}:/data/quillit.db.bak" "${BACKUP_DIR}/quillit.db"

# ── SQLite: auth ──────────────────────────────────────────────────────────────
info "Backing up quillit-auth.db..."
$COMPOSE_CMD exec -T auth sh -c \
    'sqlite3 /data/quillit-auth.db ".backup /data/quillit-auth.db.bak"' 2>/dev/null \
    || $COMPOSE_CMD exec -T auth cp /data/quillit-auth.db /data/quillit-auth.db.bak

AUTH_CONTAINER=$($COMPOSE_CMD ps -q auth)
docker cp "${AUTH_CONTAINER}:/data/quillit-auth.db.bak" "${BACKUP_DIR}/quillit-auth.db"

# ── SQLite: content ───────────────────────────────────────────────────────────
info "Backing up quillit-content.db..."
$COMPOSE_CMD exec -T content sh -c \
    'sqlite3 /data/quillit-content.db ".backup /data/quillit-content.db.bak"' 2>/dev/null \
    || $COMPOSE_CMD exec -T content cp /data/quillit-content.db /data/quillit-content.db.bak

CONTENT_CONTAINER=$($COMPOSE_CMD ps -q content)
docker cp "${CONTENT_CONTAINER}:/data/quillit-content.db.bak" "${BACKUP_DIR}/quillit-content.db"

# ── MinIO data ────────────────────────────────────────────────────────────────
info "Backing up MinIO data..."
# Get the Docker Compose project name (defaults to directory name)
PROJECT_NAME=$($COMPOSE_CMD ps --format '{{.Project}}' 2>/dev/null | head -1 || basename "$REPO_DIR" | tr '[:upper:]' '[:lower:]' | tr -cd 'a-z0-9_-')
MINIO_VOLUME="${PROJECT_NAME}_minio-data"

docker run --rm \
    -v "${MINIO_VOLUME}:/minio-src:ro" \
    -v "${BACKUP_DIR}:/out" \
    alpine \
    tar czf "/out/minio-${DATE}.tar.gz" -C /minio-src .

# ── Summary ───────────────────────────────────────────────────────────────────
BACKUP_SIZE=$(du -sh "$BACKUP_DIR" | cut -f1)
echo ""
echo -e "${GREEN}${BOLD}── Backup complete ──────────────────────────────────────────────${NC}"
echo -e "  Location: ${BOLD}${BACKUP_DIR}${NC}"
echo -e "  Size:     ${BACKUP_SIZE}"
echo ""
echo "  Files:"
ls -lh "$BACKUP_DIR"
echo ""

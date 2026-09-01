#!/usr/bin/env bash
set -euo pipefail

# ── Quillit self-hosted setup ─────────────────────────────────────────────────
# Tested on Ubuntu 22.04 LTS and Debian 12.
# Run as a non-root user with sudo privileges.

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; BOLD='\033[1m'; NC='\033[0m'

info()    { echo -e "${GREEN}[+]${NC} $*"; }
warn()    { echo -e "${YELLOW}[!]${NC} $*"; }
error()   { echo -e "${RED}[x]${NC} $*" >&2; exit 1; }
prompt()  { echo -e "${BOLD}[?]${NC} $*"; }

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# ── 1. OS check ───────────────────────────────────────────────────────────────
if ! grep -qiE 'ubuntu|debian' /etc/os-release 2>/dev/null; then
    warn "This script is written for Ubuntu/Debian. Proceeding anyway — adjust package commands if needed."
fi

# ── 2. Docker install ─────────────────────────────────────────────────────────
if command -v docker &>/dev/null && docker compose version &>/dev/null; then
    info "Docker and Compose already installed ($(docker --version | cut -d' ' -f3 | tr -d ','))"
else
    info "Installing Docker Engine + Compose plugin..."
    DOCKER_FRESH_INSTALL=1
    sudo apt-get update -qq
    sudo apt-get install -y -qq ca-certificates curl gnupg lsb-release

    # Docker only publishes apt repos for a handful of base distros (ubuntu, debian, ...).
    # Ubuntu/Debian derivatives (Pop!_OS, Mint, elementary, Zorin, ...) report their own ID
    # here, so fall back to the upstream base via ID_LIKE, preferring its own codename field
    # (some derivatives' `lsb_release -cs` returns their own codename, not the Ubuntu/Debian
    # base codename Docker's repo actually needs).
    . /etc/os-release
    DOCKER_DISTRO_ID="$ID"
    DOCKER_DISTRO_CODENAME="$(lsb_release -cs)"
    if [[ "$DOCKER_DISTRO_ID" != "ubuntu" && "$DOCKER_DISTRO_ID" != "debian" ]]; then
        if [[ "${ID_LIKE:-}" == *ubuntu* ]]; then
            DOCKER_DISTRO_ID="ubuntu"
            DOCKER_DISTRO_CODENAME="${UBUNTU_CODENAME:-$DOCKER_DISTRO_CODENAME}"
        elif [[ "${ID_LIKE:-}" == *debian* ]]; then
            DOCKER_DISTRO_ID="debian"
            DOCKER_DISTRO_CODENAME="${DEBIAN_CODENAME:-$DOCKER_DISTRO_CODENAME}"
        else
            error "Unsupported distro for automated Docker install: $ID. Install Docker manually: https://docs.docker.com/engine/install/"
        fi
        warn "Distro '$ID' isn't a Docker-supported base — using Docker's '$DOCKER_DISTRO_ID' apt repo ($DOCKER_DISTRO_CODENAME)."
    fi

    sudo install -m 0755 -d /etc/apt/keyrings
    curl -fsSL "https://download.docker.com/linux/${DOCKER_DISTRO_ID}/gpg" \
        | sudo gpg --yes --dearmor -o /etc/apt/keyrings/docker.gpg
    sudo chmod a+r /etc/apt/keyrings/docker.gpg

    echo \
        "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] \
        https://download.docker.com/linux/${DOCKER_DISTRO_ID} \
        ${DOCKER_DISTRO_CODENAME} stable" \
        | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null

    sudo apt-get update -qq
    sudo apt-get install -y -qq docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin

    info "Adding $USER to docker group..."
    sudo usermod -aG docker "$USER"
    warn "Docker group change requires a new shell session. If docker commands fail, run: newgrp docker"
fi

# ── 3. Firewall (UFW) ─────────────────────────────────────────────────────────
if command -v ufw &>/dev/null; then
    info "Configuring UFW firewall..."
    sudo ufw default deny incoming   > /dev/null
    sudo ufw default allow outgoing  > /dev/null
    sudo ufw allow ssh               > /dev/null  # port 22 — keep SSH open first
    sudo ufw allow 80/tcp            > /dev/null  # HTTP (ACME challenge + redirect)
    sudo ufw allow 443/tcp           > /dev/null  # HTTPS
    sudo ufw allow 5353/udp          > /dev/null  # mDNS (avahi hostname.local advertisement)
    # MinIO (9000/9001) binds to 127.0.0.1 only in infra/docker-compose.yml.
    # Access the console remotely via SSH tunnel:
    #   ssh -L 9001:localhost:9001 user@your-server
    # Then open http://localhost:9001 in your browser.
    sudo ufw --force enable          > /dev/null
    info "UFW enabled. Ports open: 22 (SSH), 80, 443, 5353/udp (mDNS)"
    warn "MinIO console (9001) is localhost-only — access via SSH tunnel: ssh -L 9001:localhost:9001 user@server"
else
    warn "UFW not found. Ensure your firewall allows 80 and 443 only. MinIO binds to 127.0.0.1 so no firewall rule needed."
fi

# ── 3b. mDNS (avahi) ──────────────────────────────────────────────────────────
if command -v avahi-daemon &>/dev/null; then
    info "avahi-daemon already installed"
else
    info "Installing avahi-daemon for LAN hostname discovery (<hostname>.local)..."
    sudo apt-get install -y -qq avahi-daemon
fi
sudo systemctl enable --now avahi-daemon &>/dev/null || true
MDNS_HOST="$(hostname).local"
info "Reachable on the LAN at: ${MDNS_HOST}"

# ── 4. Collect configuration ──────────────────────────────────────────────────
echo ""
echo -e "${BOLD}── Configuration ──────────────────────────────────────────────${NC}"

# Detect public/LAN IP as a default suggestion
DETECTED_IP=$(hostname -I 2>/dev/null | awk '{print $1}' || echo "")

prompt "Server IP or domain (default: ${DETECTED_IP:-localhost}):"
read -r HOST_INPUT
HOST="${HOST_INPUT:-${DETECTED_IP:-localhost}}"

prompt "Use HTTPS with Caddy? Requires a domain and ports 80/443 open. (y/N):"
read -r USE_HTTPS_INPUT
USE_HTTPS="${USE_HTTPS_INPUT:-N}"

if [[ "$USE_HTTPS" =~ ^[Yy]$ ]]; then
    CORS_ORIGIN="https://${HOST}"
    COOKIE_SECURE="true"
    CADDY_HOST="${HOST}"
    COMPOSE_FLAGS="-f infra/docker-compose.yml -f infra/docker-compose.caddy.yml --env-file .env"
    info "Caddy will serve ${HOST} (set via CADDY_HOST in .env — Caddyfile itself never needs editing)"
else
    CORS_ORIGIN="http://${HOST}:8080"
    COOKIE_SECURE="false"
    CADDY_HOST=""
    COMPOSE_FLAGS="-f infra/docker-compose.yml --env-file .env"
fi

prompt "Admin email (default: admin@quillit.local):"
read -r ADMIN_EMAIL_INPUT
ADMIN_EMAIL="${ADMIN_EMAIL_INPUT:-admin@quillit.local}"

prompt "Admin password (leave blank to auto-generate):"
read -rs ADMIN_PASSWORD_INPUT
echo ""
if [[ -z "$ADMIN_PASSWORD_INPUT" ]]; then
    ADMIN_PASSWORD=$(openssl rand -base64 16 | tr -dc 'a-zA-Z0-9' | head -c 20)
    info "Auto-generated admin password — saved to .env"
else
    ADMIN_PASSWORD="$ADMIN_PASSWORD_INPUT"
fi

# Auto-generate secrets
JWT_SECRET=$(openssl rand -hex 32)
MINIO_PASSWORD=$(openssl rand -hex 20)

# ── 5. Write .env ─────────────────────────────────────────────────────────────
ENV_FILE="${REPO_DIR}/.env"

if [[ -f "$ENV_FILE" ]]; then
    warn ".env already exists — backing up to .env.bak"
    cp "$ENV_FILE" "${ENV_FILE}.bak"
    chmod 600 "${ENV_FILE}.bak"
fi

cat > "$ENV_FILE" <<EOF
JWT_SECRET=${JWT_SECRET}
SEED_ADMIN_EMAIL=${ADMIN_EMAIL}
SEED_ADMIN_PASSWORD=${ADMIN_PASSWORD}
MINIO_USER=quillit
MINIO_PASSWORD=${MINIO_PASSWORD}
CORS_ORIGIN=${CORS_ORIGIN}
COOKIE_SECURE=${COOKIE_SECURE}
CADDY_HOST=${CADDY_HOST}
EOF

chmod 600 "$ENV_FILE"
info ".env written (mode 600)"

# usermod (section 2) only takes effect in a NEW login session. If this shell's
# group list doesn't include docker yet (i.e. Docker was just installed above),
# run docker commands via `sg docker` so this run doesn't fail with "permission
# denied" before the user ever gets a fresh shell.
run_docker() {
    # `id -nG` (no argument) reports THIS PROCESS's live group credentials, which is
    # what matters here — `id -nG "$USER"` would instead query the account's group
    # database, which already shows docker right after usermod even though this
    # shell's live credentials haven't picked it up yet.
    if id -nG | grep -qw docker; then
        docker "$@"
    else
        sg docker -c "docker $(printf '%q ' "$@")"
    fi
}

# ── 6. Build + launch ─────────────────────────────────────────────────────────
cd "$REPO_DIR"

info "Building images (this takes a few minutes on first run)..."
# shellcheck disable=SC2086
run_docker compose $COMPOSE_FLAGS build

info "Starting services..."
# shellcheck disable=SC2086
run_docker compose $COMPOSE_FLAGS up -d

# ── 7. Wait for services ──────────────────────────────────────────────────────
info "Waiting for services to be ready..."
MAX_WAIT=60
ELAPSED=0
# svc's port is not published to the host; probe its /healthz from inside the compose network.
# shellcheck disable=SC2086
until run_docker compose $COMPOSE_FLAGS exec -T svc wget -q -O /dev/null "http://localhost:3000/healthz" 2>/dev/null || [[ $ELAPSED -ge $MAX_WAIT ]]; do
    sleep 2
    ELAPSED=$((ELAPSED + 2))
done

if [[ $ELAPSED -ge $MAX_WAIT ]]; then
    warn "Services took longer than expected. Check logs: docker compose logs"
fi

# ── 8. Save compose command for future use ────────────────────────────────────
cat > "${REPO_DIR}/compose.sh" <<SCRIPT
#!/usr/bin/env bash
# Wrapper that always uses the right compose flags for this installation.
set -euo pipefail
cd "$(dirname "\$0")"
docker compose ${COMPOSE_FLAGS} "\$@"
SCRIPT
chmod +x "${REPO_DIR}/compose.sh"
chmod +x "${REPO_DIR}/backup.sh"

# ── 9. Summary ────────────────────────────────────────────────────────────────
echo ""
echo -e "${GREEN}${BOLD}── Quillit is running ─────────────────────────────────────────${NC}"
if [[ "$USE_HTTPS" =~ ^[Yy]$ ]]; then
    echo -e "  App:          ${BOLD}https://${HOST}${NC}"
else
    echo -e "  App:          ${BOLD}http://${HOST}:8080${NC}"
fi
echo -e "  LAN alias:     ${BOLD}${MDNS_HOST}${NC} (mDNS — no static IP needed)"
echo -e "  MinIO console: localhost:9001 via SSH tunnel"
echo -e "                 ssh -L 9001:localhost:9001 user@${HOST}"
echo ""
echo -e "  Admin email:    ${ADMIN_EMAIL}"
echo -e "  Admin password: ${ADMIN_PASSWORD}"
echo ""
echo -e "  Credentials also stored in ${BOLD}.env${NC}"
echo ""
echo -e "  Manage:    ${BOLD}./compose.sh up -d / down / logs${NC}"
echo -e "  Backup:    ${BOLD}./backup.sh${NC}"
echo -e "  Update:    ${BOLD}git pull && ./compose.sh build && ./compose.sh up -d${NC}"
echo ""
if [[ "${DOCKER_FRESH_INSTALL:-0}" == "1" ]]; then
    warn "Docker was just installed — log out and back in (or run 'newgrp docker') before using 'docker' or './compose.sh' directly in this terminal."
fi

#!/usr/bin/env bash
#
# Sudoku UI — One-Click Installer
# https://github.com/sudoku-ui/panel (example)
#
# Usage:
#   sudo bash -c "$(curl -fsSL https://raw.githubusercontent.com/.../install.sh)"
#
set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'
BOLD='\033[1m'

info()  { echo -e "${CYAN}[INFO]${NC} $*"; }
ok()    { echo -e "${GREEN}[OK]${NC} $*"; }
warn()  { echo -e "${YELLOW}[WARN]${NC} $*"; }
err()   { echo -e "${RED}[ERROR]${NC} $*"; exit 1; }

# ============== Config ==============
PANEL_PORT="${PANEL_PORT:-2053}"
SUDOKU_PORT="${SUDOKU_PORT:-}"          # random if empty
INSTALL_DIR="/usr/local/sudoku-ui"
DATA_DIR="/etc/sudoku-ui"
SUDOKU_BIN_DIR="/usr/local/bin"
SERVICE_NAME="sudoku-ui"
SUDOKU_SERVICE="sudoku"

# GitHub releases (замени на свой репозиторий когда опубликуешь)
PANEL_REPO="${PANEL_REPO:-sudoku-ui/panel}"
SUDOKU_REPO="SUDOKU-ASCII/sudoku"

echo ""
echo -e "${BOLD}══════════════════════════════════════════════${NC}"
echo -e "${BOLD}       Sudoku UI — One-Click Installer        ${NC}"
echo -e "${BOLD}══════════════════════════════════════════════${NC}"
echo ""

# ============== Checks ==============
[[ $EUID -eq 0 ]] || err "Запусти от root: sudo bash install.sh"

detect_arch() {
  case $(uname -m) in
    x86_64|amd64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *) err "Неподдерживаемая архитектура: $(uname -m)" ;;
  esac
  ok "Архитектура: $ARCH"
}

detect_os() {
  if [[ -f /etc/os-release ]]; then
    . /etc/os-release
    OS=$ID
  else
    OS=$(uname -s)
  fi
  ok "ОС: $OS"
}

install_deps() {
  info "Установка зависимостей..."
  if command -v apt-get &>/dev/null; then
    apt-get update -qq
    apt-get install -y -qq curl wget tar jq ca-certificates > /dev/null
  elif command -v yum &>/dev/null; then
    yum install -y -q curl wget tar jq ca-certificates
  elif command -v apk &>/dev/null; then
    apk add --quiet curl wget tar jq ca-certificates
  fi
  ok "Зависимости установлены"
}

# ============== Download Sudoku core ==============
install_sudoku_core() {
  info "Скачивание ядра Sudoku..."
  local api="https://api.github.com/repos/${SUDOKU_REPO}/releases/latest"
  local tag asset_url

  tag=$(curl -fsSL "$api" | jq -r '.tag_name')
  [[ -n "$tag" && "$tag" != "null" ]] || err "Не удалось получить релиз Sudoku"

  # Ищем linux + arch
  asset_url=$(curl -fsSL "$api" | jq -r --arg arch "$ARCH" '
    .assets[] | select(.name | test("linux"; "i")) | select(.name | test($arch; "i")) | .browser_download_url' | head -1)

  if [[ -z "$asset_url" || "$asset_url" == "null" ]]; then
    # fallback
    asset_url=$(curl -fsSL "$api" | jq -r '.assets[] | select(.name | test("linux"; "i")) | .browser_download_url' | head -1)
  fi

  [[ -n "$asset_url" && "$asset_url" != "null" ]] || err "Не найден бинарник Sudoku для linux/$ARCH"

  local tmp=$(mktemp -d)
  curl -fsSL -o "$tmp/asset" "$asset_url"

  if file "$tmp/asset" 2>/dev/null | grep -q "gzip\|tar"; then
    tar -xzf "$tmp/asset" -C "$tmp"
    find "$tmp" -type f -name "sudoku*" -executable -exec cp {} "$SUDOKU_BIN_DIR/sudoku" \; 2>/dev/null || \
    find "$tmp" -type f -name "sudoku*" -exec cp {} "$SUDOKU_BIN_DIR/sudoku" \;
  else
    cp "$tmp/asset" "$SUDOKU_BIN_DIR/sudoku"
  fi

  chmod +x "$SUDOKU_BIN_DIR/sudoku"
  rm -rf "$tmp"
  ok "Sudoku core установлен: $tag → $SUDOKU_BIN_DIR/sudoku"
}

# ============== Install Panel ==============
install_panel() {
  info "Установка панели Sudoku UI..."

  mkdir -p "$INSTALL_DIR" "$DATA_DIR" "$DATA_DIR/bin" "$DATA_DIR/sudoku"

  # Если есть локальный бинарник панели (для разработки) — копируем
  if [[ -f "./bin/panel" ]]; then
    cp "./bin/panel" "$INSTALL_DIR/panel"
  elif [[ -f "/tmp/sudoku-panel" ]]; then
    cp "/tmp/sudoku-panel" "$INSTALL_DIR/panel"
  else
    # В будущем — скачивание с GitHub Releases
    warn "Бинарник панели не найден локально. Собери его: go build -o bin/panel ./cmd/panel"
    warn "Или положи готовый бинарник в $INSTALL_DIR/panel"
    # Создаём заглушку, чтобы сервис не падал сразу
    echo '#!/bin/bash' > "$INSTALL_DIR/panel"
    echo 'echo "Panel binary missing. Build and place it here."' >> "$INSTALL_DIR/panel"
  fi

  chmod +x "$INSTALL_DIR/panel"

  # Симлинк ядра внутрь data для удобства панели
  ln -sf "$SUDOKU_BIN_DIR/sudoku" "$DATA_DIR/bin/sudoku" 2>/dev/null || true

  ok "Панель установлена в $INSTALL_DIR"
}

# ============== Generate keys & config ==============
setup_sudoku_config() {
  info "Генерация ключей и конфига Sudoku..."

  if [[ -z "$SUDOKU_PORT" ]]; then
    SUDOKU_PORT=$(shuf -i 50001-65000 -n 1)
  fi

  # Генерируем ключи
  local keygen_out
  keygen_out=$("$SUDOKU_BIN_DIR/sudoku" -keygen 2>&1) || true

  MASTER_PRIV=$(echo "$keygen_out" | grep -oP 'Master Private Key:\s*\K\S+' || true)
  MASTER_PUB=$(echo "$keygen_out" | grep -oP 'Master Public Key:\s*\K\S+' || true)

  if [[ -z "$MASTER_PUB" ]]; then
    warn "Не удалось сгенерировать ключи через бинарник, используем placeholder"
    MASTER_PUB="placeholder-public-key"
    MASTER_PRIV="placeholder-private-key"
  fi

  # Сохраняем ключи
  cat > "$DATA_DIR/sudoku/keys.json" <<EOF
{
  "master_private_key": "$MASTER_PRIV",
  "master_public_key": "$MASTER_PUB"
}
EOF
  chmod 600 "$DATA_DIR/sudoku/keys.json"

  # Конфиг сервера
  cat > "$DATA_DIR/sudoku/config.json" <<EOF
{
  "mode": "server",
  "transport": "tcp",
  "local_port": $SUDOKU_PORT,
  "server_address": "",
  "fallback_address": "127.0.0.1:80",
  "key": "$MASTER_PUB",
  "aead": "chacha20-poly1305",
  "suspicious_action": "fallback",
  "ascii": "prefer_entropy",
  "padding_min": 2,
  "padding_max": 7,
  "enable_pure_downlink": false,
  "httpmask": {
    "disable": false,
    "mode": "auto"
  }
}
EOF

  ok "Конфиг Sudoku создан (порт $SUDOKU_PORT)"
}

# ============== Systemd ==============
create_services() {
  info "Создание systemd-сервисов..."

  # Sudoku core
  cat > /etc/systemd/system/${SUDOKU_SERVICE}.service <<EOF
[Unit]
Description=Sudoku Proxy Server
After=network.target

[Service]
Type=simple
ExecStart=$SUDOKU_BIN_DIR/sudoku -c $DATA_DIR/sudoku/config.json
Restart=on-failure
RestartSec=5
LimitNOFILE=1048576

[Install]
WantedBy=multi-user.target
EOF

  # Panel
  cat > /etc/systemd/system/${SERVICE_NAME}.service <<EOF
[Unit]
Description=Sudoku UI Panel
After=network.target ${SUDOKU_SERVICE}.service

[Service]
Type=simple
Environment=SUDOKU_UI_DATA=$DATA_DIR
Environment=PANEL_PORT=$PANEL_PORT
ExecStart=$INSTALL_DIR/panel
Restart=on-failure
RestartSec=3
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
EOF

  systemctl daemon-reload
  systemctl enable ${SUDOKU_SERVICE} ${SERVICE_NAME}
  systemctl restart ${SUDOKU_SERVICE} || warn "Sudoku service failed to start"
  systemctl restart ${SERVICE_NAME} || warn "Panel service failed to start"

  ok "Сервисы созданы и запущены"
}

# ============== Firewall ==============
setup_firewall() {
  if command -v ufw &>/dev/null && ufw status | grep -q "Status: active"; then
    ufw allow ${PANEL_PORT}/tcp >/dev/null 2>&1 || true
    ufw allow ${SUDOKU_PORT}/tcp >/dev/null 2>&1 || true
    ok "UFW: открыты порты $PANEL_PORT и $SUDOKU_PORT"
  fi
}

# ============== Final output ==============
print_result() {
  local ip
  ip=$(curl -fsSL https://api.ipify.org 2>/dev/null || echo "YOUR_IP")

  echo ""
  echo -e "${GREEN}════════════════════════════════════════════════════════════${NC}"
  echo -e "${GREEN}${BOLD}  Установка завершена!${NC}"
  echo -e "${GREEN}════════════════════════════════════════════════════════════${NC}"
  echo ""
  echo -e "  Панель:     ${CYAN}http://${ip}:${PANEL_PORT}${NC}"
  echo -e "  Логин:      ${YELLOW}admin${NC}"
  echo -e "  Пароль:     ${YELLOW}admin${NC}  ${RED}(смени сразу!)${NC}"
  echo ""
  echo -e "  Sudoku порт: ${CYAN}${SUDOKU_PORT}${NC}"
  echo ""
  echo -e "  Управление:"
  echo -e "    systemctl status ${SERVICE_NAME}"
  echo -e "    systemctl status ${SUDOKU_SERVICE}"
  echo -e "    journalctl -u ${SERVICE_NAME} -f"
  echo ""
  echo -e "${GREEN}════════════════════════════════════════════════════════════${NC}"
}

# ============== Main ==============
detect_os
detect_arch
install_deps
install_sudoku_core
install_panel
setup_sudoku_config
create_services
setup_firewall
print_result

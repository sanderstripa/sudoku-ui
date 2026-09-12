#!/usr/bin/env bash
set -Eeuo pipefail
REPO="${SUDOKU_UI_REPO:-sanderstripa/sudoku-ui}"; CORE_REPO="SUDOKU-ASCII/sudoku"
BIN=/usr/local/bin/sudoku-ui; CORE_BIN=/usr/local/bin/sudoku
BLUE='\033[1;34m'; RESET='\033[0m'
echo "Select language / Выберите язык:"; echo "1 — Русский"; echo "2 — English"; read -r -p "[1/2]: " LANG_CHOICE
[[ "$LANG_CHOICE" == 2 ]] && UI_LANG=en || UI_LANG=ru
if [[ "$UI_LANG" == ru ]]; then
  ROOT_MSG="запустите установку от root"; OS_MSG="поддерживаются Ubuntu и Debian"; INSTALLING="Устанавливаем Sudoku UI…"; FOUND="Обнаружена существующая установка Sudoku."
  CHOOSE="Выберите"; SUCCESS="Sudoku UI успешно установлен"; PANEL="Панель"; LOGIN="Логин"; PASSWORD_LABEL="Пароль"
else
  ROOT_MSG="run the installer as root"; OS_MSG="Ubuntu and Debian are supported"; INSTALLING="Installing Sudoku UI…"; FOUND="An existing Sudoku installation was found."
  CHOOSE="Choose"; SUCCESS="Sudoku UI installed successfully"; PANEL="Panel"; LOGIN="Username"; PASSWORD_LABEL="Password"
fi
die(){ echo "Error: $*" >&2; exit 1; }
[[ ${EUID:-$(id -u)} -eq 0 ]] || die "$ROOT_MSG"; [[ -r /etc/os-release ]] || die "$OS_MSG"; . /etc/os-release
[[ ${ID:-} == ubuntu || ${ID:-} == debian ]] || die "$OS_MSG"
case "$(uname -m)" in x86_64|amd64) ARCH=amd64;; aarch64|arm64) ARCH=arm64;; *) die "unsupported architecture";; esac
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq; apt-get install -y -qq ca-certificates curl jq qrencode tar iproute2 nftables cron openssl >/dev/null
TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT
release_asset(){ curl -fsSL "https://api.github.com/repos/$1/releases/latest" | jq -r --arg n "$2" '.assets[]|select(.name==$n)|.browser_download_url' | head -n1; }
printf '%b%s%b\n' "$BLUE" "$INSTALLING" "$RESET"
PANEL_URL="$(release_asset "$REPO" "sudoku-ui-linux-${ARCH}")"; [[ -n "$PANEL_URL" ]] || die "Sudoku UI release asset not found"
curl -fsSL "$PANEL_URL" -o "$TMP/sudoku-ui"; chmod 755 "$TMP/sudoku-ui"; PANEL_VERSION="$("$TMP/sudoku-ui" --version)"
INSTALL_CORE=1; CLEAN_INSTALL=0
if [[ -e "$CORE_BIN" || -e /etc/sudoku || -e /etc/systemd/system/sudoku.service || -e "$BIN" ]]; then
  echo; echo "$FOUND"
  if [[ "$UI_LANG" == ru ]]; then
    echo "1 — использовать существующий Sudoku Core"; echo "2 — заменить бинарник с резервной копией"; echo "3 — заменить бинарник без резервной копии"; echo "4 — чистая переустановка с удалением данных Sudoku UI"; echo "5 — полностью удалить Sudoku UI и управляемый ею Sudoku"; echo "6 — отменить"
  else
    echo "1 — use the existing Sudoku Core"; echo "2 — replace the binary and keep a backup"; echo "3 — replace the binary without a backup"; echo "4 — clean reinstall and remove Sudoku UI data"; echo "5 — completely uninstall Sudoku UI and its managed Sudoku"; echo "6 — cancel"
  fi
  read -r -p "$CHOOSE [1/2/3/4/5/6]: " choice
  case "$choice" in
    1) INSTALL_CORE=0;;
    2) [[ -f "$CORE_BIN" ]] && cp -a "$CORE_BIN" "${CORE_BIN}.before-sudoku-ui";;
    3) :;;
    4) read -r -p "Type DEL / Введите DEL: " confirm; [[ "${confirm^^}" == DEL ]] || exit 0; CLEAN_INSTALL=1;;
    5) read -r -p "Type DEL / Введите DEL: " confirm; [[ "${confirm^^}" == DEL ]] || exit 0; systemctl disable --now sudoku-ui sudoku 2>/dev/null || true; rm -f /etc/systemd/system/sudoku-ui.service /etc/systemd/system/sudoku.service "$BIN" "$CORE_BIN"; rm -rf /etc/sudoku-ui /etc/sudoku /var/lib/sudoku-ui; nft delete table inet sudoku_ui 2>/dev/null || true; systemctl daemon-reload; echo "Sudoku UI removed / Sudoku UI удалена"; exit 0;;
    *) exit 0;;
  esac
fi
if [[ $CLEAN_INSTALL -eq 1 ]]; then
  systemctl disable --now sudoku-ui sudoku 2>/dev/null || true
  if [[ -s /etc/sudoku-ui/tls/fullchain.pem && -s /etc/sudoku-ui/tls/privkey.pem ]] && openssl x509 -checkend 3600 -noout -in /etc/sudoku-ui/tls/fullchain.pem >/dev/null 2>&1; then
    mkdir -p "$TMP/preserved-tls"; cp -a /etc/sudoku-ui/tls/fullchain.pem /etc/sudoku-ui/tls/privkey.pem "$TMP/preserved-tls/"
  fi
  rm -rf /etc/sudoku-ui /etc/sudoku /var/lib/sudoku-ui
fi
install -m 755 "$TMP/sudoku-ui" "${BIN}.new"
mv -f "${BIN}.new" "$BIN"
if [[ $INSTALL_CORE -eq 1 ]]; then
  CORE_VERSION="$(curl -fsSL "https://api.github.com/repos/${CORE_REPO}/releases/latest" | jq -r '.tag_name // empty')"
  CORE_URL="$(release_asset "$CORE_REPO" "sudoku-linux-${ARCH}.tar.gz")"; [[ -n "$CORE_URL" ]] || die "Sudoku Core release asset not found"
  curl -fsSL "$CORE_URL" -o "$TMP/sudoku.tar.gz"; tar -xzf "$TMP/sudoku.tar.gz" -C "$TMP" sudoku; chmod 755 "$TMP/sudoku"
  "$TMP/sudoku" -keygen | grep -q 'Master Public Key:' || die "Sudoku Core keygen check failed"; install -m 755 "$TMP/sudoku" "$CORE_BIN"
fi
mkdir -p /etc/sudoku-ui /etc/sudoku /var/lib/sudoku-ui /var/backups/sudoku-ui; chmod 700 /etc/sudoku-ui /etc/sudoku
[[ -n "${CORE_VERSION:-}" ]] && printf '%s\n' "$CORE_VERSION" >/etc/sudoku-ui/core-version
if [[ ! -s /etc/sudoku-ui/core-version && -x "$CORE_BIN" ]]; then
  DETECT_VERSION="$(curl -fsSL "https://api.github.com/repos/${CORE_REPO}/releases/latest" | jq -r '.tag_name // empty')"
  DETECT_URL="$(release_asset "$CORE_REPO" "sudoku-linux-${ARCH}.tar.gz")"
  if [[ -n "$DETECT_VERSION" && -n "$DETECT_URL" ]]; then
    mkdir -p "$TMP/detect"; curl -fsSL "$DETECT_URL" -o "$TMP/detect/core.tar.gz"; tar -xzf "$TMP/detect/core.tar.gz" -C "$TMP/detect" sudoku
    [[ "$(sha256sum "$CORE_BIN" | awk '{print $1}')" == "$(sha256sum "$TMP/detect/sudoku" | awk '{print $1}')" ]] && printf '%s\n' "$DETECT_VERSION" >/etc/sudoku-ui/core-version
  fi
fi
cat >/etc/systemd/system/sudoku-ui.service <<'UNIT'
[Unit]
Description=Sudoku UI
After=network-online.target
Wants=network-online.target
[Service]
Type=simple
ExecStart=/usr/local/bin/sudoku-ui
Restart=on-failure
RestartSec=2
User=root
NoNewPrivileges=true
PrivateTmp=true
ProtectHome=true
[Install]
WantedBy=multi-user.target
UNIT
cat >/etc/systemd/system/sudoku.service <<'UNIT'
[Unit]
Description=Sudoku Server
After=network-online.target
Wants=network-online.target
[Service]
Type=simple
ExecStart=/usr/local/bin/sudoku -c /etc/sudoku/config.json
Restart=on-failure
RestartSec=2
LimitNOFILE=1048576
[Install]
WantedBy=multi-user.target
UNIT
if [[ $CLEAN_INSTALL -eq 0 && -s /etc/sudoku-ui/config.json ]]; then
  systemctl daemon-reload; systemctl enable sudoku-ui >/dev/null; systemctl restart sudoku-ui
  IP="$(curl -4fsS --max-time 5 https://api.ipify.org 2>/dev/null || hostname -I | awk '{print $1}')"
  OLD_LISTEN="$(jq -r '.listen // ":2095"' /etc/sudoku-ui/config.json)"; OLD_PORT="${OLD_LISTEN##*:}"; OLD_PATH="$(jq -r '.public_path // ""' /etc/sudoku-ui/config.json)"
  printf '\n%b━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━%b\n\n' "$BLUE" "$RESET"; printf '%b%s%b\n\n' "$BLUE" "$SUCCESS" "$RESET"; printf '%b%s:%b\nhttps://%s:%s%s/\n\n' "$BLUE" "$PANEL" "$RESET" "$IP" "$OLD_PORT" "${OLD_PATH%/}"
  [[ "$UI_LANG" == ru ]] && echo "Существующие логин и пароль сохранены." || echo "Your existing username and password were preserved."
  printf '\n%b━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━%b\n' "$BLUE" "$RESET"; exit 0
fi
PORT=""; for _ in $(seq 1 200); do C=$((20000 + RANDOM % 30000)); if ! ss -lnt "sport = :$C" 2>/dev/null | grep -q LISTEN; then PORT="$C"; break; fi; done; [[ -n "$PORT" ]] || die "no free panel port"
IP="$(curl -4fsS --max-time 5 https://api.ipify.org 2>/dev/null || hostname -I | awk '{print $1}')"; [[ "$IP" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]] || die "public IPv4 not found"
if command -v ufw >/dev/null 2>&1 && ufw status 2>/dev/null | grep -q '^Status: active'; then ufw allow 80/tcp >/dev/null; ufw allow "$PORT/tcp" >/dev/null; fi
ACME=/root/.acme.sh/acme.sh; [[ -x "$ACME" ]] || curl -fsSL https://get.acme.sh | sh
CERT_DIR=/etc/sudoku-ui/tls; mkdir -p "$CERT_DIR"; "$ACME" --set-default-ca --server letsencrypt --force >/dev/null
if [[ -s "$TMP/preserved-tls/fullchain.pem" && -s "$TMP/preserved-tls/privkey.pem" ]]; then cp -a "$TMP/preserved-tls/fullchain.pem" "$CERT_DIR/fullchain.pem"; cp -a "$TMP/preserved-tls/privkey.pem" "$CERT_DIR/privkey.pem"; fi
if ! openssl x509 -checkend 3600 -noout -in "$CERT_DIR/fullchain.pem" >/dev/null 2>&1; then
  for ACME_DOMAIN_DIR in /root/.acme.sh/"$IP" /root/.acme.sh/"${IP}_ecc" /root/.acme.sh/"$IP"_*; do
    [[ -d "$ACME_DOMAIN_DIR" ]] || continue
    ACME_CERT=""; ACME_KEY=""
    for candidate in "$ACME_DOMAIN_DIR/fullchain.cer" "$ACME_DOMAIN_DIR/${IP}.cer"; do [[ -s "$candidate" ]] && ACME_CERT="$candidate" && break; done
    for candidate in "$ACME_DOMAIN_DIR/${IP}.key" "$ACME_DOMAIN_DIR/domain.key"; do [[ -s "$candidate" ]] && ACME_KEY="$candidate" && break; done
    if [[ -n "$ACME_CERT" && -n "$ACME_KEY" ]] && openssl x509 -checkend 3600 -noout -in "$ACME_CERT" >/dev/null 2>&1; then cp -a "$ACME_CERT" "$CERT_DIR/fullchain.pem"; cp -a "$ACME_KEY" "$CERT_DIR/privkey.pem"; break; fi
  done
fi
if ! openssl x509 -checkend 3600 -noout -in "$CERT_DIR/fullchain.pem" >/dev/null 2>&1; then
  "$ACME" --installcert --force -d "$IP" --key-file "$CERT_DIR/privkey.pem" --fullchain-file "$CERT_DIR/fullchain.pem" --reloadcmd "systemctl restart sudoku-ui 2>/dev/null || true" >/dev/null 2>&1 || true
fi
if ! openssl x509 -checkend 3600 -noout -in "$CERT_DIR/fullchain.pem" >/dev/null 2>&1; then
  ss -lnt 'sport = :80' 2>/dev/null | grep -q LISTEN && die "port 80 is busy; it is required briefly to issue a new IP HTTPS certificate"
  "$ACME" --issue -d "$IP" --standalone --server letsencrypt --certificate-profile shortlived --days 6 --httpport 80 --force || die "HTTPS certificate could not be issued. If the message says rateLimited, wait until the retry-after time shown by Let's Encrypt and run this installer again"
  "$ACME" --installcert --force -d "$IP" --key-file "$CERT_DIR/privkey.pem" --fullchain-file "$CERT_DIR/fullchain.pem" --reloadcmd "systemctl restart sudoku-ui 2>/dev/null || true" >/dev/null
fi
chmod 600 "$CERT_DIR/privkey.pem"; chmod 644 "$CERT_DIR/fullchain.pem"; "$ACME" --upgrade --auto-upgrade >/dev/null 2>&1 || true
USERNAME="admin-$(od -An -N3 -tx1 /dev/urandom | tr -d ' \n')"; PASSWORD="$(od -An -N12 -tx1 /dev/urandom | tr -d ' \n')"; PUBLIC_PATH="$(od -An -N4 -tx1 /dev/urandom | tr -d ' \n')"; PANEL_SUFFIX="/${PUBLIC_PATH}/"
"$BIN" --init --username "$USERNAME" --password "$PASSWORD" --listen ":$PORT" --repo "$REPO" --path "$PUBLIC_PATH" --cert "$CERT_DIR/fullchain.pem" --key "$CERT_DIR/privkey.pem"
systemctl daemon-reload; systemctl enable sudoku-ui >/dev/null; systemctl restart sudoku-ui
PANEL_READY=0; for _ in $(seq 1 30); do if curl -fsS --max-time 3 --resolve "${IP}:${PORT}:127.0.0.1" "https://${IP}:${PORT}${PANEL_SUFFIX}" >/dev/null 2>&1; then PANEL_READY=1; break; fi; sleep 1; done
[[ $PANEL_READY -eq 1 ]] || { journalctl -u sudoku-ui.service --no-pager -n 30 >&2 || true; die "panel did not start"; }
printf '\n%b━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━%b\n\n' "$BLUE" "$RESET"
printf '%b%s%b\n\n' "$BLUE" "$SUCCESS" "$RESET"; printf '%b%s:%b\nhttps://%s:%s%s\n\n' "$BLUE" "$PANEL" "$RESET" "$IP" "$PORT" "$PANEL_SUFFIX"; printf '%b%s:%b\n%s\n\n' "$BLUE" "$LOGIN" "$RESET" "$USERNAME"; printf '%b%s:%b\n%s\n' "$BLUE" "$PASSWORD_LABEL" "$RESET" "$PASSWORD"; printf '\n%b━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━%b\n' "$BLUE" "$RESET"

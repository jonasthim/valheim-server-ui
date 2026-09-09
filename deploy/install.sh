#!/usr/bin/env bash
# valheim-server-ui installer for Debian 12+ / Ubuntu 22.04+ on x86_64.
#
#   sudo ./install.sh [--binary PATH | --version vX.Y.Z] [--listen 127.0.0.1:8080]
#   sudo ./install.sh --uninstall
#
# Idempotent: re-running upgrades the binary and unit files; data in
# /var/lib/valheim and /etc/valheim-ui/config.yaml are never overwritten.
set -euo pipefail

REPO="jonasthim/valheim-server-ui"
DATA_DIR="/var/lib/valheim"
CONF_DIR="/etc/valheim-ui"
BIN="/usr/local/bin/valheim-ui"
LIB_DIR="/usr/local/lib/valheim-ui"
STEAMCMD_URL="https://steamcdn-a.akamaihd.net/client/installer/steamcmd_linux.tar.gz"

BINARY=""
VERSION="latest"
LISTEN="127.0.0.1:8080"
UNINSTALL=0
SKIP_DEPS=0

log() { printf '\033[1;32m==>\033[0m %s\n' "$*"; }
die() { printf '\033[1;31merror:\033[0m %s\n' "$*" >&2; exit 1; }

while [[ $# -gt 0 ]]; do
  case "$1" in
    --binary) BINARY="$2"; shift 2 ;;
    --version) VERSION="$2"; shift 2 ;;
    --listen) LISTEN="$2"; shift 2 ;;
    --uninstall) UNINSTALL=1; shift ;;
    --skip-deps) SKIP_DEPS=1; shift ;;
    -h|--help) sed -n '2,10p' "$0"; exit 0 ;;
    *) die "unknown flag $1" ;;
  esac
done

[[ $EUID -eq 0 ]] || die "run as root"
[[ "$(uname -m)" == "x86_64" ]] || die "only x86_64 is supported (Valheim server is x86_64 only)"
command -v systemctl >/dev/null || die "systemd is required"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if [[ $UNINSTALL -eq 1 ]]; then
  log "Stopping services"
  systemctl disable --now valheim-ui.service 2>/dev/null || true
  for u in $(systemctl list-units --type=service --all --plain --no-legend 'valheim@*' | awk '{print $1}'); do
    systemctl disable --now "$u" 2>/dev/null || true
  done
  rm -f /etc/systemd/system/valheim-ui.service /etc/systemd/system/valheim@.service
  rm -f /etc/sudoers.d/valheim-ui "$BIN"
  rm -rf "$LIB_DIR"
  systemctl daemon-reload
  log "Removed units, sudoers and binary. Data kept in $DATA_DIR and $CONF_DIR."
  exit 0
fi

if [[ $SKIP_DEPS -eq 0 ]]; then
  if command -v apt-get >/dev/null; then
    log "Installing runtime dependencies (apt)"
    export DEBIAN_FRONTEND=noninteractive
    dpkg --add-architecture i386 >/dev/null 2>&1 || true
    apt-get update -qq
    apt-get install -y -qq lib32gcc-s1 lib32stdc++6 libsdl2-2.0-0 libpulse0 libatomic1 \
      ca-certificates curl tar unzip sudo >/dev/null
  else
    echo "warning: not an apt system; install SteamCMD deps (lib32gcc, lib32stdc++, libsdl2, libpulse, libatomic) yourself" >&2
  fi
fi

log "Creating user and directories"
if ! id valheim >/dev/null 2>&1; then
  useradd --system --home-dir "$DATA_DIR" --shell /usr/sbin/nologin --user-group valheim
fi
install -d -o valheim -g valheim -m 0750 "$DATA_DIR" "$DATA_DIR/instances" "$DATA_DIR/jobs" \
  "$DATA_DIR/cache" "$DATA_DIR/steamcmd"
install -d -m 0755 "$CONF_DIR" "$LIB_DIR"

log "Installing binary"
if [[ -n "$BINARY" ]]; then
  install -m 0755 "$BINARY" "$BIN"
else
  tmp="$(mktemp -d)"
  if [[ "$VERSION" == "latest" ]]; then
    url="https://github.com/$REPO/releases/latest/download/valheim-ui_linux_amd64.tar.gz"
  else
    url="https://github.com/$REPO/releases/download/$VERSION/valheim-ui_linux_amd64.tar.gz"
  fi
  curl -fsSL "$url" -o "$tmp/valheim-ui.tar.gz" || die "download failed: $url (use --binary PATH for a local build)"
  tar -xzf "$tmp/valheim-ui.tar.gz" -C "$tmp"
  install -m 0755 "$tmp/valheim-ui" "$BIN"
  rm -rf "$tmp"
fi

log "Installing unitctl, sudoers and systemd units"
install -m 0755 -o root -g root "$SCRIPT_DIR/unitctl" "$LIB_DIR/unitctl"
install -m 0440 -o root -g root "$SCRIPT_DIR/sudoers.d/valheim-ui" /etc/sudoers.d/valheim-ui
visudo -cf /etc/sudoers.d/valheim-ui >/dev/null || { rm -f /etc/sudoers.d/valheim-ui; die "sudoers validation failed"; }
install -m 0644 "$SCRIPT_DIR/valheim@.service" /etc/systemd/system/valheim@.service
install -m 0644 "$SCRIPT_DIR/valheim-ui.service" /etc/systemd/system/valheim-ui.service

if [[ ! -f "$CONF_DIR/config.yaml" ]]; then
  log "Writing $CONF_DIR/config.yaml"
  sed "s|^listen: .*|listen: \"$LISTEN\"|" "$SCRIPT_DIR/config.example.yaml" > "$CONF_DIR/config.yaml"
  chown root:valheim "$CONF_DIR/config.yaml"; chmod 0640 "$CONF_DIR/config.yaml"
fi

if [[ ! -x "$DATA_DIR/steamcmd/steamcmd.sh" ]]; then
  log "Installing SteamCMD"
  curl -fsSL "$STEAMCMD_URL" | tar -xz -C "$DATA_DIR/steamcmd"
  chown -R valheim:valheim "$DATA_DIR/steamcmd"
  log "Running SteamCMD self-update (this takes a minute)"
  sudo -u valheim -H "$DATA_DIR/steamcmd/steamcmd.sh" +quit >/dev/null 2>&1 || echo "warning: steamcmd self-update reported an error; it usually works on the next run" >&2
fi

log "Starting valheim-ui"
systemctl daemon-reload
systemctl enable --now valheim-ui.service
sleep 1
systemctl --no-pager --lines=5 status valheim-ui.service || true

cat <<MSG

Installed. Open http://$LISTEN/ (put a TLS reverse proxy in front for remote access).
The first visit creates the admin account. Config: $CONF_DIR/config.yaml  Data: $DATA_DIR
Logs: journalctl -u valheim-ui -f
MSG

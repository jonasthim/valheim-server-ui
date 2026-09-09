#!/usr/bin/env bash
# valheim-server-ui installer for Debian 12+ / Ubuntu 22.04+ on x86_64.
#
# One-command install (nothing else to download by hand):
#
#   curl -fsSL https://raw.githubusercontent.com/jonasthim/valheim-server-ui/main/deploy/install.sh | sudo bash
#
# Or download this file first and run it locally:
#
#   sudo ./install.sh [--version vX.Y.Z | --binary PATH] [--listen ADDR] [--skip-deps]
#   sudo ./install.sh --check        # report installed vs latest version and planned changes; makes no changes
#   sudo ./install.sh --uninstall    # remove units, sudoers and binary; keeps /var/lib/valheim
#
# This script is fully self-contained: deploy/unitctl, deploy/sudoers.d/valheim-ui,
# deploy/valheim-ui.service, deploy/valheim@.service and deploy/config.example.yaml
# are embedded below as heredocs, so the one-liner above needs nothing else.
#
# GENERATED FILE. This exact file (deploy/install.sh) is produced from
# deploy/install.sh.in by deploy/build-installer.sh (`make deploy-sync`). Edit
# deploy/install.sh.in and the deploy/* source files it includes, not this file
# directly -- your edits would be overwritten on the next `make deploy-sync`.
#
# Idempotent: re-running upgrades the binary and unit files; data in
# /var/lib/valheim and /etc/valheim-ui/config.yaml are never overwritten.
#
# Layout (kept stable for in-app self-upgrade, see RUNBOOK.md #11):
#   /var/lib/valheim/bin/valheim-ui   real binary, owned valheim:valheim 0755
#   /usr/local/bin/valheim-ui         symlink -> the real binary
#   /var/lib/valheim/bin/valheim-ui.prev   previous binary, kept for rollback
set -euo pipefail

REPO="jonasthim/valheim-server-ui"
DATA_DIR="/var/lib/valheim"
CONF_DIR="/etc/valheim-ui"
BIN_DIR="$DATA_DIR/bin"
REAL_BIN="$BIN_DIR/valheim-ui"
SYMLINK="/usr/local/bin/valheim-ui"
LIB_DIR="/usr/local/lib/valheim-ui"
STEAMCMD_URL="https://steamcdn-a.akamaihd.net/client/installer/steamcmd_linux.tar.gz"

BINARY=""
VERSION="latest"
LISTEN="127.0.0.1:8080"
UNINSTALL=0
SKIP_DEPS=0
CHECK=0

log() { printf '\033[1;32m==>\033[0m %s\n' "$*"; }
die() { printf '\033[1;31merror:\033[0m %s\n' "$*" >&2; exit 1; }

usage() {
  cat <<'USAGE'
valheim-server-ui installer

Usage:
  sudo ./install.sh [options]
  curl -fsSL https://raw.githubusercontent.com/jonasthim/valheim-server-ui/main/deploy/install.sh | sudo bash

Options:
  --version vX.Y.Z   Install this release tag instead of the latest one.
  --binary PATH      Install a locally built binary instead of downloading a
                      release (skips download and checksum verification).
  --listen ADDR      Address the manager listens on (default 127.0.0.1:8080).
                      Only applied the first time config.yaml is written.
  --skip-deps        Skip installing apt runtime dependencies.
  --uninstall        Remove units, sudoers and the binary. Keeps /var/lib/valheim.
  --check            Print installed vs latest/target version and what would
                      change. Makes no changes.
  -h, --help         Show this help.
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --binary) BINARY="$2"; shift 2 ;;
    --version) VERSION="$2"; shift 2 ;;
    --listen) LISTEN="$2"; shift 2 ;;
    --uninstall) UNINSTALL=1; shift ;;
    --skip-deps) SKIP_DEPS=1; shift ;;
    --check) CHECK=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) die "unknown flag $1 (see --help)" ;;
  esac
done

[[ $EUID -eq 0 ]] || die "run as root, e.g.: curl -fsSL .../install.sh | sudo bash"
[[ "$(uname -m)" == "x86_64" ]] || die "only x86_64 is supported (Valheim server is x86_64 only)"
command -v systemctl >/dev/null 2>&1 || die "systemd is required (systemctl not found)"

# ---- embedded deploy/ file contents -----------------------------------------
# The heredoc bodies below are generated verbatim from the standalone files in
# deploy/ by deploy/build-installer.sh. Do not hand-edit them; edit the source
# file and run `make deploy-sync`, which also fails in CI on drift.

unitctl_content() {
  cat <<'UNITCTL_EOF'
#!/bin/bash
# /usr/local/lib/valheim-ui/unitctl — the only command the valheim user may sudo.
# Usage: unitctl <start|stop|restart|enable|disable> <instance-id>
# The instance id is validated against the same pattern the manager uses
# (domain.InstanceIDPattern) so the unit name can never escape valheim@*.service.
set -euo pipefail

if [[ $# -ne 2 ]]; then
  echo "usage: unitctl <start|stop|restart|enable|disable> <instance-id>" >&2
  exit 2
fi

action="$1"
inst="$2"

if ! [[ "$inst" =~ ^[a-z0-9][a-z0-9-]{0,31}$ ]]; then
  echo "unitctl: invalid instance id" >&2
  exit 2
fi

unit="valheim@${inst}.service"

case "$action" in
  start|stop|restart)
    exec /bin/systemctl "$action" "$unit"
    ;;
  enable|disable)
    exec /bin/systemctl "$action" "$unit"
    ;;
  *)
    echo "unitctl: action not allowed: $action" >&2
    exit 2
    ;;
esac
UNITCTL_EOF
}

sudoers_content() {
  cat <<'SUDOERS_EOF'
# Installed by deploy/install.sh to /etc/sudoers.d/valheim-ui (mode 0440).
# Grants the manager exactly one root command; unitctl validates its arguments.
Defaults:valheim !requiretty
valheim ALL=(root) NOPASSWD: /usr/local/lib/valheim-ui/unitctl
SUDOERS_EOF
}

manager_unit_content() {
  cat <<'MANAGER_UNIT_EOF'
# Installed by deploy/install.sh to /etc/systemd/system/valheim-ui.service
[Unit]
Description=Valheim Server UI (manager)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=valheim
Group=valheim
Environment=VALHEIM_UI_CONFIG=/etc/valheim-ui/config.yaml
# HOME must live inside ReadWritePaths: SteamCMD writes ~/Steam and Unity writes
# ~/.config/unity3d, and ProtectHome=true denies /home even if the valheim
# account's passwd home points there (e.g. a pre-existing login user).
Environment=HOME=/var/lib/valheim
ExecStart=/usr/local/bin/valheim-ui serve
# Restart=always + a short RestartSec so a clean exit(0) after a self-upgrade
# (download + checksum + atomic binary swap, see RUNBOOK.md #11) restarts the
# manager on the new binary automatically. Game instances are supervised by
# their own independent valheim@<id>.service units (see valheim@.service) and
# are never affected by the manager restarting.
Restart=always
RestartSec=3
WorkingDirectory=/var/lib/valheim
# Hardening. NoNewPrivileges must stay off: the manager calls sudo for unitctl.
ProtectSystem=strict
ReadWritePaths=/var/lib/valheim
PrivateTmp=true
ProtectHome=true
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
MANAGER_UNIT_EOF
}

instance_unit_content() {
  cat <<'INSTANCE_UNIT_EOF'
# Installed by deploy/install.sh to /etc/systemd/system/valheim@.service
# One unit per instance: valheim@<id>.service. Never edited by the manager;
# per-instance settings live in /var/lib/valheim/instances/<id>/launch.json.
# This unit is fully independent of valheim-ui.service: it is not "wanted by"
# or ordered against the manager unit in any way, so a manager restart or
# self-upgrade (which relies on valheim-ui.service's Restart=always) never
# stops, restarts or otherwise touches running game instances.
[Unit]
Description=Valheim dedicated server (%i)
After=network-online.target
Wants=network-online.target
StartLimitIntervalSec=300
StartLimitBurst=5

[Service]
Type=simple
User=valheim
Group=valheim
WorkingDirectory=/var/lib/valheim/instances/%i/server
Environment=VALHEIM_UI_CONFIG=/etc/valheim-ui/config.yaml
# HOME must live inside ReadWritePaths: SteamCMD writes ~/Steam and Unity writes
# ~/.config/unity3d, and ProtectHome=true denies /home even if the valheim
# account's passwd home points there (e.g. a pre-existing login user).
Environment=HOME=/var/lib/valheim
ExecStart=/usr/local/bin/valheim-ui launch --instance %i
StandardOutput=append:/var/lib/valheim/instances/%i/logs/console.log
StandardError=inherit
KillSignal=SIGINT
KillMode=mixed
TimeoutStopSec=120
Restart=on-failure
RestartSec=10
LimitNOFILE=100000
Nice=-5

[Install]
WantedBy=multi-user.target
INSTANCE_UNIT_EOF
}

config_example_content() {
  cat <<'CONFIG_EOF'
# /etc/valheim-ui/config.yaml — manager configuration.
# Every key can be overridden with VALHEIM_UI_<UPPERCASE_KEY>.

# Address the web UI listens on. Keep it on loopback and put a TLS reverse proxy
# (Caddy, nginx, Traefik) in front for remote access.
listen: "127.0.0.1:8080"

# Public URL of the UI. Required for OIDC (redirect URI) and used for the CSRF
# origin check. Example: https://valheim.example.com
base_url: ""

data_dir: "/var/lib/valheim"

# systemd (production) or direct (development; instances die with the manager)
supervisor: "systemd"
unitctl_path: "/usr/local/lib/valheim-ui/unitctl"
steamcmd_path: "/var/lib/valheim/steamcmd/steamcmd.sh"

# Set true only when serving plain http on a trusted LAN.
insecure_cookies: false

log_level: "info"
CONFIG_EOF
}
# ---- end embedded content ---------------------------------------------------

resolve_latest_tag() {
  local loc tag
  loc="$(curl -fsSL -o /dev/null -w '%{url_effective}' "https://github.com/$REPO/releases/latest" 2>/dev/null)" || loc=""
  if [[ "$loc" =~ /releases/tag/(.+)$ ]]; then
    printf '%s\n' "${BASH_REMATCH[1]}"
    return 0
  fi
  tag="$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" 2>/dev/null \
    | grep -m1 '"tag_name"' | sed -E 's/.*"tag_name":[[:space:]]*"([^"]+)".*/\1/')" || tag=""
  if [[ -n "$tag" ]]; then
    printf '%s\n' "$tag"
    return 0
  fi
  return 1
}

installed_version() {
  if [[ -x "$SYMLINK" ]]; then
    "$SYMLINK" version 2>/dev/null || echo "unknown"
  elif [[ -x "$REAL_BIN" ]]; then
    "$REAL_BIN" version 2>/dev/null || echo "unknown"
  else
    echo "(not installed)"
  fi
}

# Install a new valheim-ui binary at $src into the real-binary/symlink layout,
# keeping the previous binary as valheim-ui.prev for rollback (see
# `valheim-ui self-upgrade --rollback`, RUNBOOK.md #11). The swap is a rename
# within $BIN_DIR, so it is atomic from the point of view of anything that has
# the file open or execs through the symlink.
install_binary_files() {
  local src="$1"
  if [[ -f "$REAL_BIN" ]]; then
    cp -f "$REAL_BIN" "$REAL_BIN.prev"
  fi
  install -o valheim -g valheim -m 0755 "$src" "$REAL_BIN.new"
  mv -f "$REAL_BIN.new" "$REAL_BIN"
  ln -sfn "$REAL_BIN" "$SYMLINK"
}

download_and_install_binary() {
  local tag="$1" base tarball tmp
  base="https://github.com/$REPO/releases/download/$tag"
  tarball="valheim-ui_linux_amd64.tar.gz"
  tmp="$(mktemp -d)"
  log "Downloading $tarball ($tag)"
  curl -fsSL "$base/$tarball" -o "$tmp/$tarball" \
    || { rm -rf "$tmp"; die "download failed: $base/$tarball (use --binary PATH for a local build)"; }
  curl -fsSL "$base/SHA256SUMS" -o "$tmp/SHA256SUMS" \
    || { rm -rf "$tmp"; die "download failed: $base/SHA256SUMS (use --binary PATH for a local build)"; }
  log "Verifying checksum"
  ( cd "$tmp" && grep -E " \*?${tarball//./\\.}\$" SHA256SUMS | sha256sum -c - ) >/dev/null \
    || { rm -rf "$tmp"; die "checksum verification failed for $tarball"; }
  tar -xzf "$tmp/$tarball" -C "$tmp"
  [[ -f "$tmp/valheim-ui" ]] || { rm -rf "$tmp"; die "$tarball did not contain a valheim-ui binary"; }
  install_binary_files "$tmp/valheim-ui"
  rm -rf "$tmp"
}

# ---- uninstall ---------------------------------------------------------------
do_uninstall() {
  if [[ $CHECK -eq 1 ]]; then
    echo "Would stop and disable valheim-ui.service and any valheim@*.service units"
    echo "Would remove: /etc/systemd/system/valheim-ui.service /etc/systemd/system/valheim@.service"
    echo "Would remove: /etc/sudoers.d/valheim-ui $LIB_DIR $SYMLINK $BIN_DIR"
    echo "Would keep:   $DATA_DIR $CONF_DIR"
    return 0
  fi
  log "Stopping services"
  systemctl disable --now valheim-ui.service 2>/dev/null || true
  local u
  for u in $(systemctl list-units --type=service --all --plain --no-legend 'valheim@*' 2>/dev/null | awk '{print $1}'); do
    systemctl disable --now "$u" 2>/dev/null || true
  done
  rm -f /etc/systemd/system/valheim-ui.service /etc/systemd/system/valheim@.service
  rm -f /etc/sudoers.d/valheim-ui "$SYMLINK"
  rm -rf "$LIB_DIR" "$BIN_DIR"
  systemctl daemon-reload
  log "Removed units, sudoers and binary. Data kept in $DATA_DIR and $CONF_DIR."
}

if [[ $UNINSTALL -eq 1 ]]; then
  do_uninstall
  exit 0
fi

# ---- check mode ----------------------------------------------------------------
if [[ $CHECK -eq 1 ]]; then
  echo "valheim-server-ui installer -- check mode, no changes will be made"
  echo
  echo "Installed version: $(installed_version)"
  if [[ -n "$BINARY" ]]; then
    echo "Target: local binary $BINARY"
  else
    tag="$VERSION"
    if [[ "$VERSION" == "latest" ]]; then
      tag="$(resolve_latest_tag)" || die "could not resolve the latest release (network?); pass --version vX.Y.Z or --binary PATH"
    fi
    echo "Target version: $tag"
  fi
  echo
  echo "Planned changes:"
  if [[ $SKIP_DEPS -eq 1 ]]; then
    echo "  - runtime deps: skipped (--skip-deps)"
  else
    echo "  - runtime deps: apt install (idempotent)"
  fi
  if id valheim >/dev/null 2>&1; then
    cur_home="$(getent passwd valheim | cut -d: -f6)"
    if [[ "$cur_home" == "$DATA_DIR" ]]; then
      echo "  - system user 'valheim': present (home $cur_home)"
    else
      echo "  - system user 'valheim': present with home $cur_home; would be changed to $DATA_DIR (units set HOME=$DATA_DIR and deny /home)"
    fi
  else
    echo "  - system user 'valheim': would be created"
  fi
  if [[ -d "$DATA_DIR" ]]; then
    echo "  - $DATA_DIR: present"
  else
    echo "  - $DATA_DIR: would be created"
  fi
  if diff -q <(unitctl_content) "$LIB_DIR/unitctl" >/dev/null 2>&1; then
    echo "  - unitctl: up to date"
  else
    echo "  - unitctl: would install/update"
  fi
  if diff -q <(sudoers_content) /etc/sudoers.d/valheim-ui >/dev/null 2>&1; then
    echo "  - sudoers drop-in: up to date"
  else
    echo "  - sudoers drop-in: would install/update"
  fi
  if diff -q <(manager_unit_content) /etc/systemd/system/valheim-ui.service >/dev/null 2>&1; then
    echo "  - valheim-ui.service: up to date"
  else
    echo "  - valheim-ui.service: would install/update"
  fi
  if diff -q <(instance_unit_content) /etc/systemd/system/valheim@.service >/dev/null 2>&1; then
    echo "  - valheim@.service: up to date"
  else
    echo "  - valheim@.service: would install/update"
  fi
  if [[ -f "$CONF_DIR/config.yaml" ]]; then
    echo "  - $CONF_DIR/config.yaml: present, left untouched"
  else
    echo "  - $CONF_DIR/config.yaml: would be created (listen=$LISTEN)"
  fi
  if [[ -x "$DATA_DIR/steamcmd/steamcmd.sh" ]]; then
    echo "  - SteamCMD: present"
  else
    echo "  - SteamCMD: would be downloaded and self-updated"
  fi
  if systemctl is-enabled --quiet valheim-ui.service 2>/dev/null; then
    echo "  - valheim-ui.service: enabled"
  else
    echo "  - valheim-ui.service: would be enabled and started"
  fi
  exit 0
fi

# ---- install / upgrade ----------------------------------------------------------

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
else
  cur_home="$(getent passwd valheim | cut -d: -f6)"
  if [[ "$cur_home" != "$DATA_DIR" ]]; then
    echo "warning: user 'valheim' already exists with home $cur_home; setting it to $DATA_DIR (the units run with ProtectHome=true and HOME=$DATA_DIR). Files under $cur_home are left in place." >&2
    usermod -d "$DATA_DIR" valheim
  fi
fi
install -d -o valheim -g valheim -m 0750 "$DATA_DIR" "$DATA_DIR/instances" "$DATA_DIR/jobs" \
  "$DATA_DIR/cache" "$DATA_DIR/steamcmd"
install -d -o valheim -g valheim -m 0755 "$BIN_DIR"
install -d -m 0755 "$CONF_DIR" "$LIB_DIR"

log "Installing binary"
if [[ -n "$BINARY" ]]; then
  [[ -f "$BINARY" ]] || die "--binary $BINARY not found"
  log "Installing local binary $BINARY"
  install_binary_files "$BINARY"
else
  tag="$VERSION"
  if [[ "$VERSION" == "latest" ]]; then
    tag="$(resolve_latest_tag)" || die "could not resolve the latest release (network?); pass --version vX.Y.Z or --binary PATH for a local build"
  fi
  download_and_install_binary "$tag"
fi

log "Installing unitctl, sudoers and systemd units"
unitctl_content > "$LIB_DIR/unitctl"
chown root:root "$LIB_DIR/unitctl"
chmod 0755 "$LIB_DIR/unitctl"
sudoers_content > /etc/sudoers.d/valheim-ui.new
chown root:root /etc/sudoers.d/valheim-ui.new
chmod 0440 /etc/sudoers.d/valheim-ui.new
visudo -cf /etc/sudoers.d/valheim-ui.new >/dev/null || { rm -f /etc/sudoers.d/valheim-ui.new; die "sudoers validation failed"; }
mv -f /etc/sudoers.d/valheim-ui.new /etc/sudoers.d/valheim-ui
manager_unit_content > /etc/systemd/system/valheim-ui.service
instance_unit_content > /etc/systemd/system/valheim@.service
chmod 0644 /etc/systemd/system/valheim-ui.service /etc/systemd/system/valheim@.service

if [[ ! -f "$CONF_DIR/config.yaml" ]]; then
  log "Writing $CONF_DIR/config.yaml"
  config_example_content | sed "s|^listen: .*|listen: \"$LISTEN\"|" > "$CONF_DIR/config.yaml"
  chown root:valheim "$CONF_DIR/config.yaml"
  chmod 0640 "$CONF_DIR/config.yaml"
fi

if [[ ! -x "$DATA_DIR/steamcmd/steamcmd.sh" ]]; then
  log "Installing SteamCMD"
  curl -fsSL "$STEAMCMD_URL" | tar -xz -C "$DATA_DIR/steamcmd"
  chown -R valheim:valheim "$DATA_DIR/steamcmd"
  log "Running SteamCMD self-update (this takes a minute)"
  sudo -u valheim -H "$DATA_DIR/steamcmd/steamcmd.sh" +quit >/dev/null 2>&1 \
    || echo "warning: steamcmd self-update reported an error; it usually works on the next run" >&2
fi

log "Starting valheim-ui"
systemctl daemon-reload
systemctl enable --now valheim-ui.service
sleep 1
systemctl --no-pager --lines=5 status valheim-ui.service || true

cat <<MSG

Installed. Open http://$LISTEN/ (put a TLS reverse proxy in front for remote access).
The first visit creates the admin account. Config: $CONF_DIR/config.yaml  Data: $DATA_DIR
Binary: $REAL_BIN (symlinked from $SYMLINK)
Logs: journalctl -u valheim-ui -f

Upgrade later by re-running this installer, from the UI (Settings -> Application ->
Check for updates), or on the host with 'valheim-ui self-upgrade --apply'
(rollback: 'valheim-ui self-upgrade --rollback'). See RUNBOOK.md #11.
MSG

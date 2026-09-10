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
#   /var/lib/valheim/bin/valheim-ui        real binary, owned root:root 0755
#   /var/lib/valheim/bin/valheim-ui.prev   previous binary, kept for rollback
#   /var/lib/valheim/staging/              valheim-owned; verified downloads wait
#                                          here for `unitctl apply-upgrade`
#   /usr/local/bin/valheim-ui              symlink -> the real binary
# The manager never writes to bin/ itself: it stages a release and asks the
# root-side sudo wrapper to install it after re-verifying the checksum.
set -euo pipefail

REPO="jonasthim/valheim-server-ui"
DATA_DIR="/var/lib/valheim"
CONF_DIR="/etc/valheim-ui"
BIN_DIR="$DATA_DIR/bin"
STAGING_DIR="$DATA_DIR/staging"
REAL_BIN="$BIN_DIR/valheim-ui"
SYMLINK="/usr/local/bin/valheim-ui"
LIB_DIR="/usr/local/lib/valheim-ui"
STEAMCMD_URL="https://steamcdn-a.akamaihd.net/client/installer/steamcmd_linux.tar.gz"

BINARY=""
VERSION="latest"
LISTEN="127.0.0.1:8080"
BASE_URL=""
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
  --base-url URL     Public URL of the UI (https://valheim.example.com). Needed
                      for single sign-on and the Origin check on state-changing
                      requests. Only applied the first time config.yaml is written.
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
    --base-url) BASE_URL="$2"; shift 2 ;;
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
#
#   unitctl <start|stop|restart|enable|disable> <instance-id>
#   unitctl apply-upgrade <release-tag>     install a staged, verified binary
#   unitctl rollback-upgrade                restore the previous binary
#   unitctl capabilities                    list the verbs this build supports
#
# The instance id is validated against the same pattern the manager uses
# (domain.InstanceIDPattern) so the unit name can never escape valheim@*.service.
#
# apply-upgrade is what keeps the manager binary root-owned while still
# letting the manager upgrade itself: the manager downloads and verifies a
# release as the valheim user into /var/lib/valheim/staging, and this wrapper
# checks the staged file against the SHA256SUMS published with that release
# before copying it into the root-owned bin directory. A local process running
# as valheim therefore cannot install an arbitrary binary.
set -euo pipefail

BIN_DIR=/var/lib/valheim/bin
STAGING_DIR=/var/lib/valheim/staging
REPO=jonasthim/valheim-server-ui
SERVICE_USER=valheim

usage() {
  echo "usage: unitctl <start|stop|restart|enable|disable> <instance-id>" >&2
  echo "       unitctl apply-upgrade <release-tag> | rollback-upgrade | capabilities" >&2
  exit 2
}

[[ $# -ge 1 ]] || usage
action="$1"

case "$action" in
  start|stop|restart|enable|disable)
    [[ $# -eq 2 ]] || usage
    inst="$2"
    if ! [[ "$inst" =~ ^[a-z0-9][a-z0-9-]{0,31}$ ]]; then
      echo "unitctl: invalid instance id" >&2
      exit 2
    fi
    exec /usr/bin/systemctl "$action" "valheim@${inst}.service"
    ;;
  capabilities)
    [[ $# -eq 1 ]] || usage
    echo "apply-upgrade rollback-upgrade"
    exit 0
    ;;
  apply-upgrade)
    [[ $# -eq 2 ]] || usage
    tag="$2"
    ;;
  rollback-upgrade)
    [[ $# -eq 1 ]] || usage
    ;;
  *)
    echo "unitctl: action not allowed: $action" >&2
    exit 2
    ;;
esac

# ---- upgrade verbs (run as root) -------------------------------------------

ensure_bin_dir() {
  if [[ -L "$BIN_DIR" ]]; then
    echo "unitctl: $BIN_DIR is a symlink; refusing" >&2
    exit 1
  fi
  install -d -o root -g root -m 0755 "$BIN_DIR"
  # Older installs left the directory and binary owned by the service user;
  # take them over so a compromised game process cannot rewrite the manager.
  chown root:root "$BIN_DIR"
  chmod 0755 "$BIN_DIR"
  for f in "$BIN_DIR/valheim-ui" "$BIN_DIR/valheim-ui.prev"; do
    [[ -f "$f" && ! -L "$f" ]] && chown root:root "$f" && chmod 0755 "$f"
  done
  return 0
}

if [[ "$action" == "rollback-upgrade" ]]; then
  ensure_bin_dir
  prev="$BIN_DIR/valheim-ui.prev"
  cur="$BIN_DIR/valheim-ui"
  if [[ ! -f "$prev" || -L "$prev" ]]; then
    echo "unitctl: no previous binary at $prev" >&2
    exit 1
  fi
  rm -f "$cur.rolledback"
  mv -f "$cur" "$cur.rolledback"
  if ! mv -f "$prev" "$cur"; then
    mv -f "$cur.rolledback" "$cur"
    echo "unitctl: restoring the previous binary failed" >&2
    exit 1
  fi
  rm -f "$cur.rolledback"
  echo "rolled back to the previous binary"
  exit 0
fi

# apply-upgrade <tag>
if ! [[ "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?$ ]]; then
  echo "unitctl: invalid release tag" >&2
  exit 2
fi
staged="$STAGING_DIR/valheim-ui.new"
if [[ ! -f "$staged" || -L "$staged" ]]; then
  echo "unitctl: no staged binary at $staged" >&2
  exit 1
fi
if [[ "$(stat -c %U "$staged")" != "$SERVICE_USER" ]]; then
  echo "unitctl: staged binary is not owned by $SERVICE_USER" >&2
  exit 1
fi
if [[ "$(head -c 4 "$staged" | od -An -c | tr -d ' ')" != "177ELF" ]]; then
  echo "unitctl: staged file is not an ELF binary" >&2
  exit 1
fi

# The staged file must be byte-identical to the binary published in the
# release: compare against the SHA256SUMS the release workflow signs off.
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
sums_url="https://github.com/$REPO/releases/download/$tag/SHA256SUMS"
if ! curl -fsSL --proto '=https' --tlsv1.2 --max-time 60 -o "$tmp/SHA256SUMS" "$sums_url"; then
  echo "unitctl: could not fetch $sums_url" >&2
  exit 1
fi
expected="$(awk '$2 == "valheim-ui" { print $1 }' "$tmp/SHA256SUMS" | head -n1)"
if [[ -z "$expected" ]]; then
  echo "unitctl: release $tag publishes no checksum for the raw binary; it predates privileged upgrades" >&2
  exit 1
fi
actual="$(sha256sum "$staged" | awk '{ print $1 }')"
if [[ "$expected" != "$actual" ]]; then
  echo "unitctl: staged binary does not match the checksum published for $tag" >&2
  exit 1
fi

ensure_bin_dir
cur="$BIN_DIR/valheim-ui"
install -o root -g root -m 0755 "$staged" "$cur.tmp"
if [[ -f "$cur" ]]; then
  mv -f "$cur" "$cur.prev"
fi
if ! mv -f "$cur.tmp" "$cur"; then
  [[ -f "$cur.prev" ]] && mv -f "$cur.prev" "$cur"
  echo "unitctl: installing the new binary failed" >&2
  exit 1
fi
rm -f "$staged"
echo "installed $tag as $cur (previous kept at $cur.prev)"
UNITCTL_EOF
}

sudoers_content() {
  cat <<'SUDOERS_EOF'
# Installed by deploy/install.sh to /etc/sudoers.d/valheim-ui (mode 0440).
# Grants the manager exactly one root command; unitctl validates its arguments.
# env_reset and secure_path are stated explicitly so a site edit of
# /etc/sudoers can never widen what the wrapper sees.
Defaults:valheim !requiretty, env_reset, secure_path="/usr/sbin:/usr/bin:/sbin:/bin"
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
# Hardening. NoNewPrivileges and RestrictSUIDSGID must stay off: the manager
# calls sudo (setuid) for unitctl. Everything else that does not interfere
# with sudo is on. /var/lib/valheim/bin is root-owned, so the manager cannot
# rewrite its own binary; upgrades go through `unitctl apply-upgrade`.
ProtectSystem=strict
ReadWritePaths=/var/lib/valheim
PrivateTmp=true
ProtectHome=true
PrivateDevices=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectKernelLogs=true
ProtectControlGroups=true
ProtectClock=true
ProtectHostname=true
RestrictNamespaces=true
RestrictRealtime=true
LockPersonality=true
RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6 AF_NETLINK
CapabilityBoundingSet=
AmbientCapabilities=
SystemCallArchitectures=native
UMask=0027
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
# Hardening. Mods run inside this process as arbitrary code, so the game gets
# no more of the host than it needs: its own data tree, HOME (also under
# /var/lib/valheim) and network sockets. NoNewPrivileges blocks sudo from the
# game process entirely; the manager's unit keeps sudo for unitctl.
# MemoryDenyWriteExecute is deliberately absent: the Unity/Mono runtime JITs.
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/valheim
PrivateTmp=true
PrivateDevices=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectKernelLogs=true
ProtectControlGroups=true
ProtectClock=true
ProtectHostname=true
RestrictSUIDSGID=true
RestrictNamespaces=true
RestrictRealtime=true
LockPersonality=true
RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6 AF_NETLINK
CapabilityBoundingSet=
AmbientCapabilities=
SystemCallArchitectures=native
UMask=0027

[Install]
WantedBy=multi-user.target
INSTANCE_UNIT_EOF
}

config_example_content() {
  cat <<'CONFIG_EOF'
# /etc/valheim-ui/config.yaml — manager configuration.
# Every key can be overridden with VALHEIM_UI_<UPPERCASE_KEY>.
# (Windows: %ProgramData%\valheim-ui\config.yaml, written by deploy/install.ps1
# with supervisor: direct and steamcmd_path pointing at steamcmd.exe.)

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

# Runs the installed binary to print its version. Never as root: on an
# install that predates root-owned binaries the file is writable by the
# service user, and a compromised game process must not get a root exec by
# waiting for an admin to run `install.sh --check`.
installed_version() {
  local bin=""
  if [[ -x "$SYMLINK" ]]; then
    bin="$SYMLINK"
  elif [[ -x "$REAL_BIN" ]]; then
    bin="$REAL_BIN"
  else
    echo "(not installed)"
    return 0
  fi
  if id valheim >/dev/null 2>&1; then
    run_as_valheim "$bin" version 2>/dev/null || echo "unknown"
  else
    echo "unknown"
  fi
}

# run_as_valheim CMD... executes CMD as the service user with a clean
# environment (runuser where available, sudo otherwise).
run_as_valheim() {
  if command -v runuser >/dev/null 2>&1; then
    runuser -u valheim -- "$@"
  else
    sudo -u valheim -H -- "$@"
  fi
}

# Install a new valheim-ui binary at $src into the real-binary/symlink layout,
# keeping the previous binary as valheim-ui.prev for rollback (see
# `valheim-ui self-upgrade --rollback`, RUNBOOK.md #11). The swap is a rename
# within $BIN_DIR, so it is atomic from the point of view of anything that has
# the file open or execs through the symlink.
install_binary_files() {
  local src="$1"
  ensure_bin_dir
  if [[ -f "$REAL_BIN" ]]; then
    cp -f "$REAL_BIN" "$REAL_BIN.prev"
    chown root:root "$REAL_BIN.prev"
  fi
  install -o root -g root -m 0755 "$src" "$REAL_BIN.new"
  mv -f "$REAL_BIN.new" "$REAL_BIN"
  ln -sfn "$REAL_BIN" "$SYMLINK"
}

# ensure_bin_dir makes the binary directory a real, root-owned directory.
# It refuses a symlink (a service-user process could otherwise redirect
# root's writes) and takes over an older valheim-owned layout in place.
ensure_bin_dir() {
  if [[ -L "$BIN_DIR" ]]; then
    die "$BIN_DIR is a symlink; refusing to install into it"
  fi
  install -d -o root -g root -m 0755 "$BIN_DIR"
  chown root:root "$BIN_DIR"
  chmod 0755 "$BIN_DIR"
  local f
  for f in "$REAL_BIN" "$REAL_BIN.prev"; do
    if [[ -f "$f" && ! -L "$f" ]]; then
      chown root:root "$f"
      chmod 0755 "$f"
    fi
  done
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
      echo "    (valheim-ui and any running valheim@ instances are stopped briefly for usermod, then started again)"
    fi
  else
    echo "  - system user 'valheim': would be created"
  fi
  if [[ -d "$DATA_DIR" ]]; then
    echo "  - $DATA_DIR: present"
  else
    echo "  - $DATA_DIR: would be created"
  fi
  if [[ -f "$REAL_BIN" ]]; then
    owner="$(stat -c %U "$REAL_BIN" 2>/dev/null || echo '?')"
    if [[ "$owner" == "root" ]]; then
      echo "  - binary ownership: root (privileged upgrades via unitctl)"
    else
      echo "  - binary ownership: $owner -> would move to root:root (see RUNBOOK.md #11)"
    fi
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
  if systemctl is-active --quiet valheim-ui.service 2>/dev/null; then
    echo "  - valheim-ui.service: active; would be restarted onto the installed binary"
  elif systemctl is-enabled --quiet valheim-ui.service 2>/dev/null; then
    echo "  - valheim-ui.service: enabled but not running; would be started"
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

MANAGER_WAS_ACTIVE=0
if systemctl is-active --quiet valheim-ui.service 2>/dev/null; then
  MANAGER_WAS_ACTIVE=1
fi
STOPPED_INSTANCES=()

log "Creating user and directories"
if ! id valheim >/dev/null 2>&1; then
  useradd --system --home-dir "$DATA_DIR" --shell /usr/sbin/nologin --user-group valheim
else
  cur_home="$(getent passwd valheim | cut -d: -f6)"
  if [[ "$cur_home" != "$DATA_DIR" ]]; then
    echo "warning: user 'valheim' already exists with home $cur_home; setting it to $DATA_DIR (the units run with ProtectHome=true and HOME=$DATA_DIR). Files under $cur_home are left in place." >&2
    # usermod refuses to change a user that owns running processes, so stop the
    # manager and any running game instances first; they are started again below.
    if [[ $MANAGER_WAS_ACTIVE -eq 1 ]]; then
      log "Stopping valheim-ui for the home directory change"
      systemctl stop valheim-ui.service
    fi
    while read -r unit; do
      [[ -n "$unit" ]] || continue
      log "Stopping $unit for the home directory change"
      systemctl stop "$unit"
      STOPPED_INSTANCES+=("$unit")
    done < <(systemctl list-units --type=service --state=active --plain --no-legend 'valheim@*' 2>/dev/null | awk '{print $1}')
    usermod -d "$DATA_DIR" valheim
  fi
fi
install -d -o valheim -g valheim -m 0750 "$DATA_DIR" "$DATA_DIR/instances" "$DATA_DIR/jobs" \
  "$DATA_DIR/cache" "$DATA_DIR/steamcmd" "$STAGING_DIR"
ensure_bin_dir
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
  if [[ -n "$BASE_URL" ]]; then
    sed -i "s|^base_url: .*|base_url: \"$BASE_URL\"|" "$CONF_DIR/config.yaml"
  fi
  chown root:valheim "$CONF_DIR/config.yaml"
  chmod 0640 "$CONF_DIR/config.yaml"
fi
if ! grep -Eq '^base_url: *"[^"]+"' "$CONF_DIR/config.yaml" 2>/dev/null; then
  echo "warning: base_url is empty in $CONF_DIR/config.yaml; set it to the public URL (https://...)" >&2
  echo "         so single sign-on works and state-changing requests are Origin-checked." >&2
fi

if [[ ! -x "$DATA_DIR/steamcmd/steamcmd.sh" ]]; then
  log "Installing SteamCMD"
  # Download as root (TLS verified), but unpack as the service user with the
  # archive's ownership and modes ignored: root must never extract a
  # third-party tarball into a directory the service user controls.
  steam_tmp="$(mktemp -d)"
  curl -fsSL --proto '=https' --tlsv1.2 -o "$steam_tmp/steamcmd_linux.tar.gz" "$STEAMCMD_URL"
  log "SteamCMD tarball sha256: $(sha256sum "$steam_tmp/steamcmd_linux.tar.gz" | cut -d' ' -f1) (Valve publishes no pinned digest; recorded for your audit trail)"
  chmod 0644 "$steam_tmp/steamcmd_linux.tar.gz"
  chown valheim:valheim "$DATA_DIR/steamcmd"
  run_as_valheim tar -xzf "$steam_tmp/steamcmd_linux.tar.gz" -C "$DATA_DIR/steamcmd" --no-same-owner --no-same-permissions
  rm -rf "$steam_tmp"
  log "Running SteamCMD self-update (this takes a minute)"
  sudo -u valheim -H "$DATA_DIR/steamcmd/steamcmd.sh" +quit >/dev/null 2>&1 \
    || echo "warning: steamcmd self-update reported an error; it usually works on the next run" >&2
fi

log "Starting valheim-ui"
systemctl daemon-reload
systemctl enable valheim-ui.service >/dev/null 2>&1 || systemctl enable valheim-ui.service
if [[ $MANAGER_WAS_ACTIVE -eq 1 ]]; then
  # Upgrade: pick up the installed binary and unit files. Game instances are
  # independent units and keep running across this restart.
  systemctl restart valheim-ui.service
else
  systemctl start valheim-ui.service
fi
for unit in "${STOPPED_INSTANCES[@]}"; do
  log "Starting $unit again"
  systemctl start "$unit" || echo "warning: could not start $unit; start it from the UI" >&2
done
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

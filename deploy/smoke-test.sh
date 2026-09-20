#!/usr/bin/env bash
# deploy/smoke-test.sh — installs valheim-server-ui for real with deploy/install.sh
# on a systemd host, drives the real systemd supervisor path end to end against
# the fake game server, then uninstalls. See docs/RUNBOOK.md "Install smoke test".
#
# THIS SCRIPT INSTALLS SOFTWARE SYSTEM-WIDE: apt packages, a "valheim" system
# user, systemd units, a sudoers drop-in, and it starts/stops real services.
# Run it only on a throwaway VM or CI runner, never on a workstation. It
# refuses to run unless SMOKE_TEST_I_KNOW_THIS_INSTALLS=1 is set.
#
# Usage (as root, on a systemd host):
#
#   SMOKE_TEST_I_KNOW_THIS_INSTALLS=1 BIN=bin/valheim-ui \
#     FAKE_SERVER=testdata/fake-server.sh bash deploy/smoke-test.sh
#
# Required env:
#   BIN          path to a built valheim-ui binary (linux/amd64)
#   FAKE_SERVER  path to testdata/fake-server.sh
#
# Optional env:
#   SMOKE_DIAG_LOG  where the on-failure diagnostics dump is written
#                   (default /tmp/valheim-ui-smoke-diagnostics.log; CI uploads
#                   this path as an artifact on failure)
set -euo pipefail

if [[ "${SMOKE_TEST_I_KNOW_THIS_INSTALLS:-}" != "1" ]]; then
  echo "refusing to run: this installs valheim-server-ui system-wide (systemd units," >&2
  echo "a sudoers drop-in, a 'valheim' system user, apt packages) and starts/stops" >&2
  echo "real services. Set SMOKE_TEST_I_KNOW_THIS_INSTALLS=1 on a throwaway VM or CI" >&2
  echo "runner to proceed. See docs/RUNBOOK.md, 'Install smoke test'." >&2
  exit 1
fi
[[ $EUID -eq 0 ]] || { echo "smoke-test.sh: must run as root" >&2; exit 1; }
command -v systemctl >/dev/null 2>&1 || { echo "smoke-test.sh: systemd (systemctl) is required" >&2; exit 1; }

: "${BIN:?set BIN to a built valheim-ui binary path, e.g. bin/valheim-ui}"
: "${FAKE_SERVER:?set FAKE_SERVER to testdata/fake-server.sh}"
[[ -f "$BIN" && -x "$BIN" ]] || { echo "smoke-test.sh: BIN=$BIN is not an executable file" >&2; exit 1; }
[[ -f "$FAKE_SERVER" ]] || { echo "smoke-test.sh: FAKE_SERVER=$FAKE_SERVER not found" >&2; exit 1; }

for tool in curl jq visudo systemd-run runuser realpath stat; do
  command -v "$tool" >/dev/null 2>&1 || { echo "smoke-test.sh: required tool not found: $tool" >&2; exit 1; }
done

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN_ABS="$(realpath "$BIN")"
FAKE_SERVER_ABS="$(realpath "$FAKE_SERVER")"

DATA_DIR=/var/lib/valheim
CONF_DIR=/etc/valheim-ui
LIB_DIR=/usr/local/lib/valheim-ui
INSTANCE=smoke
BASE_URL=http://127.0.0.1:8080
API="$BASE_URL/api/v1"
CSRF_HEADER="X-Requested-With: valheim-ui"
COOKIES="$(mktemp)"
DIAG_LOG="${SMOKE_DIAG_LOG:-/tmp/valheim-ui-smoke-diagnostics.log}"

log() { printf '\n\033[1;32m==>\033[0m %s\n' "$*"; }
die() { printf '\033[1;31merror:\033[0m %s\n' "$*" >&2; exit 1; }

# dump_diagnostics prints (and saves to $DIAG_LOG, for CI to upload as an
# artifact) the journal for both units and the manager's config, so a failure
# is debuggable from the CI log alone. Every command is individually guarded
# with `|| true`: this runs from the EXIT trap under `set -e`, and one missing
# unit (e.g. the manager failed before valheim@smoke.service ever existed)
# must not stop the rest of the dump from printing.
dump_diagnostics() {
  {
    echo "----- journalctl -u valheim-ui.service -u 'valheim@*' -----"
    journalctl -u valheim-ui.service -u 'valheim@*' --no-pager -n 200 || true
    echo "----- systemctl status valheim-ui.service -----"
    systemctl --no-pager status valheim-ui.service || true
    echo "----- systemctl status valheim@${INSTANCE}.service -----"
    systemctl --no-pager status "valheim@${INSTANCE}.service" || true
    if [[ -f "$CONF_DIR/config.yaml" ]]; then
      echo "----- $CONF_DIR/config.yaml -----"
      cat "$CONF_DIR/config.yaml" || true
    fi
  } 2>&1 | tee "$DIAG_LOG" >&2
}

on_exit() {
  local rc=$?
  trap - EXIT
  if [[ $rc -ne 0 ]]; then
    dump_diagnostics || true
  fi
  rm -f "$COOKIES" 2>/dev/null || true
  exit "$rc"
}
trap on_exit EXIT

# wait_for BUDGET_SECONDS DESCRIPTION CHECK_FN -- polls CHECK_FN (a niladic
# shell function) once a second until it succeeds, or dies after BUDGET_SECONDS.
wait_for() {
  local budget="$1" desc="$2" fn="$3" waited=0
  until "$fn"; do
    waited=$((waited + 1))
    if [[ $waited -ge $budget ]]; then
      die "timed out after ${budget}s waiting for: $desc"
    fi
    sleep 1
  done
}

manager_active() { systemctl is-active --quiet valheim-ui.service; }
manager_http_ready() { curl -fsS -o /dev/null "$BASE_URL/healthz"; }
instance_inactive() { ! systemctl is-active --quiet "valheim@${INSTANCE}.service"; }

# api METHOD PATH [JSON_BODY] -- calls $API$PATH with the CSRF header and the
# cookie jar (so the session set by /auth/setup or /auth/login survives to the
# next call), and sets API_STATUS / API_BODY for the caller to check.
api() {
  local method="$1" path="$2" body="${3:-}" resp
  if [[ -n "$body" ]]; then
    resp="$(curl -sS -b "$COOKIES" -c "$COOKIES" -X "$method" "$API$path" \
      -H 'Content-Type: application/json' -H "$CSRF_HEADER" -d "$body" -w $'\n%{http_code}')"
  else
    resp="$(curl -sS -b "$COOKIES" -c "$COOKIES" -X "$method" "$API$path" \
      -H "$CSRF_HEADER" -w $'\n%{http_code}')"
  fi
  API_STATUS="${resp##*$'\n'}"
  API_BODY="${resp%$'\n'*}"
}

# ---------------------------------------------------------------------------
# (a) install
# ---------------------------------------------------------------------------
log "install: deploy/install.sh --binary $BIN_ABS"
bash "$REPO_ROOT/deploy/install.sh" --binary "$BIN_ABS"

# ---------------------------------------------------------------------------
# (b) install sanity checks
# ---------------------------------------------------------------------------
log "assert: valheim-ui.service is enabled"
systemctl is-enabled --quiet valheim-ui.service || die "valheim-ui.service is not enabled"

log "assert: valheim-ui.service is active within 30s"
wait_for 30 "valheim-ui.service active" manager_active

log "assert: system user 'valheim' exists"
id valheim >/dev/null 2>&1 || die "user 'valheim' was not created"

log "assert: /etc/sudoers.d/valheim-ui parses"
visudo -cf /etc/sudoers.d/valheim-ui >/dev/null || die "/etc/sudoers.d/valheim-ui failed to validate"

log "assert: valheim can sudo unitctl capabilities (proves the sudoers rule)"
sudo -u valheim sudo -n "$LIB_DIR/unitctl" capabilities >/dev/null \
  || die "sudo -u valheim sudo -n $LIB_DIR/unitctl capabilities failed"

log "assert: GET /healthz returns ok:true"
healthz="$(curl -fsS "$BASE_URL/healthz")" || die "curl $BASE_URL/healthz failed"
printf '%s' "$healthz" | grep -q '"ok":true' || die "unexpected /healthz body: $healthz"

# ---------------------------------------------------------------------------
# steamcmd under the manager unit's real hardening, plus a negative control.
#
# The manager (valheim-ui.service) spawns SteamCMD -- a 32-bit x86 binary --
# as a child process to install/update instances. The API flow below never
# exercises that path: like web/e2e, it uses install:false and a hand-written
# stub binary, specifically so this script does not need real Steam network
# access. That means the flow below, on its own, would never have caught
# SteamCMD being killed by the manager unit's syscall filter (see
# docs/RUNBOOK.md, "SteamCMD is a 32-bit binary and the manager unit's
# system-call filter only allowed the native ABI"). Reproduce the unit's exact
# sandbox with systemd-run instead, against the SteamCMD the installer just
# downloaded, so a regression here is still caught.
#
# Every -p below is copied by hand from deploy/valheim-ui.service's [Service]
# hardening block (current as of this writing); keep the two in sync if that
# block changes. NoNewPrivileges and RestrictSUIDSGID are deliberately absent
# from both: the manager unit leaves them off (it calls sudo for unitctl), so
# a faithful mirror leaves them off here too. Every property in that block is
# expressible as a systemd-run -p flag; none had to be dropped.
# ---------------------------------------------------------------------------
STEAMCMD_HARDENING=(
  -p ProtectSystem=strict
  -p ReadWritePaths=/var/lib/valheim
  -p PrivateTmp=true
  -p ProtectHome=true
  -p PrivateDevices=true
  -p ProtectKernelTunables=true
  -p ProtectKernelModules=true
  -p ProtectKernelLogs=true
  -p ProtectControlGroups=true
  -p ProtectClock=true
  -p ProtectHostname=true
  -p RestrictNamespaces=true
  -p RestrictRealtime=true
  -p LockPersonality=true
  -p "RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6 AF_NETLINK"
  -p CapabilityBoundingSet=
  -p AmbientCapabilities=
  -p UMask=0027
)

# run_steamcmd_sandboxed UNIT_NAME SYSCALL_ARCHITECTURES
run_steamcmd_sandboxed() {
  systemd-run --wait --pipe --collect --unit="$1" \
    --uid=valheim --gid=valheim --setenv=HOME=/var/lib/valheim \
    "${STEAMCMD_HARDENING[@]}" -p "SystemCallArchitectures=$2" \
    /var/lib/valheim/steamcmd/steamcmd.sh +quit
}

log "steamcmd runs under the manager unit's real hardening (SystemCallArchitectures=native x86)"
set +e
steamcmd_out="$(run_steamcmd_sandboxed smoke-steamcmd-ok 'native x86' 2>&1)"
steamcmd_rc=$?
set -e
if [[ $steamcmd_rc -ne 0 ]]; then
  echo "$steamcmd_out"
  die "steamcmd exited $steamcmd_rc under the manager unit's real hardening; this is the exact bug this check exists to catch"
fi
log "steamcmd exits 0 under the manager unit's real hardening"

log "negative control: SystemCallArchitectures=native (no x86) must kill steamcmd with SIGSYS"
set +e
sigsys_out="$(run_steamcmd_sandboxed smoke-steamcmd-sigsys native 2>&1)"
sigsys_rc=$?
set -e
echo "$sigsys_out"
if [[ $sigsys_rc -eq 0 ]]; then
  die "negative control did not fail: steamcmd exited 0 under SystemCallArchitectures=native (no x86); this check would not catch a regression back to the old, broken filter"
fi
if [[ $sigsys_rc -eq 159 || $sigsys_rc -eq 31 ]] || grep -qiE 'bad system call|sigsys' <<<"$sigsys_out"; then
  log "confirmed: steamcmd was killed by SIGSYS under the old filter (exit $sigsys_rc), as expected"
else
  die "negative control failed (exit $sigsys_rc) but not with the expected SIGSYS signature; investigate before trusting this check"
fi

# ---------------------------------------------------------------------------
# (c) point the installed manager at the fake game server
# ---------------------------------------------------------------------------
log "point the installed manager at the fake game server"
# ProtectHome=true on both units makes /home (and /root, /run/user) entirely
# inaccessible to them -- not just read-only, invisible. GitHub-hosted runners
# check this repo out under /home/runner/..., so testdata/fake-server.sh would
# be unreachable to valheim@smoke.service if we pointed fake_server_path at it
# directly there. Copy it under /var/lib/valheim instead, which is already
# each unit's ReadWritePaths target and is not under /home.
install -o valheim -g valheim -m 0755 "$FAKE_SERVER_ABS" "$DATA_DIR/smoke-fake-server.sh"

# fake_server / fake_server_path are read by both valheim-ui.service (serve)
# and valheim@.service (launch --instance ...): both units already set
# Environment=VALHEIM_UI_CONFIG=/etc/valheim-ui/config.yaml, so editing that
# one shared file reaches both processes. No systemctl daemon-reload is
# needed -- that command reloads unit *files*; this is a plain data file each
# process reads for itself the next time it starts.
#
# insecure_cookies must flip to true as well: the session cookie the API sets
# is Secure-flagged by default (internal/auth/session.go), and curl -- like a
# browser -- will not resend a Secure cookie over the plain http:// this
# script talks. web/e2e/config.yaml sets the same two keys for the same two
# reasons.
sed -i 's/^insecure_cookies: .*/insecure_cookies: true/' "$CONF_DIR/config.yaml"
{
  echo "fake_server: true"
  echo "fake_server_path: \"$DATA_DIR/smoke-fake-server.sh\""
} >> "$CONF_DIR/config.yaml"

systemctl restart valheim-ui.service
wait_for 30 "valheim-ui.service active after config change" manager_active
# is-active for a Type=simple unit means "exec'd", not "listener bound"; poll
# /healthz too so the API flow below does not race the HTTP server's startup.
wait_for 30 "manager HTTP listener ready after restart" manager_http_ready

# ---------------------------------------------------------------------------
# (d) drive the API like web/e2e does: setup, login, create, fake-install,
#     start, wait for running, stop.
# ---------------------------------------------------------------------------
log "first-run setup"
api POST /auth/setup '{"username":"smoke-admin","password":"smoke-admin-pw","display_name":"Smoke Admin"}'
[[ "$API_STATUS" == "201" ]] || die "auth/setup failed (status $API_STATUS): $API_BODY"

log "login"
api POST /auth/login '{"username":"smoke-admin","password":"smoke-admin-pw"}'
[[ "$API_STATUS" == "200" ]] || die "auth/login failed (status $API_STATUS): $API_BODY"

log "create instance $INSTANCE (install:false, like web/e2e)"
api POST /instances "$(cat <<JSON
{"id":"$INSTANCE","name":"Smoke Test","install":false,"autostart":false,
 "config":{"name":"Smoke Test","world":"Smoketest","password":"smoke-1234","port":2456}}
JSON
)"
[[ "$API_STATUS" == "201" ]] || die "create instance failed (status $API_STATUS): $API_BODY"

log "fake-install $INSTANCE (mirrors web/e2e/tests/helpers.ts fakeInstall, as the valheim user)"
server_bin="$DATA_DIR/instances/$INSTANCE/server/valheim_server.x86_64"
runuser -u valheim -- sh -c "printf '#!/bin/sh\n' > '$server_bin' && chmod 0755 '$server_bin'"

log "start $INSTANCE"
api POST "/instances/$INSTANCE/start"
[[ "$API_STATUS" == "200" ]] || die "start failed (status $API_STATUS): $API_BODY"

log "poll GET /instances/$INSTANCE/status until state == running (60s)"
waited=0
state="(none)"
while :; do
  api GET "/instances/$INSTANCE/status"
  [[ "$API_STATUS" == "200" ]] || die "status check failed (status $API_STATUS): $API_BODY"
  state="$(printf '%s' "$API_BODY" | jq -r '.status.state')"
  [[ "$state" == "running" ]] && break
  waited=$((waited + 1))
  [[ $waited -ge 60 ]] && die "instance did not reach state=running within 60s (last state: $state)"
  sleep 1
done
log "instance reached state=running after ${waited}s"

log "assert: systemctl is-active valheim@${INSTANCE}.service"
systemctl is-active --quiet "valheim@${INSTANCE}.service" || die "valheim@${INSTANCE}.service is not active"

log "assert: logs/console.log grows"
console_log="$DATA_DIR/instances/$INSTANCE/logs/console.log"
[[ -f "$console_log" ]] || die "$console_log does not exist"
size1=$(stat -c %s "$console_log")
sleep 5
size2=$(stat -c %s "$console_log")
[[ "$size2" -gt "$size1" ]] || die "$console_log did not grow ($size1 -> $size2 bytes over 5s)"

log "stop $INSTANCE via the API"
api POST "/instances/$INSTANCE/stop"
[[ "$API_STATUS" == "200" ]] || die "stop failed (status $API_STATUS): $API_BODY"

log "assert: valheim@${INSTANCE}.service becomes inactive"
wait_for 30 "valheim@${INSTANCE}.service inactive" instance_inactive

# ---------------------------------------------------------------------------
# (e) uninstall
# ---------------------------------------------------------------------------
log "uninstall: deploy/install.sh --uninstall"
bash "$REPO_ROOT/deploy/install.sh" --uninstall

log "assert: both unit files are gone from systemctl list-unit-files"
if systemctl list-unit-files 2>/dev/null | grep -qE '^valheim-ui\.service|^valheim@\.service'; then
  die "valheim-ui.service or valheim@.service still listed in 'systemctl list-unit-files' after uninstall"
fi

log "assert: /etc/sudoers.d/valheim-ui is gone"
[[ -e /etc/sudoers.d/valheim-ui ]] && die "/etc/sudoers.d/valheim-ui still present after uninstall"

log "assert: the binary is gone"
[[ -e /usr/local/bin/valheim-ui ]] && die "/usr/local/bin/valheim-ui still present after uninstall"
[[ -e "$DATA_DIR/bin/valheim-ui" ]] && die "$DATA_DIR/bin/valheim-ui still present after uninstall"

log "assert: $DATA_DIR still exists (data kept by design)"
[[ -d "$DATA_DIR" ]] || die "$DATA_DIR was removed; instance/world data must be kept across uninstall"

log "all checks passed"

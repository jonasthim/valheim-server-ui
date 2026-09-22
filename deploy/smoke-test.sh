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

for tool in curl jq visudo systemd-run runuser realpath stat awk; do
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
manager_http_ready() { curl -fs -o /dev/null "$BASE_URL/healthz" 2>/dev/null; }
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
# sudo and steamcmd inside the manager unit's real sandbox, with negative
# controls.
#
# Two things the API flow below cannot pin down on its own:
#
#  1. The manager (valheim-ui.service) escalates to root through the setuid
#     sudo (for unitctl) -- and systemd silently sets the no_new_privs flag on
#     any User= unit that uses a seccomp-backed sandboxing option (see the
#     comment block in deploy/valheim-ui.service). With that flag set, sudo
#     refuses to run and every start, stop and upgrade fails. The API flow
#     does go through sudo, but several layers deep; this runs the exact
#     command the manager runs, alone, so the failure is unmistakable.
#  2. SteamCMD is a 32-bit x86 binary the manager spawns as a child. The API
#     flow never runs it (install:false and a stub binary, like web/e2e, so no
#     Steam network access is needed), so a syscall filter killing it would
#     go unnoticed.
#
# Both run under systemd-run with every sandboxing directive copied from the
# unit file install.sh just wrote to /etc/systemd/system -- read at runtime,
# so this mirror cannot drift from the unit. Each check has a negative
# control that adds SystemCallArchitectures=native (what the manager unit
# shipped from v1.3.0 to v1.16.6) and must fail, proving the check would
# catch that regression.
# ---------------------------------------------------------------------------
MANAGER_UNIT=/etc/systemd/system/valheim-ui.service
[[ -f "$MANAGER_UNIT" ]] || die "$MANAGER_UNIT was not installed"

# Every [Service] directive except the ones that say *what* runs and how it is
# supervised; what remains is the sandbox. Read line by line into an array so
# a value with spaces stays one argument.
mapfile -t MANAGER_SANDBOX < <(
  awk '
    /^\[/ { in_service = ($0 == "[Service]"); next }
    !in_service { next }
    /^[[:space:]]*(#|;|$)/ { next }
    /^(Type|User|Group|Environment|EnvironmentFile|ExecStart|ExecStartPre|ExecStartPost|ExecStop|ExecStopPost|ExecReload|Restart|RestartSec|WorkingDirectory|StandardInput|StandardOutput|StandardError|KillSignal|KillMode|TimeoutStopSec|TimeoutStartSec|Nice)=/ { next }
    { print "-p"; print }
  ' "$MANAGER_UNIT"
)
[[ ${#MANAGER_SANDBOX[@]} -gt 0 ]] || die "no sandboxing directives found in $MANAGER_UNIT"
log "manager sandbox mirrored from $MANAGER_UNIT:$(printf ' %s' "${MANAGER_SANDBOX[@]}")"

# run_sandboxed UNIT_NAME [-p EXTRA_PROPERTY]... -- COMMAND [ARG]...
# Runs COMMAND as the valheim user under the mirrored sandbox (plus any extra
# properties), with stdout/stderr piped back and the unit's exit status
# propagated.
run_sandboxed() {
  local unit="$1" extra=()
  shift
  while [[ $# -gt 0 && "$1" != "--" ]]; do
    extra+=("$1")
    shift
  done
  [[ $# -gt 0 ]] && shift
  systemd-run --wait --pipe --collect --unit="$unit" \
    --uid=valheim --gid=valheim --setenv=HOME=/var/lib/valheim \
    "${MANAGER_SANDBOX[@]}" "${extra[@]}" -- "$@"
}

SUDO_BIN="$(command -v sudo)"
UNITCTL_CMD=("$SUDO_BIN" -n "$LIB_DIR/unitctl" capabilities)

log "sudo -n unitctl works inside the manager unit's sandbox (what every start/stop/upgrade does)"
set +e
sudo_out="$(run_sandboxed smoke-sudo-ok -- "${UNITCTL_CMD[@]}" 2>&1)"
sudo_rc=$?
set -e
echo "$sudo_out"
if [[ $sudo_rc -ne 0 ]]; then
  die "sudo exited $sudo_rc inside the manager unit's sandbox: the unit's hardening stops the setuid sudo (no_new_privs, or an emptied capability set; see deploy/valheim-ui.service), so nothing the manager does through unitctl can work"
fi
grep -q 'apply-upgrade' <<<"$sudo_out" || die "unitctl capabilities printed nothing recognisable: $sudo_out"
log "sudo -n unitctl exits 0 inside the manager unit's sandbox"

log "negative control: NoNewPrivileges=true must make sudo fail (proves this check can see a broken sudo)"
set +e
sudo_nnp_out="$(run_sandboxed smoke-sudo-nnp -p NoNewPrivileges=true -- "${UNITCTL_CMD[@]}" 2>&1)"
sudo_nnp_rc=$?
set -e
echo "$sudo_nnp_out"
if [[ $sudo_nnp_rc -eq 0 ]]; then
  die "negative control did not fail: sudo ran under NoNewPrivileges=true; this check could not catch a manager unit that blocks sudo"
fi
log "confirmed: sudo fails (exit $sudo_nnp_rc) under NoNewPrivileges=true, as expected"

# Two probes of what the v1.3.0-v1.16.6 hardening block does to sudo on this
# host's systemd. Before v255 a seccomp-backed option on a User= unit implied
# NoNewPrivileges (the bug); v255+ installs the filter before dropping
# privileges instead, so the same option no longer breaks sudo there
# (exec-invoke.c, keep_seccomp_privileges). An empty CapabilityBoundingSet=
# leaves the root that sudo becomes without any capability, on every version.
# The pre-v255 behaviour is asserted where it applies; both results are
# logged as evidence either way.
systemd_version="$(systemctl --version | awk 'NR == 1 { print $2 }')"
log "systemd $systemd_version: probing what the old unit's hardening does to sudo"
set +e
probe_arch_out="$(run_sandboxed smoke-sudo-probe-arch -p SystemCallArchitectures=native -- "${UNITCTL_CMD[@]}" 2>&1)"
probe_arch_rc=$?
probe_caps_out="$(run_sandboxed smoke-sudo-probe-caps -p CapabilityBoundingSet= -- "${UNITCTL_CMD[@]}" 2>&1)"
probe_caps_rc=$?
set -e
echo "--- SystemCallArchitectures=native -> sudo exit $probe_arch_rc"
sed 's/^/    /' <<<"$probe_arch_out"
echo "--- CapabilityBoundingSet= (empty) -> sudo exit $probe_caps_rc"
sed 's/^/    /' <<<"$probe_caps_out"
if [[ "${systemd_version%%.*}" -lt 255 && $probe_arch_rc -eq 0 ]]; then
  die "systemd $systemd_version ran sudo under SystemCallArchitectures=native; versions before 255 are expected to imply NoNewPrivileges here (docs/DECISIONS.md, ADR-019)"
fi

STEAMCMD=/var/lib/valheim/steamcmd/steamcmd.sh

log "steamcmd (32-bit) runs inside the manager unit's sandbox"
set +e
steamcmd_out="$(run_sandboxed smoke-steamcmd-ok -- "$STEAMCMD" +quit 2>&1)"
steamcmd_rc=$?
set -e
if [[ $steamcmd_rc -ne 0 ]]; then
  echo "$steamcmd_out"
  die "steamcmd exited $steamcmd_rc inside the manager unit's sandbox; every install/update job would fail the same way"
fi
log "steamcmd exits 0 inside the manager unit's sandbox"

log "negative control: SystemCallArchitectures=native must kill the 32-bit steamcmd with SIGSYS"
set +e
sigsys_out="$(run_sandboxed smoke-steamcmd-sigsys -p SystemCallArchitectures=native -- "$STEAMCMD" +quit 2>&1)"
sigsys_rc=$?
set -e
echo "$sigsys_out"
if [[ $sigsys_rc -eq 0 ]]; then
  die "negative control did not fail: steamcmd exited 0 under SystemCallArchitectures=native; this check would not catch a regression back to the old, broken filter"
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

log "assert: logs/console.log keeps growing (the fake server logs at least every 10s)"
console_log="$DATA_DIR/instances/$INSTANCE/logs/console.log"
[[ -f "$console_log" ]] || die "$console_log does not exist"
size1=$(stat -c %s "$console_log")
console_grew() { [[ "$(stat -c %s "$console_log")" -gt "$size1" ]]; }
wait_for 30 "$console_log to grow beyond $size1 bytes" console_grew

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

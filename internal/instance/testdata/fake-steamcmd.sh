#!/usr/bin/env bash
# Fake steamcmd for WP-05 job tests: no network, no real download. Mimics just
# enough of steamcmd's behaviour for internal/steam.Client and
# internal/instance.SteamJobs to exercise their success/failure paths:
#
#   - `+force_install_dir <dir> ... +app_update 896660 validate +quit`
#     writes <dir>/steamapps/appmanifest_896660.acf with a buildid and prints
#     the success marker steam.Client looks for.
#   - `+app_info_print 896660` prints a VDF snippet with the public branch's
#     buildid, for LatestBuildID/UpdateChecker.
#
# The buildid written/reported is $FAKE_STEAMCMD_BUILDID (default 10000001),
# so tests can change it between calls to simulate a new release. Setting
# FAKE_STEAMCMD_FAIL=1 simulates a steamcmd failure instead (app_update mode
# only; app_info_print always succeeds).
set -u

buildid="${FAKE_STEAMCMD_BUILDID:-10000001}"
installdir=""
mode="update"

args=("$@")
i=0
while [[ $i -lt ${#args[@]} ]]; do
  case "${args[$i]}" in
    +force_install_dir)
      installdir="${args[$((i+1))]}"
      i=$((i+2))
      ;;
    +app_info_print)
      mode="info"
      i=$((i+2))
      ;;
    *)
      i=$((i+1))
      ;;
  esac
done

# Simulate real steamcmd taking a moment, so tests calling EnqueueInstall
# twice in a row can observe the first job still active.
sleep 0.2

if [[ "$mode" == "info" ]]; then
  cat <<EOF
"896660"
{
	"branches"
	{
		"public"
		{
			"buildid"		"$buildid"
		}
	}
}
EOF
  exit 0
fi

if [[ "${FAKE_STEAMCMD_FAIL:-0}" == "1" ]]; then
  echo "ERROR! Update job failed (simulated)" >&2
  exit 1
fi

if [[ -n "$installdir" ]]; then
  mkdir -p "$installdir/steamapps"
  cat > "$installdir/steamapps/appmanifest_896660.acf" <<EOF
"AppState"
{
	"appid"		"896660"
	"universe"		"1"
	"buildid"		"$buildid"
}
EOF
fi

echo "Success! App '896660' fully installed"
exit 0

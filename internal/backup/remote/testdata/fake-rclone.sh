#!/usr/bin/env bash
# Fake rclone for F-1.4 uploader tests: no network, no real remote. Records
# its argv (one per line) to the file named by $FAKE_RCLONE_LOG, so a test can
# assert the exact "copyto <zip> <remote>/<instance>/<name>" invocation.
# Setting FAKE_RCLONE_FAIL=1 simulates a failed copy: prints "boom" to
# stderr and exits 1.
set -u

if [[ -n "${FAKE_RCLONE_LOG:-}" ]]; then
  printf '%s\n' "$@" > "$FAKE_RCLONE_LOG"
fi

if [[ "${FAKE_RCLONE_FAIL:-0}" == "1" ]]; then
  echo "boom" >&2
  exit 1
fi

exit 0

#!/usr/bin/env bash
# Generates deploy/install.sh from deploy/install.sh.in by inlining the
# standalone deploy/* files at their `# @@INCLUDE <name>@@` marker lines.
#
# deploy/*  (unitctl, sudoers.d/valheim-ui, valheim-ui.service, valheim@.service,
# config.example.yaml) remain the source of truth for their own content; this
# script only copies them, verbatim, into the heredoc bodies of install.sh.in
# to produce a single self-contained install.sh that a plain
# `curl -fsSL .../install.sh | sudo bash` can run with nothing else to fetch.
#
# Run via `make deploy-sync`. The generated deploy/install.sh is committed to
# the repo (the raw.githubusercontent.com URL in README/RUNBOOK serves it
# directly); `make deploy-sync-check` (run in CI) regenerates it and fails the
# build if the committed file has drifted from its sources.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

SRC="deploy/install.sh.in"
OUT="deploy/install.sh"

[[ -f "$SRC" ]] || { echo "build-installer.sh: $SRC not found" >&2; exit 1; }

declare -A INCLUDE_FILES=(
  [unitctl]="deploy/unitctl"
  [sudoers]="deploy/sudoers.d/valheim-ui"
  ["valheim-ui.service"]="deploy/valheim-ui.service"
  ["valheim@.service"]="deploy/valheim@.service"
  ["config.example.yaml"]="deploy/config.example.yaml"
)

tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT

first_line=1
while IFS= read -r line || [[ -n "$line" ]]; do
  if [[ $first_line -eq 1 ]]; then
    first_line=0
    if [[ "$line" == "#!"* ]]; then
      printf '%s\n' "$line" >> "$tmp"
      continue
    fi
    # $SRC has no shebang for some reason; fall through and process it as a
    # normal line below.
  fi
  if [[ "$line" =~ ^([[:space:]]*)#\ @@INCLUDE\ (.+)@@[[:space:]]*$ ]]; then
    indent="${BASH_REMATCH[1]}"
    key="${BASH_REMATCH[2]}"
    path="${INCLUDE_FILES[$key]:-}"
    [[ -n "$path" ]] || { echo "build-installer.sh: unknown include '@@INCLUDE $key@@' in $SRC" >&2; exit 1; }
    [[ -f "$path" ]] || { echo "build-installer.sh: include source '$path' not found" >&2; exit 1; }
    sed "s/^/${indent}/" "$path" >> "$tmp"
  else
    printf '%s\n' "$line" >> "$tmp"
  fi
done < "$SRC"

install -m 0755 "$tmp" "$OUT"
echo "Generated $OUT from $SRC"

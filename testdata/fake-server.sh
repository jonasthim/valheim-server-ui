#!/usr/bin/env bash
# Fake valheim_server.x86_64 for development and e2e. Mimics the console output
# the log parser cares about (ARCHITECTURE.md §8) and exits cleanly on SIGINT.
# Args are the real Valheim flags; we echo them (password masked) for debugging.
set -u
ts() { date '+%m/%d/%Y %H:%M:%S'; }
say() { echo "$(ts): $*"; }

name="Fake"; port=2456; world="Dedicated"
while [[ $# -gt 0 ]]; do
  case "$1" in
    -name) name="$2"; shift 2 ;;
    -port) port="$2"; shift 2 ;;
    -world) world="$2"; shift 2 ;;
    -password) shift 2 ;;
    *) shift ;;
  esac
done

stopping=0
on_int() { stopping=1; }
trap on_int INT TERM

echo "Starting server PRESS CTRL-C to exit"
say "Valheim version: 0.220.5 (fake) DOORSTOP_ENABLED=${DOORSTOP_ENABLED:-unset} SteamAppId=${SteamAppId:-unset}"
say "Load world: $world"
sleep 1
say "Zonesystem Start 0"
say "DungeonDB Start 0"
sleep 1
say "Steam game server initialized"
say "Game server connected"
say "Session \"$name\" with join code 123456 and IP 203.0.113.10:$port is active with 0 player(s)"

players=0
i=0
while [[ $stopping -eq 0 ]]; do
  sleep 1
  i=$((i+1))
  if [[ $i -eq 5 ]]; then
    say "Got connection SteamID 76561198000000001"
    say "Got handshake from client 76561198000000001"
    say "Got character ZDOID from Bjorn : 12345:1"
    players=1
  fi
  if [[ $i -eq 25 ]]; then
    say "Closing socket 76561198000000001"
    players=0
  fi
  if (( i % 10 == 0 )); then
    say "Session \"$name\" with join code 123456 and IP 203.0.113.10:$port is active with $players player(s)"
  fi
  if (( i % 30 == 0 )); then
    say "World saved ( 42.1ms )"
  fi
done
say "OnApplicationQuit"
say "World saved ( 38.7ms )"
say "Net scene destroyed"
exit 0

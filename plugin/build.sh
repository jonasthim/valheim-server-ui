#!/usr/bin/env bash
# Builds the Valheim UI Agent plugin and packages it as a Thunderstore-style
# zip the manager installs like any other mod.
#
#   plugin/build.sh [VERSION]      VERSION is X.Y.Z (default 0.0.0)
#
# The game assemblies are proprietary and never committed: they are taken
# from the dedicated server, which SteamCMD serves to anonymous logins
# (app 896660). Set VALHEIM_MANAGED to reuse an existing install's
# valheim_server_Data/Managed directory instead of downloading.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")"

VERSION="${1:-0.0.0}"
[[ "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo "build.sh: version must be X.Y.Z, got $VERSION" >&2; exit 2; }

LIB="$PWD/lib"
MANAGED="${VALHEIM_MANAGED:-$LIB/valheim}"

if [[ ! -f "$MANAGED/assembly_valheim.dll" ]]; then
  echo "==> downloading the Valheim dedicated server with SteamCMD (anonymous)"
  mkdir -p "$LIB/steamcmd" "$LIB/server"
  if [[ ! -x "$LIB/steamcmd/steamcmd.sh" ]]; then
    curl -fsSL https://steamcdn-a.akamaihd.net/client/installer/steamcmd_linux.tar.gz | tar -xz -C "$LIB/steamcmd"
  fi
  "$LIB/steamcmd/steamcmd.sh" +@sSteamCmdForcePlatformType linux \
    +force_install_dir "$LIB/server" +login anonymous +app_update 896660 validate +quit
  mkdir -p "$MANAGED"
  cp "$LIB/server/valheim_server_Data/Managed/"*.dll "$MANAGED/"
  # Only the assemblies are needed from here on; drop the 1 GB install.
  rm -rf "$LIB/server"
fi

echo "==> building ValheimUI.Agent $VERSION"
cat > ValheimUI.Agent/Version.cs <<CS
namespace ValheimUI.Agent
{
    // Rewritten by plugin/build.sh with the release version; 0.0.0 marks a
    // local or branch build.
    internal static class BuildInfo
    {
        public const string Version = "$VERSION";
    }
}
CS
dotnet build ValheimUI.Agent/ValheimUI.Agent.csproj -c Release --nologo \
  -p:Version="$VERSION" -p:ValheimManaged="$MANAGED"

echo "==> packaging"
OUT="$PWD/dist"
rm -rf "$OUT"
mkdir -p "$OUT/pkg/plugins"
cp ValheimUI.Agent/bin/Release/net472/ValheimUI.Agent.dll "$OUT/pkg/plugins/"
sed "s/__VERSION__/$VERSION/" manifest.json > "$OUT/pkg/manifest.json"
cp README.md "$OUT/pkg/README.md"
[[ -f icon.png ]] && cp icon.png "$OUT/pkg/icon.png"
( cd "$OUT/pkg" && zip -qr ../valheim-ui-agent.zip . )
( cd "$OUT" && sha256sum valheim-ui-agent.zip )
echo "artifact: $OUT/valheim-ui-agent.zip"

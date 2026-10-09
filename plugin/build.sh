#!/usr/bin/env bash
# Builds the server agent, gameplay, and optional Survival client plugins and packages
# each as a Thunderstore-style zip the manager installs like any other mod.
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

# Packs $1 (a directory) into $2 (a zip path), preferring zip but falling
# back to bsdtar (this workstation has no zip; CI has both). A failure here
# must fail the script: the old `( cd ... && zip ... )` subshell swallowed
# packaging errors.
package_zip() {
  local src="$1" out="$2"
  if command -v zip >/dev/null 2>&1; then
    ( cd "$src" && zip -qr "$out" . ) || exit 1
  else
    ( cd "$src" && bsdtar -a -cf "$out" -- * ) || exit 1
  fi
}

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

echo "==> building ValheimUI.Gameplay $VERSION"
cat > ValheimUI.Gameplay/Version.cs <<CS
namespace ValheimUI.Gameplay
{
    // Rewritten by plugin/build.sh with the release version; 0.0.0 marks a
    // local or branch build.
    internal static class BuildInfo
    {
        public const string Version = "$VERSION";
    }
}
CS
dotnet build ValheimUI.Gameplay/ValheimUI.Gameplay.csproj -c Release --nologo \
  -p:Version="$VERSION" -p:ValheimManaged="$MANAGED"

echo "==> building ValheimUI.SurvivalClient $VERSION"
cat > ValheimUI.SurvivalClient/Version.cs <<CS
namespace ValheimUI.SurvivalClient
{
    internal static class BuildInfo
    {
        public const string Version = "$VERSION";
    }
}
CS
dotnet build ValheimUI.SurvivalClient/ValheimUI.SurvivalClient.csproj -c Release --nologo \
  -p:Version="$VERSION" -p:ValheimManaged="$MANAGED"

echo "==> packaging"
OUT="$PWD/dist"
rm -rf "$OUT"

mkdir -p "$OUT/agent-pkg/plugins"
cp ValheimUI.Agent/bin/Release/net472/ValheimUI.Agent.dll "$OUT/agent-pkg/plugins/"
sed "s/__VERSION__/$VERSION/" manifest.json > "$OUT/agent-pkg/manifest.json"
cp README.md "$OUT/agent-pkg/README.md"
[[ -f icon.png ]] && cp icon.png "$OUT/agent-pkg/icon.png"
package_zip "$OUT/agent-pkg" "$OUT/valheim-ui-agent.zip"

mkdir -p "$OUT/gameplay-pkg/plugins"
cp ValheimUI.Gameplay/bin/Release/net472/ValheimUI.Gameplay.dll "$OUT/gameplay-pkg/plugins/"
sed "s/__VERSION__/$VERSION/" ValheimUI.Gameplay/manifest.json > "$OUT/gameplay-pkg/manifest.json"
cp ValheimUI.Gameplay/README.md "$OUT/gameplay-pkg/README.md"
package_zip "$OUT/gameplay-pkg" "$OUT/valheim-ui-gameplay.zip"

mkdir -p "$OUT/survival-client-pkg/plugins"
cp ValheimUI.SurvivalClient/bin/Release/net472/ValheimUI.SurvivalClient.dll "$OUT/survival-client-pkg/plugins/"
sed "s/__VERSION__/$VERSION/" ValheimUI.SurvivalClient/manifest.json > "$OUT/survival-client-pkg/manifest.json"
cp ValheimUI.SurvivalClient/README.md "$OUT/survival-client-pkg/README.md"
cp ValheimUI.SurvivalClient/icon.png "$OUT/survival-client-pkg/icon.png"
package_zip "$OUT/survival-client-pkg" "$OUT/valheim-ui-survival-client.zip"

( cd "$OUT" && sha256sum valheim-ui-agent.zip valheim-ui-gameplay.zip valheim-ui-survival-client.zip )
echo "artifact: $OUT/valheim-ui-agent.zip"
echo "artifact: $OUT/valheim-ui-gameplay.zip"
echo "artifact: $OUT/valheim-ui-survival-client.zip"

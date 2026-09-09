#!/bin/bash
# BepInEx/doorstop launcher script (pack 5.4.2333 variant).

####
export DOORSTOP_ENABLED=1
export DOORSTOP_TARGET_ASSEMBLY=./BepInEx/core/BepInEx.Preloader.dll
export LD_LIBRARY_PATH="./doorstop_libs:$LD_LIBRARY_PATH"
export LD_PRELOAD="libdoorstop_x64.so:$LD_PRELOAD"
####

export LD_LIBRARY_PATH="./linux64:$LD_LIBRARY_PATH"
export SteamAppId=892970

exec ./valheim_server.x86_64 -name "My server" -port 2456 -world "Dedicated" -password "secret" -public 1

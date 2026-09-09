#!/bin/bash
# BepInEx/doorstop launcher script (pack 5.4.2202 variant).
echo "Preloader is starting..."

####
export DOORSTOP_ENABLE=TRUE
export DOORSTOP_INVOKE_DLL_PATH=./BepInEx/core/BepInEx.Preloader.dll
export DOORSTOP_CORLIB_OVERRIDE_PATH=./unstripped_corlib
export LD_LIBRARY_PATH="./doorstop_libs:$LD_LIBRARY_PATH"
export LD_PRELOAD="libdoorstop_x64.so:$LD_PRELOAD"
####

export LD_LIBRARY_PATH="./linux64:$LD_LIBRARY_PATH"
export SteamAppId=892970

exec ./valheim_server.x86_64 -name "My server" -port 2456 -world "Dedicated" -password "secret" -public 1

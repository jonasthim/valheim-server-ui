# Valheim UI Survival Client

Optional BepInEx plugin for each player's Valheim game. It records the fatal
hit's attacker, level, and cause on the player's own network object just before
Valheim marks it dead. The Valheim UI Agent on the dedicated server reads those
details only after it confirms the death. No account, token, or extra network
port is needed.

Install BepInExPack Valheim on the game client, then place
`ValheimUI.SurvivalClient.dll` in `BepInEx/plugins/` and restart the game.
The server must run a matching or newer Valheim UI Agent. Players without this
plugin still have their deaths and biomes recorded; killer and cause remain
unknown. Environmental deaths have a cause but no killer. Existing deaths are
not retroactively enriched.

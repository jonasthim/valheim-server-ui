# Valheim UI Agent

Server-side BepInEx plugin installed by [Valheim Server UI](https://github.com/jonasthim/valheim-server-ui)
together with BepInEx. It exposes the running world to the manager over a
loopback-only HTTP API (bearer token written by the manager before each start)
and executes a fixed set of admin commands. Players need nothing installed.

- `GET /v1/status`: players with positions, day, time of day, weather, global
  keys (progression only) and world modifiers.
- `GET /v1/events?since=N`: join/leave and executed commands.
- `POST /v1/commands/{save|kick|ban|unban|broadcast|time|say|setkey|removekey|event|eventstop}`:
  results are `{"ok","message"}` plus an optional `data` object (see the
  contract in docs/ARCHITECTURE.md §20).
- `GET /v1/chat?since=N&limit=M`: shouts and normal chat the server relayed
  (never whispers); `GET /v1/catalog`: known global keys and the world's
  random events for pickers.
- `GET /v1/map/layers`: raw map layers (biome, height, forest) the manager
  draws the map from; `GET /v1/map/info`, `POST /v1/map/render`.
- `GET /v1/map/objects`: portals, ships, carts, tombstones, beds, shared
  pins, boss altars and other locations with display labels;
  `GET /v1/map/explored`: fog-of-war mask.

Commands are POSTs; a client that sends no `Content-Length` on a bodyless
POST gets `411 Length Required` from the .NET listener (`curl -X POST` alone
does that; add `-d ''`). The manager always sends it.

Configuration: `BepInEx/config/se.jonasthim.valheimui.agent.cfg`. The API only
listens on 127.0.0.1 unless `BindAddress` is changed. `[Chat] ServerName`
overrides the name the `say` command speaks as; empty uses the game's server
name.

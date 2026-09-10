# Valheim UI Agent

Server-side BepInEx plugin installed by [Valheim Server UI](https://github.com/jonasthim/valheim-server-ui)
together with BepInEx. It exposes the running world to the manager over a
loopback-only HTTP API (bearer token written by the manager before each start)
and executes a fixed set of admin commands. Players need nothing installed.

- `GET /v1/status`: players with positions, day, time of day, weather, global keys.
- `GET /v1/events?since=N`: join/leave and executed commands.
- `POST /v1/commands/{save|kick|ban|unban|broadcast}`.

Configuration: `BepInEx/config/se.jonasthim.valheimui.agent.cfg`. The API only
listens on 127.0.0.1 unless `BindAddress` is changed.

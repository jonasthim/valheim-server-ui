# Valheim UI Gameplay

Opt-in, server-side BepInEx plugin installed by [Valheim Server UI](https://github.com/jonasthim/valheim-server-ui)
together with BepInEx, separately from [Valheim UI Agent](../README.md). Two
small gameplay tweaks, each with its own on/off switch — disable or uninstall
this plugin at any time without affecting the agent or the rest of the
manager. Players install nothing.

- **Autofeed:** feeds hungry tamed animals from nearby chests marked with a
  sign, so you do not have to walk food to every pen by hand.
- **Raids:** turns specific random events (raids) off, so they never start on
  their own — the manager's "event" command (and the game's own console
  command) can still start any of them deliberately.

Both are configured and reloaded live from the manager's Mods tab; this
plugin never needs a server restart to pick up a change.

## Configuration

`BepInEx/config/se.jonasthim.valheimui.gameplay.cfg`:

| Section    | Key              | Default | Range   | Meaning |
|------------|------------------|---------|---------|---------|
| `Autofeed` | `Enabled`        | `false` | —       | Master switch for autofeed. |
| `Autofeed` | `SignPrefix`     | `feed`  | —       | Case-insensitive prefix of a sign's text that marks a chest as a feed chest. Empty marks every chest in range. |
| `Autofeed` | `SignRadius`     | `2.0`   | 0.5–6   | Metres between the sign and the chest it marks. |
| `Autofeed` | `Range`          | `20`    | 1–100   | Metres between a feed chest and the animal it can feed. |
| `Autofeed` | `IntervalSeconds`| `30`    | 5–600   | Seconds between scans for hungry animals. |
| `Raids`    | `Enabled`        | `true`  | —       | Master switch for the raid filter. |
| `Raids`    | `Disabled`       | (empty) | —       | Comma-separated random event names that never start by themselves, e.g. `army_eikthyr,wolves`. The event command still starts them. |

The manager writes this file atomically; the plugin notices the change and
calls `Config.Reload()` on the main thread about half a second later — no
restart.

## The sign convention

Vanilla Valheim has no way to name a chest, so this plugin reuses a **sign**:
place a sign within `SignRadius` metres of a chest with text starting with
`SignPrefix` (case-insensitive), for example a sign reading `feed` next to a
chest full of berries and mushrooms next to the wolf pen. Any unmodded client
can place that sign — nothing to install for players. Leave `SignPrefix`
empty to treat every chest within range as a feed chest.

## Raids

Random events are picked and started on the server only. Disabling a raid
here stops it from being picked on its own; it does not remove the event or
change its chance of being chosen among the others, and it never blocks the
manager (or an operator typing `event <name>` at the console) from starting
it on demand.

## Operated from the manager

Installed, updated and configured from the manager's **Mods** tab (the
Gameplay card). The autofeed logic itself — the scan loop that actually
reads chests and feeds animals — ships in a later version; this release
binds and reloads its configuration and enforces the raid filter.

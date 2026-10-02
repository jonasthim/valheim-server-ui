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
| `Autofeed` | `Debug`          | `false` | —       | Logs each individual feed (animal prefab, chest prefab, distance). Verbose; off unless you're diagnosing autofeed. Added automatically if missing — it does not need to be in a hand-written cfg. |
| `Raids`    | `Enabled`        | `true`  | —       | Master switch for the raid filter. |
| `Raids`    | `Disabled`       | (empty) | —       | Comma-separated random event names that never start by themselves, e.g. `army_eikthyr,wolves`. The event command still starts them. |

The manager writes this file atomically; the plugin notices the change and
calls `Config.Reload()` on the main thread about half a second later — no
restart. Change detection uses a `FileSystemWatcher`, backed by a polling
fallback (stats the file at most every 2 s) since the watcher is known to
miss events on some Mono/Linux setups; the server log line says which one
fired (`config reloaded (watcher)` vs `config reloaded (polled)`), so a
polled-only reload for every change is a sign to check the watcher.

## The sign convention

Vanilla Valheim has no way to name a chest, so this plugin reuses a **sign**:
place a sign within `SignRadius` metres of a chest with text starting with
`SignPrefix` (case-insensitive), for example a sign reading `feed` next to a
chest full of berries and mushrooms next to the wolf pen. Any unmodded client
can place that sign — nothing to install for players. Leave `SignPrefix`
empty to treat every chest within range as a feed chest.

## How the scan works

Tamed animals near players are simulated on the players' own clients, so the
server cannot call game logic on them directly. Instead, every
`IntervalSeconds` the plugin scans the world's ZDOs — the data the server
owns the authoritative copy of and syncs to every connected peer — in slices
of at most 2 ms per game tick, so a large world never causes a stutter:

1. **Scan** buckets every object by its prefab's components: signs with
   non-empty text, chests (`Container`) that are not currently open
   (`InUse`), and tamed animals whose prefab has both a `Tameable` and a
   `MonsterAI` with at least one consumable food item.
2. **Match** narrows chests to "feed chests" (within `SignRadius` of a
   matching sign, or every chest when `SignPrefix` is empty) and tames to
   "hungry" ones.
3. **Feed**: for each hungry animal, the *nearest* feed chest within `Range`
   that still holds a matching food item loses one unit of it; the animal's
   feeding timer resets so the game stops showing it as hungry and breeding
   can resume. A chest's contents are loaded once and saved once per scan
   even if several animals eat from it in the same pass. At most 200 feeds
   happen per scan, and the server never takes ownership of a chest or
   animal to do this — if the owning client writes the same ZDO in the same
   tick, the feed is lost and simply retried on the next scan. Every write
   here also jumps the ZDO's `DataRevision` ahead by 1000 (on top of the +1
   the game's own `ZDO.Set` already applies): the client that owns a wandering
   animal keeps bumping its own local revision every tick, and the game only
   accepts an incoming update when its revision is strictly greater than the
   peer's local one, so a plain +1 from the server is routinely ignored by
   the very client the animal is simulated on. The owning client just resumes
   counting up from the new value afterwards; this has no visible effect.

**"Hungry"** uses the game's own rule (`Tameable.IsHungry()`): the time since
the animal's last feeding exceeds its `m_fedDuration` (30 s for most vanilla
animals). The plugin reads that timer straight from the ZDO
(`TameLastFeeding`) rather than from the live component, since most tamed
animals in the world are not loaded as game objects on the server.

**Limits:** the world is scanned at most once every `IntervalSeconds`; one
item is removed per hungry animal per scan (an animal that is still hungry
next scan gets fed again then); chests currently open by a player (`InUse`)
are skipped that scan; a chest or animal whose prefab is unrecognized is
never touched.

## Raids

Random events are picked and started on the server only. Disabling a raid
here stops it from being picked on its own; it does not remove the event or
change its chance of being chosen among the others, and it never blocks the
manager (or an operator typing `event <name>` at the console) from starting
it on demand.

## Validation protocol (spike-grade until run in a real game)

On a non-production instance or a test world: place a chest with a sign
reading "feed" within 2 m, put 10 carrots in it, keep two tamed boars within
20 m, then turn Autofeed on from the manager. Within 30–60 s you should see:

- a console line `autofeed: fed …`;
- the chest down to 8 carrots when a player opens it;
- the boars no longer showing "Hungry", and able to show "Happy"/breed;
- a boar 40 m away still hungry;
- a chest with a sign reading "food" (no `feed` prefix) left untouched;
- turning the switch off stops feeding immediately, no restart needed.

If anything duplicates items, or a chest's contents revert for a connected
player, the ZDO write was clobbered by its owning client in the same tick;
the documented fallback (not implemented by this card) is for the plugin to
briefly take ownership of the two ZDOs before writing
(`zdo.SetOwner(ZDOMan.GetSessionID())`, write, release next frame).

## Operated from the manager

Installed, updated and configured from the manager's **Mods** tab (the
Gameplay card). This release binds and reloads its configuration, enforces
the raid filter, and runs the autofeed scan loop above.

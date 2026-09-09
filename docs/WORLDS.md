# Worlds

A **world** is a Valheim save (`<name>.db` + `<name>.fwl`). An **instance** is a
server process with its own settings, port, mods and player lists. One instance
plays one world at a time (its *active world*) and can keep any number of other
worlds on disk. Everything on this page acts on world files only; it never
creates, deletes or reconfigures an instance.

Files live under the instance's save directory:

```
/var/lib/valheim/instances/<instance>/save/worlds_local/
  <name>.fwl        world metadata: name and seed, written when the world is created
  <name>.db         the world itself, written at every save
  <name>.db.old     Valheim's previous copy
  <name>_backup_auto-<timestamp>.*   Valheim's own rolling copies (-backups); hidden in the UI
```

Player characters are not part of a world; Valheim keeps them on each player's
own machine. Nothing here affects them.

## When a world appears and when it is saved

Valheim writes the `.fwl` the moment a world is generated and the `.db` only at
the first save. Saves happen every save interval (30 minutes by default, set in
Config) and when the server stops. A brand-new world therefore shows **Not saved
yet** in the Worlds tab until its first save; until then there is nothing to back
up or download.

## Actions in the Worlds tab

| Action | What happens | Server impact |
|--------|--------------|---------------|
| **Make active** | Sets the instance's world name to this world. | Takes effect on the next restart; the UI shows a restart banner. |
| **New world** (Config → World name, then restart) | Type a name that does not exist yet and restart. Valheim generates it with a random seed; the previous world stays on disk and stays listed. | One restart. |
| **Regenerate** (active world only) | Takes a backup, deletes the world's files, and lets Valheim generate a fresh world with the **same name and a new random seed**. | The instance is stopped for the swap and started again if it was running; players are disconnected for the minute or two generation takes. Instance settings, mods and lists are untouched. |
| **Delete** (inactive worlds only) | Removes the world's files, including Valheim's rolling copies. | None; the running world is never the target. |
| **Download** | Zip with the `.db` and `.fwl`. | None. |
| **Upload** | Import a `.db`+`.fwl` pair or a zip. Refuses to overwrite the active world while the server runs. | None until you make it active and restart. |

Regenerate and Delete open a confirmation dialog that describes the effect and
only enables the button after you type the world name exactly. Regenerate
offers a "stop, regenerate, start again" checkbox when the server is running
and is refused otherwise. The safety backup it takes is stored as a *manual*
backup, which retention never deletes.

Valheim has no server-side way to choose a seed. To play a specific seed, create
the world in the game client with that seed, then upload its `.db` and `.fwl`.

## Backups vs worlds

The **Backups** tab stores zips of a world plus the admin, ban and permitted
lists, with a retention policy for scheduled ones. **Restore** brings a backup
back into place (taking a pre-restore backup first) and makes that world active.
The Worlds tab is for the live files; the Backups tab is for copies over time.

## Recovering a regenerated world

Regenerate keeps a backup named `<instance>-<world>-<timestamp>-manual.zip`.
Backups tab → that entry → Restore. The instance must be stopped, or tick the
stop-and-start option in the dialog.

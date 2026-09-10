# Architecture Decision Records

Short form. Newest at the bottom. Do not delete superseded records; mark them.

## ADR-001 Native SteamCMD + systemd instead of Docker
Owner preference. Simplest ops on a bare host, direct access to save files and logs,
no container/systemd impedance mismatch for mods (BepInEx lives inside the game dir).

## ADR-002 Go single binary with embedded React build
One static artifact (`CGO_ENABLED=0`, `modernc.org/sqlite`), trivial install/upgrade,
low idle footprint next to a game server that already wants the RAM.

## ADR-003 Per-instance game install (not shared)
BepInEx and plugins are installed inside the game directory, so a shared install
would force every instance to run the same mod set. ~1.2 GB per instance is accepted.

## ADR-004 Manager is unprivileged; root only through `unitctl`
Requested least-privilege model. Options weighed:
- polkit JS rule scoped to `valheim@*.service`: not available on Ubuntu 22.04 (polkit 0.105).
- `systemd --user` units with linger: zero root, but fragile session-bus plumbing when
  the manager itself is a system service; harder to debug for operators.
- **Chosen**: a fixed system template unit installed once by root, plus a 15-line
  root-owned wrapper that whitelists actions and validates the instance id, granted via
  a single sudoers line. Portable to every systemd distro and auditable at a glance.
Status reads use `systemctl show`, which needs no privileges.

## ADR-005 Launcher is the manager binary (`valheim-ui launch`), driven by `launch.json`
Rendering the Valheim command line in Go and `execve`-ing avoids shell quoting bugs
with server names/passwords, keeps `KillSignal=SIGINT` delivered to the game process,
and makes the argument builder unit-testable. BepInEx env vars are read from the pack's
own `start_server_bepinex.sh` because their names changed between pack versions.

## ADR-006 Console log via `StandardOutput=append:` file, not journald
Avoids `systemd-journal` group membership and journalctl parsing; gives a plain file the
manager can tail, rotate (only while the unit is inactive) and offer for download.

## ADR-007 SSE instead of WebSockets
All realtime traffic is server→client (logs, status, job progress). SSE works through
every reverse proxy without upgrade config and needs no client library.

## ADR-008 SQLite + JSON columns for option-heavy objects
Single-host app, one writer. Instance config, settings and job summaries are JSON
validated in Go; queried fields are real columns. Migrations via goose embedded SQL.

## ADR-009 Contract-first API
`docs/openapi.yaml` is written before handlers and frontend. The frontend generates
its types from it (`openapi-typescript`), so backend and frontend work can proceed in
parallel by less capable implementers with a shared source of truth.

## ADR-010 Mantine 9 as the component library
Batteries included (forms, tables, modals, notifications, dropzone) reduces bespoke UI
code, which is where inexperienced implementers produce the most bugs.

## ADR-011 Password is required (≥5 chars)
The dedicated server aborts with "Password too short" for shorter/empty passwords.
The UI enforces Valheim's rule and the "password must not appear in the server name" rule.

## ADR-012 Scheduled restarts cannot warn players
Valheim has no server console, RCON or broadcast. Mitigation is `only_when_empty`.
Announcements would require a BepInEx mod and are out of scope for v1.

## ADR-013 OpenAPI type generator runs outside the dependency tree
`openapi-typescript` 7.x depends on `@redocly/openapi-core` 1.x, which pins a `js-yaml`
line with an open advisory; 2.x of the core is incompatible with the generator. Rather
than carry a known CVE in `package-lock.json`, `web/src/api/schema.d.ts` is committed
and regenerated with `npx --yes openapi-typescript@7.13.0` (`make gen`); CI regenerates
and fails on drift. Revisit when a generator release moves to the patched core.

## ADR-014 First-run setup wizard, no seeded credentials
No default admin password ever exists. While the users table is empty the SPA routes to
`/setup`, where the first administrator is created through `POST /api/v1/auth/setup`;
the endpoint disappears (404 `setup_done`) after the first user. Break-glass afterwards
is `valheim-ui admin reset-password` on the host.

## ADR-015 Releases are cut from a VERSION file on main
Publishing a release requires only a normal push to `main`: CI compares `v$(cat VERSION)`
with the existing tags and, when the tag is new, creates it and publishes the GitHub
release from that commit. Rationale: the release must be reproducible by anyone (or any
automation) that can push a branch, without depending on tag-push or workflow-dispatch
permissions that some tooling and hosted environments lack. Tag pushes and manual
dispatch remain as escape hatches; the tag CI creates uses `GITHUB_TOKEN`, so it does not
trigger a second run.

## ADR-016 One design system, Mantine only, dark first
The UI follows docs/DESIGN.md: a Mantine theme (`web/src/theme/theme.ts`) with Valheim accent
colours on cool neutrals, scheme tokens exposed as CSS variables, and a small set of shared
primitives in `web/src/ui/` (PageHeader, StatTile, StatusPill, SectionCard, EmptyState). Pages
compose those primitives instead of styling ad hoc, so the product stays coherent as features
are added by different people or agents. Dark is the default because the audience runs game
servers at night; light is a full peer, not an afterthought. No fonts or assets are fetched at
runtime: the single binary keeps working offline.

## ADR-017 Audit entries carry a field-level diff
Update endpoints record `details.changes` (path, from, to) computed from the JSON form of the
object before and after the change, with secrets masked. Rationale: an audit log that stores
whole objects cannot answer "what did this edit change?", which is the question operators ask.

## ADR-018 The manager binary is root-owned; upgrades go through the sudo wrapper
Mods run inside the game process as the same `valheim` user as the manager, so a
`valheim`-writable manager binary let a malicious mod replace the administrative UI and
persist across reboots and upgrades. `/var/lib/valheim/bin` is now root-owned; the manager
stages a verified download in `/var/lib/valheim/staging` and `unitctl apply-upgrade <tag>`
re-verifies it against the digest published in the release's `SHA256SUMS` before installing
it. The one-command upgrade story is unchanged; a local compromise can no longer install an
unpublished binary. Root never executes the managed binary (the installer reads its version
as `valheim`). Installs that predate the layout keep working in the old mode until the
installer is re-run.

## ADR-019 Game units are sandboxed; the manager unit keeps only sudo
`valheim@.service` runs with `NoNewPrivileges`, `ProtectSystem=strict`, `ProtectHome`,
`PrivateTmp`, an empty capability set, address-family and namespace restrictions and
`UMask=0027`: a mod gets the instance tree, HOME and network sockets, nothing else, and
cannot call sudo. `valheim-ui.service` gets the same set minus `NoNewPrivileges` and
`RestrictSUIDSGID`, which the setuid `sudo` used for `unitctl` needs. `SystemCallFilter`
and `MemoryDenyWriteExecute` are deliberately left out: the Unity/Mono runtime JITs and
the syscall surface of the game is not something this project can vouch for.

## ADR-020 Security review findings are tracked in docs/SECURITY.md
Every finding from the September 2026 review, its severity, status and the accepted
residual risks live in one document so the next review starts from the last one.

## ADR-021 Windows support runs the direct supervisor behind a launcher proxy

**Context.** Windows has no systemd, no `exec`, and no signals: a dedicated
server is asked to save and exit with a console Ctrl+C, which only a process
attached to the same console can generate, and a service has no console.

**Decision.** One binary, one code base, platform files behind build tags.
On Windows the manager is a Windows service (`x/sys/windows/svc`, already an
indirect dependency) using the existing `direct` supervisor; `launch` does not
exec but stays alive as a proxy that spawns `valheim_server.exe` on a hidden
console inside a kill-on-close job object and turns `stop`/`kill` lines on its
stdin into `CTRL_C_EVENT` / termination. Doorstop is switched through its
command-line options because it reads an ini rather than the environment on
Windows. Restart after self-upgrade relies on SCM recovery actions. Defaults
move to `%ProgramData%\valheim-ui`; `install.ps1` replaces `install.sh`.

**Consequences.** Game processes are children of the service (stopping the
manager stops them, autostart is re-applied at service start) and run under
the same account as the manager, so the Linux isolation (root-owned binary,
sandboxed game units) has no Windows equivalent yet; SECURITY.md records this.
Metrics come from Win32 counters (no load average). Tests use a Go fake game
server (`tools/fake-server`) so the launcher and supervisor suites run on a
Windows CI runner; the real `valheim_server.exe` has not been exercised in CI.

## ADR-022 Our own server plugin instead of RCON

**Context.** Vanilla Valheim has no RCON or command channel; third-party
RCON mods give a command pipe and nothing else. The UI wants live players
with positions, world state and admin actions, and eventually a live map.

**Decision.** Ship a server-only BepInEx plugin of our own (`plugin/`) with a
loopback HTTP API and a per-instance token written by the manager, installed
automatically with BepInEx and versioned with the manager release. A fixed
set of commands mapped to public game APIs (save, kick, ban, unban,
broadcast via the `ShowMessage` RPC) rather than a raw console. CI compiles
it against the real game assemblies fetched with anonymous SteamCMD; no game
files are committed.

**Consequences.** Players need nothing installed; crossplay unaffected. The
plugin depends on public game APIs, so patches can break it and the fix rides
the next release (the UI shows the update). Loopback plus a token keeps the
surface local; commands are audited. Runtime behaviour against the real
server is verified on a real host, not in CI.

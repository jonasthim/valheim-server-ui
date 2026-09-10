<h1 align="center">Valheim Server UI</h1>

<p align="center">
  Run, configure, back up and mod <b>Valheim dedicated servers</b> from a web browser, on <b>Linux or Windows</b>.<br/>
  One static binary. Native SteamCMD installs. Supervised instances (systemd or a Windows service). Multi-user with SSO.
</p>

<p align="center">
  <img src="docs/screenshots/dashboard.png" alt="Dashboard" width="900"/>
</p>

---

## Why

Running a Valheim server means juggling SteamCMD, a start script or batch file
full of quoted flags, `adminlist.txt`, world files that need backing up, BepInEx
plugins and their `.cfg` files, and a restart every time Iron Gate ships a patch.
This project puts all of that behind a clean web UI with proper forms, live logs
and an audit trail, on a Linux box or a Windows machine, without turning your
server into a Docker puzzle.

## Features

| Area | What you get |
|------|--------------|
| **Instances** | Several isolated servers per host, each with its own game install, world folder, ports and process (a systemd unit on Linux, a supervised child of the service on Windows). Start, stop, restart, autostart. |
| **Configuration** | Real form controls for every server option: name, world, password, port, public, crossplay, difficulty preset, every combat/death/resource/raid/portal modifier, world keys, save interval, Valheim's own rolling saves. Validation with the same rules Valheim enforces. |
| **Live console** | Streams the server log in real time with filter, follow, download, and highlighting for ready/join/leave/save events. |
| **Players** | Online players (via the query port and the log), a history of everyone who ever joined, and editors for the admin, banned and permitted lists. |
| **Backups & worlds** | One-click or scheduled world backups, retention policy, restore with an automatic safety backup, world upload/download, switch the active world. |
| **Mods** | Install BepInEx, browse and install from Thunderstore with dependency resolution, upload zips or DLLs, enable/disable/update/uninstall, and edit plugin `.cfg` files with typed inputs. |
| **Live world (agent)** | Our own server plugin, installed with BepInEx: day, in-game clock, weather, world keys and players with positions straight from the running server, plus save-now, kick and an in-game broadcast to all players. Nothing to install for players. |
| **Live map** | The whole world rendered from the seed on the server (no fog), with players moving live, portals and their tags, ships, carts, tombstones, beds and boss locations as layers. Pan, zoom, re-render at up to 4096 px. |
| **Schedules** | Cron-based restarts, backups and Steam update checks with a "only when nobody is online" switch. |
| **Updates** | Detects new Valheim builds on Steam and updates with an optional pre-update backup. |
| **Self-upgrade** | The manager polls GitHub releases, shows what's new, and upgrades itself from the UI with a verified download, atomic swap and rollback. On Linux game servers keep running while it restarts; on Windows they are stopped cleanly and autostarted again. |
| **Users & SSO** | First-run wizard creates the admin. Local accounts plus one OIDC provider (Authelia, Keycloak, Authentik, Google…) with group-to-role mapping. Roles: viewer, operator, admin. |
| **Operations** | Job queue with live logs for every long operation, an audit log that records exactly which fields each edit changed, live CPU and memory for the host and for each game server, disk usage and SteamCMD health on the dashboard. |
| **Security** | The manager runs unprivileged; on Linux its only path to root is a small sudo wrapper that validates its arguments, on Windows it is a service under its own virtual account. Argon2id passwords, hardened session cookies, CSRF guard, no default credentials. |

## Screenshots

A dark-first, modern interface with Valheim accents; light mode is one click away in the header.

<p align="center"><img src="docs/screenshots/dashboard.png" width="900" alt="Dashboard"/></p>

<table>
  <tr>
    <td align="center"><b>Instance overview</b><br/><img src="docs/screenshots/overview.png" width="440"/></td>
    <td align="center"><b>Live console</b><br/><img src="docs/screenshots/console.png" width="440"/></td>
  </tr>
  <tr>
    <td align="center"><b>Server configuration</b><br/><img src="docs/screenshots/config.png" width="440"/></td>
    <td align="center"><b>Players and lists</b><br/><img src="docs/screenshots/players.png" width="440"/></td>
  </tr>
  <tr>
    <td align="center"><b>Mods and config editor</b><br/><img src="docs/screenshots/mods.png" width="440"/></td>
    <td align="center"><b>Audit log with field-level diffs</b><br/><img src="docs/screenshots/audit.png" width="440"/></td>
  </tr>
  <tr>
    <td align="center"><b>Backups</b><br/><img src="docs/screenshots/backups.png" width="440"/></td>
    <td align="center"><b>Schedules</b><br/><img src="docs/screenshots/schedules.png" width="440"/></td>
  </tr>
  <tr>
    <td align="center"><b>Jobs</b><br/><img src="docs/screenshots/jobs.png" width="440"/></td>
    <td align="center"><b>Users</b><br/><img src="docs/screenshots/users.png" width="440"/></td>
  </tr>
  <tr>
    <td align="center"><b>Settings and SSO</b><br/><img src="docs/screenshots/settings.png" width="440"/></td>
    <td align="center"><b>Login</b><br/><img src="docs/screenshots/login.png" width="440"/></td>
  </tr>
</table>

## Install

Both platforms get one installer command, a checksum-verified binary, SteamCMD,
and a service listening on `127.0.0.1:8080`. Re-running the installer upgrades in
place and never touches your data. Budget about 2 GB of disk per instance plus
room for backups.

### Linux

Requirements: Debian 12+ or Ubuntu 22.04+ on x86_64 with systemd and root access.

One command, nothing else to download by hand:

```bash
curl -fsSL https://raw.githubusercontent.com/jonasthim/valheim-server-ui/main/deploy/install.sh | sudo bash
```

The installer creates the `valheim` system user and `/var/lib/valheim`, installs
the binary (to `/var/lib/valheim/bin`, symlinked from `/usr/local/bin/valheim-ui`),
the sudo wrapper, the systemd units and SteamCMD, verifies the release's
checksum, then starts `valheim-ui.service`.

Local build instead of a release (installer script and repo checked out already):

```bash
make build && sudo ./deploy/install.sh --binary bin/valheim-ui
```

Before installing or upgrading, see what would change without touching
anything:

```bash
sudo ./deploy/install.sh --check
```

Put a TLS reverse proxy in front for remote access and set `base_url` in
`/etc/valheim-ui/config.yaml` (Caddy and nginx snippets are in the
[runbook](docs/RUNBOOK.md#2-reverse-proxy-and-tls)). For a trusted LAN only,
`sudo ./deploy/install.sh --listen 0.0.0.0:8080` works too.

### Windows

Requirements: Windows Server 2019+ or Windows 10/11 (x64) and an administrator
PowerShell.

```powershell
irm https://raw.githubusercontent.com/jonasthim/valheim-server-ui/main/deploy/install.ps1 -OutFile install.ps1
.\install.ps1
```

The installer registers the Windows service `valheim-ui` (running as the
virtual account `NT SERVICE\valheim-ui`), puts the binary in
`%ProgramFiles%\valheim-ui`, data and `config.yaml` in `%ProgramData%\valheim-ui`,
downloads SteamCMD, verifies the release checksum and starts the service.
`.\install.ps1 -Check` reports without changing anything; `-Uninstall` removes
the service and binary and keeps the data. Game servers run as child processes
of the service and are stopped with a console Ctrl+C, so worlds are saved on
stop exactly as on Linux. Open UDP `port` and `port+1` per instance in Windows
Firewall. The [runbook](docs/RUNBOOK.md#13-windows) lists what differs from
Linux.

For remote access on either platform, put a TLS reverse proxy in front and set
`base_url` in the config file.

## First run

1. Open the UI. Because no users exist yet, it shows the **setup wizard**; create
   the administrator account. There are no default credentials, and the wizard
   disappears once the first account exists.
2. Click **New instance**. Give it a display name, a server name, a world name, a
   password (5 to 32 characters, not contained in the server name) and a port.
   Leave **Download game files now** on.
3. Watch the SteamCMD install under **Jobs** (a couple of minutes on a fast link).
4. Open the instance and press **Start**. The overview turns green when the world
   has loaded; with crossplay on, the join code appears next to the player count.
5. Open UDP ports `port`, `port+1` and `port+2` on your firewall and router.

## How to

**Change server settings.** Instance → Config. Every option is a switch, dropdown,
checkbox or number field with inline help. Saving while the server runs shows a
"restart required" banner; press Restart when convenient.

**Make someone an admin, ban a player.** Instance → Players. Pick a known player
from the history (the UI records everyone who ever joined) or paste a platform id.
Valheim reloads these files live; no restart needed.

**Back up and restore.** Instance → Backups. "Back up now" zips the world and the
player lists. Restore stops the server if you ask it to, takes a safety backup
first, restores, and starts it again. Manual and uploaded backups are never
auto-deleted; scheduled ones follow the retention policy in Config.

**Manage worlds.** Instance → Worlds. Switch the active world, start a new one
(Config → World name → restart; the old one stays on disk), regenerate the
active world with a new seed (backup first, typed confirmation), delete inactive
ones, upload or download saves. Worlds are files inside an instance; none of
this touches the instance itself. Details: [docs/WORLDS.md](docs/WORLDS.md).

**Install mods.** Instance → Mods → Install BepInEx (the server must be stopped;
the UI can stop and restart it for you). Then Browse Thunderstore, pick a mod and
its version; dependencies are resolved and installed automatically and the plan
is printed in the job log. Plugin `.cfg` files show up below as typed forms.

**See and steer the live world.** Installing BepInEx also installs the
Valheim UI Agent, the manager's own server plugin. The Overview then shows a
World card (day, clock, weather, players, world keys) with *Save world* and
*Broadcast*, the Players tab gets a *Kick* button, and the **Map** tab shows
the world rendered from its seed with players moving live and portals, ships,
tombstones and boss locations as layers. Everything runs over loopback with a
per-instance token; players need nothing. Details:
[docs/ARCHITECTURE.md §20](docs/ARCHITECTURE.md#20-valheim-ui-agent-server-plugin).

**Keep the server up to date.** The overview shows when Steam has a newer build.
"Update now" stops, backs up, updates and restarts. For hands-off operation add a
schedule of kind *update* with "only when empty" on.

**Upgrade Valheim Server UI itself.** The dashboard and Settings → Application
show when a new release is out, with its release notes. Press Upgrade: the
manager downloads the release, verifies its checksum, sanity-runs the new
binary, swaps it in atomically and restarts within seconds; the page reconnects
on the new version. On Linux your game servers are separate systemd units and
keep running throughout; on Windows they belong to the service, so they are
saved and stopped first and autostarted again afterwards. Turn on *auto-upgrade*
to have this happen automatically when nobody is online. Rollback on the host:
`valheim-ui self-upgrade --rollback`, then restart the service.

**Schedule restarts.** Instance → Schedules → New schedule, kind *restart*, pick a
preset such as "Daily at 04:00". Valheim has no way to warn players, so keep
"only when empty" on unless your players know the routine.

**Add users or SSO.** Users → Create user (viewer, operator or admin). For single
sign-on, Settings → Authentication → OIDC: issuer URL, client id and secret, map
your groups to roles, press "Test connection", save. The redirect URI to register
at the provider is shown in the form. Keep one local admin until SSO is proven.
Provider recipes and troubleshooting: [docs/OIDC.md](docs/OIDC.md).

**Run several servers.** Create another instance with a different port range
(each uses three consecutive UDP ports). Each instance has its own game install,
so mod sets can differ.

More operational detail, including upgrades, host-level backups and
troubleshooting, is in [docs/RUNBOOK.md](docs/RUNBOOK.md); single sign-on setup
for Authelia, Keycloak, Authentik, Pocket ID, Google and Entra ID is in
[docs/OIDC.md](docs/OIDC.md).

## Roles

| Role | Can |
|------|-----|
| viewer | See everything except secrets (passwords are masked). |
| operator | Start/stop/configure instances, players, backups, worlds, mods, schedules. |
| admin | Everything, plus create/delete instances, manage users and settings, read the audit log. |

## How it works

Linux:

```
Browser ──HTTPS (your proxy)──► valheim-ui (Go, unprivileged)
                                   │ sudo unitctl start|stop|restart <id>   (only root path)
                                   ▼
                           systemd valheim@<id>.service
                                   │ ExecStart: valheim-ui launch --instance <id>
                                   ▼
                           valheim_server.x86_64  ── stdout ──► console.log ──► live UI
```

Windows:

```
Browser ──HTTPS (your proxy)──► valheim-ui service (NT SERVICE\valheim-ui)
                                   │ spawns valheim-ui launch --instance <id>  (hidden console, job object)
                                   ▼
                           launch proxy ── "stop" on stdin ──► console Ctrl+C
                                   │
                                   ▼
                           valheim_server.exe  ── stdout ──► console.log ──► live UI
```

The manager renders each instance's command line into `launch.json`; the
`launch` subcommand starts the real server binary with the right environment
(on Linux it execs it, with the BepInEx doorstop variables read from the pack's
own start script; on Windows it stays in front of the game as a proxy and
drives Doorstop through its command-line flags). The manager tails the console
log for readiness, join codes and players, and polls the Steam query port for
the authoritative player count. Everything else (SteamCMD, Thunderstore
downloads, backups) runs as tracked jobs with streamed logs. Full design:
[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

## Development

```bash
make deps         # go mod download + npm ci
make build        # frontend build + Go build → bin/valheim-ui
make check        # vet, golangci-lint, go test -race, typecheck, lint
make e2e          # Playwright suite against a fake game server
make dev-backend  # backend on :8080 (direct supervisor + fake server, no systemd needed)
make dev-web      # Vite dev server on :5173 with API proxy
make build-go-windows  # cross-compile bin/valheim-ui.exe
```

Requirements: Go 1.26, Node 22. The API contract is `docs/openapi.yaml`; frontend
types are generated from it (`make gen`). Dependencies stay on their latest
majors with zero known vulnerabilities; CI runs `govulncheck` and `npm audit`.
Platform-specific code lives in `_unix.go` / `_windows.go` files; CI vets the
Windows build on Linux and runs the launcher, supervisor, metrics and log
packages natively on a Windows runner against `tools/fake-server`.

Releases are cut by bumping the `VERSION` file on `main`: CI builds, tests and
publishes the GitHub release `v<VERSION>` with the Linux and Windows binaries,
`install.sh`, `install.ps1` and checksums, and running installs pick it up
through the in-app upgrade (see `docs/RUNBOOK.md` §12).

## Security

The threat model, hardening measures and the findings of the latest security review
(with what was fixed and what is accepted) are in [docs/SECURITY.md](docs/SECURITY.md).
In short: unprivileged service user, sandboxed game units, a one-command sudo wrapper,
root-owned binary with checksum-verified upgrades, argon2id passwords, hashed sessions,
CSRF and security headers, and every input validated before it touches the filesystem.
On Windows the game servers share the service's account, so the process isolation is
weaker there; SECURITY.md spells out the difference.

## Status and non-goals

Targets a single host: Linux with systemd, or Windows (since v1.4.0; the
launcher and stop path are exercised in CI against the fake game server, so
please report anything the real `valheim_server.exe` does differently). Not in
scope: Docker-based game runtime, arm64, macOS, Valheim Plus, generic RCON
(the built-in agent covers admin commands), multi-host management, TLS
termination (use a reverse proxy).

## License

MIT

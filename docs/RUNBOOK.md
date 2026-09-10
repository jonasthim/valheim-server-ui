# Runbook

Operations guide for valheim-server-ui on a Linux host. Architecture and design
rationale live in `ARCHITECTURE.md`; this file is the "how do I" companion.

## 1. Install

Requirements: Debian 12+ or Ubuntu 22.04+ on x86_64, systemd, root access, ~2 GB
disk per instance plus backups. For Windows see §13.

One command, nothing else to download by hand:

```bash
curl -fsSL https://raw.githubusercontent.com/jonasthim/valheim-server-ui/main/deploy/install.sh | sudo bash
```

`install.sh` is fully self-contained (`unitctl`, the sudoers drop-in and both
systemd units are embedded in it, see `deploy/install.sh.in` and
`deploy/build-installer.sh`), so the one-liner above is genuinely everything.
Add flags after the script name if you downloaded it first, e.g.
`sudo ./install.sh --listen 0.0.0.0:8080` (only for a trusted LAN) or
`sudo ./install.sh --version v1.2.3` to pin a release.

What the installer does (idempotent, safe to re-run for upgrades):

1. `apt-get install` of the SteamCMD/Valheim runtime libraries.
2. Creates the system user `valheim` with home `/var/lib/valheim`.
3. Downloads the release tarball and `SHA256SUMS` for the requested (default:
   latest) tag, verifies the checksum, and installs the binary to
   `/var/lib/valheim/bin/valheim-ui` (owned `valheim:valheim`, mode 0755),
   symlinking `/usr/local/bin/valheim-ui` to it. On an upgrade the previous
   binary is kept as `/var/lib/valheim/bin/valheim-ui.prev`.
4. Installs the sudo wrapper `/usr/local/lib/valheim-ui/unitctl`,
   `/etc/sudoers.d/valheim-ui` (validated with `visudo -c`), and the units
   `valheim-ui.service` and `valheim@.service`.
5. Writes `/etc/valheim-ui/config.yaml` if absent and downloads SteamCMD.
6. Enables and starts `valheim-ui.service`.

Then open the UI. **The first visit shows the setup wizard, which creates the
administrator account.** No default credentials exist. After that the wizard is gone.

Preview what an install or upgrade would do without changing anything:
`sudo ./install.sh --check` (prints the installed vs. latest/target version and
which files would be created or updated).

Local build instead of a release: `make build` then `sudo ./deploy/install.sh --binary bin/valheim-ui`
(skips download and checksum verification).

Uninstall: see §10.

## 2. Reverse proxy and TLS

The manager listens on `127.0.0.1:8080` by default and expects a TLS-terminating
proxy in front. Set `base_url` in `/etc/valheim-ui/config.yaml` to the public URL
(needed for the OIDC redirect URI and the CSRF origin check) and restart the service.

Caddy:

```
valheim.example.com {
    header Strict-Transport-Security "max-age=31536000; includeSubDomains"
    reverse_proxy 127.0.0.1:8080 {
        flush_interval -1      # required for the live log stream (SSE)
    }
}
```

The manager trusts the last `X-Forwarded-For` element only when the request
comes from loopback, i.e. from this proxy; keep the proxy on the same host (or
put the manager behind one that appends the client address). Set `base_url` in
`/etc/valheim-ui/config.yaml` to the public URL: it enables the Origin check on
state-changing requests and is required for single sign-on.

nginx:

```
server {
    add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;
    listen 443 ssl http2;
    server_name valheim.example.com;
    # ssl_certificate ...; ssl_certificate_key ...;
    client_max_body_size 512m;             # world/backup/mod uploads
    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_buffering off;               # SSE
        proxy_read_timeout 1h;
    }
}
```

The manager trusts `X-Real-IP`/`X-Forwarded-For` only when the direct peer is a
loopback address, so a proxy on the same host is handled; a remote proxy is not.

## 3. Firewall

Each instance uses three UDP ports starting at its configured port (default
2456–2458). Open them in your host firewall and forward them on your router. The
web UI port never needs to be exposed directly.

## 4. Day-to-day

| Task | Where |
|------|-------|
| Create an instance | Dashboard → New instance. Leave "Download game files now" on; the install job streams SteamCMD output under Jobs. |
| Start/stop/restart | Dashboard card or the instance Overview tab. |
| Change server name, password, world, modifiers | Instance → Config. Changes take effect on the next restart; the UI shows a "restart required" banner. |
| Watch the console | Instance → Console (live, filterable, downloadable). |
| Admins / bans / allow-list | Instance → Players. Valheim reloads the list files live. |
| Backups | Instance → Backups: manual, scheduled, restore, upload/download. Retention is per instance in Config. |
| Worlds | Instance → Worlds: switch the active world, regenerate it with a new seed, delete inactive ones, upload/download saves. World files only, the instance is untouched. See [WORLDS.md](WORLDS.md). |
| Mods | Instance → Mods: install BepInEx, browse Thunderstore, upload zips, edit `.cfg` files. |
| Game updates | Overview → Check for updates / Update now, or a schedule of kind `update`. |
| Users and roles | Users (admin only). Roles: viewer < operator < admin. |
| SSO | Settings → Authentication → OIDC. |
| Audit trail | Audit log (admin only). |

## 5. Service management on the host

```bash
systemctl status valheim-ui               # the manager
journalctl -u valheim-ui -f               # manager logs
systemctl status valheim@main             # one game instance (id "main")
tail -f /var/lib/valheim/instances/main/logs/console.log
sudo -u valheim /usr/local/bin/valheim-ui admin list-users
sudo -u valheim /usr/local/bin/valheim-ui admin reset-password --username admin
```

The manager never needs root: game units are controlled through
`/usr/local/lib/valheim-ui/unitctl`, the only command granted in sudoers. You can
run `sudo -u valheim sudo -n /usr/local/lib/valheim-ui/unitctl restart main` to
confirm the grant works.

Host paths for the binary itself: the real executable lives at
`/var/lib/valheim/bin/valheim-ui`, owned `root:root` mode 0755 in a root-owned
directory, so neither the manager nor a game process (which runs mods as the
same `valheim` user) can rewrite it. `/usr/local/bin/valheim-ui` is a symlink to
it, which is what `ExecStart=` and the commands above actually run. Upgrades go
through the sudo wrapper: the manager stages a verified download under
`/var/lib/valheim/staging` and `unitctl apply-upgrade <tag>` re-checks it against
the release's published checksum before installing it (§11).
`/var/lib/valheim/bin/valheim-ui.prev` is the previous binary, kept for rollback.

Always run the CLI as the service user, never as root:
`sudo -u valheim /usr/local/bin/valheim-ui admin ...`. The installer follows the
same rule (`--check` runs the binary as `valheim`).

## 6. Upgrade

Any of the following work; all three end with the manager restarting on the
new binary while game instances keep running (systemd, not the manager,
supervises them, and `valheim@*.service` units are never touched by a manager
restart — see §11):

- From the UI: **Settings → Application → Check for updates**, then **Upgrade**.
- Re-run the installer: `curl -fsSL https://raw.githubusercontent.com/jonasthim/valheim-server-ui/main/deploy/install.sh | sudo bash`
  (or `sudo ./install.sh --version vX.Y.Z` / `--binary PATH` if you have the
  script and a specific build already).
- On the host directly: `sudo -u valheim /usr/local/bin/valheim-ui self-upgrade --apply`.
  To undo a bad upgrade: `sudo -u valheim /usr/local/bin/valheim-ui self-upgrade --rollback`
  (restores `valheim-ui.prev` and restarts the service). Upgrading to an older
  release through the UI or `--apply` is refused; rollback is the way back.

**Upgrading from 1.2.x or older:** those installs kept the binary writable by
the `valheim` user. The in-app upgrade to the current release still works, and
afterwards the dashboard reports "binary directory not writable ... re-run
install.sh". Run the install one-liner once: it moves `bin/` to root ownership,
installs the new `unitctl` and the hardened units. From then on in-app upgrades
use `unitctl apply-upgrade`.

Database migrations run automatically on start in all cases.

If the `valheim` user existed before the first install with a different home
directory, the installer changes it to `/var/lib/valheim`; the manager and any
running instances are stopped briefly for that one `usermod` and started again.

## 7. Backup of the manager itself

Everything lives in `/var/lib/valheim`. For a full host backup, stop the manager
(`systemctl stop valheim-ui`) and copy that directory plus
`/etc/valheim-ui/config.yaml`. For a hot backup of only the manager database:

```bash
sudo -u valheim sqlite3 /var/lib/valheim/manager.db ".backup /tmp/manager-backup.db"
```

World backups made by the UI are plain zips under `instances/<id>/backups/` and
are safe to copy off-host at any time.

## 8. Single sign-on (OIDC)

Full guide with settings reference, provider recipes (Authelia, Keycloak,
Authentik, Pocket ID, Google, Entra ID) and troubleshooting: [OIDC.md](OIDC.md).

Short version: register a confidential client with redirect URI
`https://<base_url>/api/v1/auth/oidc/callback` and scopes
`openid profile email groups`, fill in Settings → Authentication → OIDC, press
Test connection, map your groups to roles, keep one local admin until SSO is
proven.

## 9. Troubleshooting

| Symptom | Check |
|---------|-------|
| Dashboard says SteamCMD is not installed | `/var/lib/valheim/steamcmd/steamcmd.sh` missing: re-run `install.sh` or download SteamCMD there as the `valheim` user. |
| Install job fails with `Disk write failure` | Almost never a full disk. SteamCMD could not write to `$HOME` or the install dir. The units run with `ProtectHome=true` and set `HOME=/var/lib/valheim`; check `systemctl show valheim-ui -p Environment`, `getent passwd valheim` (home must be `/var/lib/valheim`; re-run `install.sh`, which repoints a pre-existing user), and ownership of `/var/lib/valheim`. |
| Install job fails with `0x6`/`0x202`/`0x602` | SteamCMD transient errors; the job retries once. Re-run the install; check disk space (`df -h /var/lib/valheim`). |
| Start fails with "unitctl" or sudo errors | `visudo -cf /etc/sudoers.d/valheim-ui`; confirm `/usr/local/lib/valheim-ui/unitctl` is root-owned 0755; `sudo -u valheim sudo -n /usr/local/lib/valheim-ui/unitctl start <id>`. |
| Console shows `Failed to open plugin: .../libparty.so` or an `ArgumentNullException` during startup | Stock Valheim dedicated-server noise, seen on every install; the server continues to "Game server connected". Not a permissions or sandbox problem. |
| Instance goes to `failed` right after start | `journalctl -u valheim@<id>` and `logs/console.log`. Common causes: port already in use, missing 32-bit libs, password shorter than 5 characters. |
| Players cannot connect | UDP ports not forwarded; server name contains the password (Valheim refuses); wrong crossplay setting for console players. |
| Player count shows "?" / "from log" | The A2S query port (port+1) is not answering yet; names come from the log heuristic until it does. |
| OIDC login fails | Settings → Test connection; ensure `base_url` matches the browser URL exactly; check the provider's redirect URI. |
| Live console stops updating behind a proxy | Buffering: add `proxy_buffering off` (nginx) or `flush_interval -1` (Caddy). |
| "cross-origin request rejected" | `base_url` does not match the host the browser uses. |
| Scheduled restart never fires | Schedules with `only_when_empty` skip while players are online; see the schedule's last result. Valheim cannot warn players, so this is by design. |

## 10. Uninstall

```bash
sudo ./install.sh --uninstall     # removes units, sudoers, binary; keeps /var/lib/valheim
sudo rm -rf /var/lib/valheim /etc/valheim-ui && sudo userdel valheim   # only if you want the data gone
```

`--check` composes with `--uninstall` too: `sudo ./install.sh --uninstall --check`
prints what would be removed without touching anything.

## 11. Self-upgrade

The layout `install.sh` sets up lets the manager upgrade itself without ever
being able to overwrite its own binary:

- The real executable is `/var/lib/valheim/bin/valheim-ui`, owned `root:root`
  mode 0755 in a root-owned directory; `/usr/local/bin/valheim-ui` is only a
  symlink to it. `/var/lib/valheim/staging` is owned by `valheim`.
- Mechanism (`valheim-ui self-upgrade --apply`, and what "Settings →
  Application → Check for updates → Upgrade" calls in the API): the manager,
  as `valheim`, resolves the target release, downloads its tarball and
  `SHA256SUMS`, verifies the checksum, extracts the binary, sanity-runs it
  (`version` must print the expected tag) and moves it to
  `staging/valheim-ui.new`. It then runs `sudo -n unitctl apply-upgrade <tag>`.
  The wrapper, as root, checks that the staged file is a regular ELF file owned
  by `valheim`, downloads the release's `SHA256SUMS` itself and compares the
  raw binary's digest (the release workflow lists it as `valheim-ui`), then
  installs it root-owned: current binary → `valheim-ui.prev`, staged file →
  `valheim-ui` (renames within one directory, so a process that already has the
  old file open is unaffected until the next exec). The manager exits cleanly.
  A process running as `valheim` can stage anything it likes, but only a
  binary published in a release ever gets installed.
- Installs that predate this layout (binary writable by `valheim`, no
  `apply-upgrade` verb in the wrapper) keep the old rename-in-place path until
  the installer is re-run; the dashboard says so in the update card.
- `valheim-ui.service` has `Restart=always` with `RestartSec=3`, so systemd
  immediately restarts the manager, this time running the new binary. There is
  no separate "restart" step to script or forget.
- Game instances are untouched throughout: `valheim@<id>.service` units are
  independent systemd units with no ordering or dependency relationship to
  `valheim-ui.service` (see the comment in `deploy/valheim@.service`), so a
  manager restart never stops, restarts, or otherwise affects a running
  Valheim server.
- Rollback: `valheim-ui self-upgrade --rollback` (via `unitctl
  rollback-upgrade`) restores `/var/lib/valheim/bin/valheim-ui.prev` over the
  current binary and exits, and the same `Restart=always` brings the manager
  back up on the restored build. Only one previous version is kept; roll back
  promptly if an upgrade misbehaves.
- `install.sh` implements the same download-and-verify-and-swap logic for the
  initial install and for installer-driven upgrades, so a fresh `curl | sudo
  bash` run and an in-app self-upgrade produce byte-identical results on disk.

## 12. Releasing

A release is a git tag `vX.Y.Z` plus a GitHub release carrying
`valheim-ui_linux_amd64.tar.gz`, `install.sh`, `deploy.tar.gz` and
`SHA256SUMS`. Both `install.sh` and the in-app self-upgrade (§11) resolve the
latest release from GitHub, so publishing one is all it takes to roll a new
version out to every install.

The normal way to cut one is to bump the `VERSION` file on `main`:

```bash
echo 1.0.2 > VERSION
git commit -am "chore: release v1.0.2"
git push origin main
```

The `version` job in `.github/workflows/ci.yml` reads the file on every push
to `main`; when the tag `v<VERSION>` does not exist yet, the `release` job
(after backend, frontend and e2e pass) builds the binary with that version
string, creates the tag at the pushed commit and publishes the release with
generated notes. A push to `main` whose `VERSION` is already tagged publishes
nothing, so unrelated commits are safe. The two older paths still work:
pushing a `v*` tag by hand releases that tag, and the workflow's manual
"Run workflow" button takes a version to release from any ref.

Installs learn about the release at their next update check (Settings →
Application, hourly by default); press Upgrade there or re-run the install
one-liner on the host.

If the `release` job fails at the publish step with `Resource not accessible
by integration`, the workflow token is not allowed to write releases. Check
Settings → Actions → General → Workflow permissions ("Read and write
permissions"), or add a repository secret `RELEASE_TOKEN` holding a
fine-grained personal access token with *Contents: read and write* on this
repository; the job prefers that secret when present. The built assets are
also kept as the workflow artifact `release-vX.Y.Z` for 30 days, so a failed
publish can be completed by hand with `gh release upload vX.Y.Z dist/*`.


## 13. Windows

Supported: Windows Server 2019+ and Windows 10/11 (x64). Same binary features,
different plumbing: there is no systemd and no sudo wrapper, so the manager runs
as a Windows service and supervises the game processes itself.

Install or upgrade from an elevated PowerShell:

```powershell
irm https://raw.githubusercontent.com/jonasthim/valheim-server-ui/main/deploy/install.ps1 -OutFile install.ps1
.\install.ps1                              # latest release
.\install.ps1 -Version v1.4.0              # pin a release
.\install.ps1 -BaseUrl https://valheim.example.com
.\install.ps1 -Check                       # report, change nothing
.\install.ps1 -Uninstall                   # remove service + binary, keep data
```

What it does (idempotent):

1. Downloads `valheim-ui_windows_amd64.zip` and `SHA256SUMS` for the release
   and verifies the checksum.
2. Installs `%ProgramFiles%\valheim-ui\valheim-ui.exe` (previous binary kept as
   `.prev`, installed tag in `VERSION`), creates `%ProgramData%\valheim-ui` and
   writes `config.yaml` there if absent (`supervisor: direct`).
3. Registers the service `valheim-ui` (`valheim-ui.exe serve --config ...`),
   running as the virtual account `NT SERVICE\valheim-ui`, start type
   Automatic, recovery actions "restart after 5 s" with the failure flag set.
   Grants that account Modify on both directories.
4. Downloads SteamCMD into `%ProgramData%\valheim-ui\steamcmd` (skip with
   `-NoSteamCmd`).
5. Starts the service.

Day to day:

```powershell
Get-Service valheim-ui                       # state
Restart-Service valheim-ui
Get-Content -Wait $env:ProgramData\valheim-ui\manager.log          # manager log
Get-Content -Wait $env:ProgramData\valheim-ui\instances\main\logs\console.log
& "$env:ProgramFiles\valheim-ui\valheim-ui.exe" admin list-users --config $env:ProgramData\valheim-ui\config.yaml
```

Differences from Linux worth knowing:

- **Game processes belong to the service.** Stopping or restarting the manager
  (including a self-upgrade) stops every running instance first; each one gets
  a console Ctrl+C, so it saves its world and exits cleanly. Instances flagged
  *autostart* are started again when the service comes up. On Linux the game
  units are independent of the manager.
- **Stop is a console Ctrl+C**, delivered by the `launch` proxy on the hidden
  console it shares with `valheim_server.exe`; the game cannot outlive the
  proxy (job object). If the server ignores Ctrl+C for 120 s it is killed.
- **Mods:** the BepInEx pack is installed the same way; the launcher passes
  `--doorstop-enabled true/false` so the *Enable BepInEx* switch works without
  editing `doorstop_config.ini`. Mods run as `NT SERVICE\valheim-ui`, the same
  account as the manager (see SECURITY.md).
- **Self-upgrade** swaps `valheim-ui.exe` in place and exits; the SCM recovery
  action restarts the service on the new binary. Rollback:
  `valheim-ui.exe self-upgrade --rollback` from an elevated prompt, then
  `Restart-Service valheim-ui`.
- **No load average** on the Overview host tile (Windows has none); CPU and
  memory come from the Win32 process counters.
- **Firewall:** open UDP `<port>` and `<port>+1` for each instance yourself
  (`New-NetFirewallRule -DisplayName "Valheim main" -Direction Inbound -Protocol UDP -LocalPort 2456-2457 -Action Allow`).
- **Reverse proxy / TLS:** the UI still listens on loopback; put IIS (ARR),
  Caddy or nginx in front and set `base_url`, as in §2.

Windows support is new in v1.4.0. The launcher, stop protocol and metrics are
covered by the Windows CI job against the fake game server; please report
anything the real `valheim_server.exe` does differently.

## 14. Valheim UI Agent (server plugin)

The agent is the manager's own BepInEx plugin. Installing BepInEx from the Mods
tab installs it too; instances that had BepInEx before v1.5.0 get an
**Install agent** button on the Mods tab. It needs nothing on players' machines.

What it gives you: a **World** card on the Overview (day, in-game clock, weather,
players, world keys) with **Save world** and **Broadcast**, a **Kick** button per
online player, and `agent_connected` in the instance status.

How it works on the host:

- The plugin listens on `127.0.0.1:<game port>` over TCP, loopback only. Nothing
  to open in the firewall; the manager talks to it on the same machine.
- Before every start the manager writes
  `instances/<id>/server/BepInEx/config/se.jonasthim.valheimui.agent.cfg` with
  the port and a random token. The token stays in that file and in memory.
- Commands are audited (`agent.command` with target and message).

Troubleshooting:

- **"agent offline" while the server runs:** check that BepInEx is enabled for
  the instance (Mods tab) and that `BepInEx/LogOutput.log` in the server
  directory shows `[Info   :Valheim UI Agent] agent listening on
  http://127.0.0.1:<port>/`. A `could not listen` line means the port is taken;
  set `Port` in the cfg to a free TCP port and restart.
- **`unauthorized` in `last_error`:** the cfg token and the running plugin
  disagree (the file was edited while the server ran). Restart the instance.
- **Update available on the agent card:** a manager upgrade shipped a newer
  plugin; press *Update agent* (stops and restarts the instance).
- **Broadcast shows nothing in-game:** the message uses the raid/sleep banner;
  players in menus or loading screens do not see it.

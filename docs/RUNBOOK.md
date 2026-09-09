# Runbook

Operations guide for valheim-server-ui on a Linux host. Architecture and design
rationale live in `ARCHITECTURE.md`; this file is the "how do I" companion.

## 1. Install

Requirements: Debian 12+ or Ubuntu 22.04+ on x86_64, systemd, root access, ~2 GB
disk per instance plus backups.

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
    reverse_proxy 127.0.0.1:8080 {
        flush_interval -1      # required for the live log stream (SSE)
    }
}
```

nginx:

```
server {
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
`/var/lib/valheim/bin/valheim-ui` (owned `valheim:valheim`, mode 0755, in a
directory of the same ownership/mode so the unprivileged `valheim` user can
replace it); `/usr/local/bin/valheim-ui` is a symlink to it, which is what
`ExecStart=` and the commands above actually run. This split lets the manager
process (running as `valheim`) swap its own binary during a self-upgrade
without needing write access to `/usr/local/bin`. `/var/lib/valheim/bin/valheim-ui.prev`
is the previous binary, kept for rollback (§11).

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
  (restores `valheim-ui.prev` and restarts the service).

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

The layout `install.sh` sets up exists specifically so the manager can upgrade
its own binary without any external tooling:

- The real executable is `/var/lib/valheim/bin/valheim-ui`, owned
  `valheim:valheim` mode 0755, in a directory with the same ownership/mode.
  `/usr/local/bin/valheim-ui` is only a symlink to it. Because the manager
  process runs as `valheim` and owns that file and directory, it can replace
  its own binary while it is running — something it could not do if the
  binary lived directly under root-owned `/usr/local/bin`.
- Mechanism (`valheim-ui self-upgrade --apply`, and what "Settings →
  Application → Check for updates → Upgrade" calls in the API): resolve the
  target release, download its tarball and `SHA256SUMS`, verify the checksum,
  copy the current binary to `valheim-ui.prev`, write the new binary as
  `valheim-ui.new` and rename it over `valheim-ui` (a rename within the same
  directory, so anything that already has the old file open — or execs
  through the `/usr/local/bin` symlink — is unaffected until the next exec),
  then exit cleanly.
- `valheim-ui.service` has `Restart=always` with `RestartSec=3`, so systemd
  immediately restarts the manager, this time running the new binary. There is
  no separate "restart" step to script or forget.
- Game instances are untouched throughout: `valheim@<id>.service` units are
  independent systemd units with no ordering or dependency relationship to
  `valheim-ui.service` (see the comment in `deploy/valheim@.service`), so a
  manager restart never stops, restarts, or otherwise affects a running
  Valheim server.
- Rollback: `valheim-ui self-upgrade --rollback` restores
  `/var/lib/valheim/bin/valheim-ui.prev` over the current binary and exits, and
  the same `Restart=always` brings the manager back up on the restored build.
  Only one previous version is kept; roll back promptly if an upgrade misbehaves.
- `install.sh` implements the same download-and-verify-and-swap logic for the
  initial install and for installer-driven upgrades, so a fresh `curl | sudo
  bash` run and an in-app self-upgrade produce byte-identical results on disk.

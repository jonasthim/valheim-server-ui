# Runbook

Operations guide for valheim-server-ui on a Linux host. Architecture and design
rationale live in `ARCHITECTURE.md`; this file is the "how do I" companion.

## 1. Install

Requirements: Debian 12+ or Ubuntu 22.04+ on x86_64, systemd, root access, ~2 GB
disk per instance plus backups.

```bash
curl -fsSLO https://github.com/jonasthim/valheim-server-ui/releases/latest/download/deploy.tar.gz
tar -xzf deploy.tar.gz && cd deploy
sudo ./install.sh                     # add --listen 0.0.0.0:8080 only for a trusted LAN
```

What the installer does (idempotent, safe to re-run for upgrades):

1. `apt-get install` of the SteamCMD/Valheim runtime libraries.
2. Creates the system user `valheim` with home `/var/lib/valheim`.
3. Installs `/usr/local/bin/valheim-ui`, the sudo wrapper `/usr/local/lib/valheim-ui/unitctl`,
   `/etc/sudoers.d/valheim-ui`, and the units `valheim-ui.service` and `valheim@.service`.
4. Writes `/etc/valheim-ui/config.yaml` if absent and downloads SteamCMD.
5. Enables and starts `valheim-ui.service`.

Then open the UI. **The first visit shows the setup wizard, which creates the
administrator account.** No default credentials exist. After that the wizard is gone.

Local build instead of a release: `make build` then `sudo ./deploy/install.sh --binary bin/valheim-ui`.

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
| Worlds | Instance → Worlds: upload `.db`+`.fwl`, download, switch the active world (restart required). |
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

## 6. Upgrade

Re-run `install.sh` with the new release (or `--binary`). Database migrations run
automatically on start. Game instances keep running across manager upgrades
because systemd, not the manager, supervises them.

## 7. Backup of the manager itself

Everything lives in `/var/lib/valheim`. For a full host backup, stop the manager
(`systemctl stop valheim-ui`) and copy that directory plus
`/etc/valheim-ui/config.yaml`. For a hot backup of only the manager database:

```bash
sudo -u valheim sqlite3 /var/lib/valheim/manager.db ".backup /tmp/manager-backup.db"
```

World backups made by the UI are plain zips under `instances/<id>/backups/` and
are safe to copy off-host at any time.

## 8. OIDC examples

Register a confidential client with redirect URI
`https://<base_url>/api/v1/auth/oidc/callback` (shown read-only in Settings) and
scopes `openid profile email groups`.

Authelia (`configuration.yml`):

```yaml
identity_providers:
  oidc:
    clients:
      - client_id: valheim-ui
        client_secret: '$pbkdf2-sha512$...'
        redirect_uris: [https://valheim.example.com/api/v1/auth/oidc/callback]
        scopes: [openid, profile, email, groups]
        authorization_policy: two_factor
```

Keycloak: create a client (Standard flow, Client authentication on), add a
"Group Membership" mapper named `groups` (full path off) to the client scope, and
map the group names to roles in Settings → OIDC → Role mapping. Set the default
role to `deny` if only mapped groups may log in.

Keep at least one local admin until SSO is proven; `local_login_enabled` can be
switched off afterwards, and `admin reset-password` remains the break-glass path.

## 9. Troubleshooting

| Symptom | Check |
|---------|-------|
| Dashboard says SteamCMD is not installed | `/var/lib/valheim/steamcmd/steamcmd.sh` missing: re-run `install.sh` or download SteamCMD there as the `valheim` user. |
| Install job fails with `0x6`/`0x202`/`0x602` | SteamCMD transient errors; the job retries once. Re-run the install; check disk space (`df -h /var/lib/valheim`). |
| Start fails with "unitctl" or sudo errors | `visudo -cf /etc/sudoers.d/valheim-ui`; confirm `/usr/local/lib/valheim-ui/unitctl` is root-owned 0755; `sudo -u valheim sudo -n /usr/local/lib/valheim-ui/unitctl start <id>`. |
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

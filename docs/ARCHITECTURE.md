# Valheim Server UI — Architecture

Status: v1 design, approved 2026-09-09. Owner: jonasthim.
This document is the source of truth for implementers. If code and this document
disagree, fix one of them in the same PR.

## 1. Goal

A self-hosted web application that installs, configures, runs, backs up and mods
one or more **Valheim dedicated servers** on a single Linux host. It ships as a
single static Go binary with the React frontend embedded, plus a small set of
systemd/sudo files installed by `deploy/install.sh`.

Decisions locked with the owner (see `docs/DECISIONS.md` for rationale):

| Topic            | Decision                                                                 |
|------------------|--------------------------------------------------------------------------|
| Game runtime     | Native install via SteamCMD, each instance a systemd unit `valheim@<id>` |
| Stack            | Go 1.26 backend + React 19/TypeScript/Vite frontend, one binary          |
| Instances        | Multiple per host, fully isolated (own game install, save dir, ports)    |
| Privileges       | Manager runs as unprivileged `valheim` user; narrow sudo wrapper for systemctl |
| Auth             | Multi-user, roles `admin`/`operator`/`viewer`; local accounts **and** one OIDC provider |
| Mods             | BepInEx + Thunderstore browse/install/update with dependency resolution  |
| Backups          | Manager-side zip backups, retention, restore, world upload/download      |
| Scheduling       | Cron-based restarts, backups, update checks/auto-update                  |
| Packaging        | `install.sh` + systemd unit, linux/amd64 only                            |

## 2. System context

```
                 HTTPS (reverse proxy: Caddy/nginx, user-provided)
 Browser  ───────────────────────────────►  valheim-ui  (Go, :8080, user=valheim)
                                              │  │  │  │
             ┌────────────────────────────────┘  │  │  └──────────────► Thunderstore API (HTTPS)
             ▼                                   │  └────────► SteamCMD (subprocess)  ──► Steam CDN
   sudo -n /usr/local/lib/valheim-ui/unitctl     ▼
             │                        /var/lib/valheim/**  (SQLite, instances, backups, cache)
             ▼
   systemd  valheim@<id>.service ──► ExecStart=/usr/local/bin/valheim-ui launch --instance <id>
                                          │  reads instances/<id>/launch.json, execs
                                          ▼
                                 valheim_server.x86_64  (stdout → instances/<id>/logs/console.log)
                                          ▲
                                 UDP <port>, <port+1> (A2S query)  ◄── manager polls A2S
```

There are exactly three trust boundaries:

1. **Browser ↔ manager**: authenticated HTTP API with role checks and audit log.
2. **Manager ↔ root**: only via `unitctl`, a root-owned shell wrapper that accepts
   `start|stop|restart|enable|disable` and an instance id matching
   `^[a-z0-9][a-z0-9-]{0,31}$`. Nothing else is granted in sudoers.
3. **Manager ↔ game process**: the manager never talks to the game directly. It
   writes `launch.json`, asks systemd to start the unit, tails the console log file
   and polls the A2S query port. Valheim has no RCON; there is no command channel.

## 3. On-disk layout

All paths below are relative to `data_dir` (default `/var/lib/valheim`), owned by
`valheim:valheim`, mode 0750. The manager config lives in `/etc/valheim-ui/config.yaml`.

```
/var/lib/valheim/
├── manager.db                      SQLite database (WAL mode)
├── steamcmd/                       SteamCMD install (steamcmd.sh, linux32/...)
├── cache/
│   ├── thunderstore/index.json     cached package index (+ index.etag, index.ts)
│   └── thunderstore/pkgs/<owner>-<name>-<ver>.zip   downloaded packages
├── jobs/<job-id>.log               job output logs
└── instances/<id>/
    ├── launch.json                 rendered launch contract (0600), see §7
    ├── server/                     game install (SteamCMD force_install_dir)
    │   ├── valheim_server.x86_64
    │   ├── steamapps/appmanifest_896660.acf   (buildid lives here)
    │   ├── BepInEx/ doorstop_libs/ start_server_bepinex.sh ...   (when BepInEx installed)
    │   └── BepInEx/plugins/<owner>-<name>/    one folder per managed mod
    ├── save/                       passed as -savedir
    │   ├── worlds_local/<World>.db, <World>.fwl
    │   ├── adminlist.txt bannedlist.txt permittedlist.txt
    ├── backups/<id>-<world>-<UTC timestamp>-<kind>.zip
    └── logs/console.log (+ rotated console-<ts>.log)
```

Instance ids are slugs, immutable, used verbatim in unit names and paths.

## 4. Backend structure (Go module `github.com/jonasthim/valheim-server-ui`)

```
cmd/valheim-ui/main.go        subcommands: serve (default) | launch | admin | migrate | version
internal/
  config/      load /etc/valheim-ui/config.yaml + VALHEIM_UI_* env overrides
  domain/      shared types & enums (Instance, InstanceConfig, Status, Job, User, Role, events)
  db/          sqlite open (modernc, WAL, busy_timeout), goose migrations embedded from db/migrations
  auth/        argon2id passwords, sessions, RBAC middleware, OIDC (go-oidc), setup flow
  api/         chi router, one *_handlers.go per resource, JSON helpers, SSE endpoint
  events/      in-process pub/sub bus feeding SSE
  instance/    instance service: CRUD, validation, path helpers, launch.json rendering, pending-restart tracking
  launcher/    `valheim-ui launch` implementation (reads launch.json, applies BepInEx env, execve)
  supervisor/  Supervisor interface + systemd (sudo unitctl) impl + direct (child process) impl
  logs/        console.log tailer, line parser (players, ready, join code), rotation
  query/       A2S_INFO UDP client (with challenge handling)
  steam/       SteamCMD wrapper: install/update/validate, buildid read, remote buildid check
  jobs/        job runner: per-instance serial queue, log file + event streaming, cancel
  backup/      zip create/restore, retention, world import/export
  scheduler/   robfig/cron driven schedules → jobs
  mods/        BepInEx install/enable, Thunderstore client+cache+search, package installer, cfg parser
  players/     adminlist/bannedlist/permittedlist read/write, known-players registry
  audit/       audit log writes
web/           Vite + React + TypeScript + Mantine, built into web/dist and embedded
deploy/        install.sh, valheim-ui.service, valheim@.service, unitctl, sudoers, config.example.yaml
docs/          this file, openapi.yaml, WORKPLAN.md, DECISIONS.md, RUNBOOK.md
```

### 4.1 Libraries (pinned in go.mod; do not add others without a note in the PR)

| Need         | Library                                         |
|--------------|-------------------------------------------------|
| Router       | `github.com/go-chi/chi/v5`                       |
| SQLite       | `modernc.org/sqlite` (pure Go, CGO_ENABLED=0)    |
| Migrations   | `github.com/pressly/goose/v3` (embedded SQL)     |
| OIDC         | `github.com/coreos/go-oidc/v3`, `golang.org/x/oauth2` |
| Passwords    | `golang.org/x/crypto/argon2`                     |
| Cron         | `github.com/robfig/cron/v3`                      |
| File watch   | `github.com/fsnotify/fsnotify`                   |
| YAML         | `gopkg.in/yaml.v3`                               |
| IDs          | `github.com/google/uuid`                         |
| Testing      | stdlib `testing` + `github.com/stretchr/testify` |
| Logging      | stdlib `log/slog`                                |

Frontend: `react`, `react-router-dom`, `@tanstack/react-query`, `@mantine/core|hooks|form|notifications|modals|dropzone`, `@tabler/icons-react`, `openapi-typescript` (dev, generates `web/src/api/schema.d.ts` from `docs/openapi.yaml`).

## 5. Data model

Authoritative DDL is `internal/db/migrations/00001_init.sql`. Summary:

| Table            | Purpose                                                                                   |
|------------------|-------------------------------------------------------------------------------------------|
| `users`          | id, username (unique, lowercase), display_name, email, password_hash (nullable for OIDC-only), role, disabled, timestamps |
| `user_identities`| (provider, subject) → user_id; links OIDC subjects to users                               |
| `sessions`       | id (sha256 of cookie token), user_id, created/expires/last_seen, ip, user_agent           |
| `settings`       | single-row JSON document (auth/OIDC, update check interval, thunderstore refresh, defaults)|
| `instances`      | id (slug), name, config_json (InstanceConfig), autostart, pending_restart, installed_buildid, latest_buildid, buildid_checked_at, timestamps |
| `jobs`           | id (uuid), type, instance_id (nullable), status, requested_by, created/started/finished, error, summary_json |
| `backups`        | id, instance_id, world_name, kind (manual/scheduled/pre_update/pre_restore/uploaded), filename, size_bytes, note, created_at |
| `schedules`      | id, instance_id, kind (restart/backup/update), cron_expr, enabled, only_when_empty, last_run_at, last_result, last_job_id |
| `mods`           | id, instance_id, source (thunderstore/manual), owner, name, version, enabled, files_json (relative paths), installed_at, updated_at; unique(instance_id, owner, name) |
| `players`        | instance_id, platform_id, name, first_seen_at, last_seen_at, session_count; PK(instance_id, platform_id) |
| `audit_log`      | id, ts, user_id, username, action, instance_id, target, details_json, ip                   |

Settings, instance config and job summaries are JSON columns validated in Go;
this avoids schema churn for option-heavy objects. Everything that is queried or
joined (ids, kinds, timestamps, enabled flags) is a real column.

### 5.1 InstanceConfig (JSON, see `internal/domain/instance.go`)

| Field                 | Type / values                                                                 | Valheim flag            |
|-----------------------|--------------------------------------------------------------------------------|-------------------------|
| `name`                | string 1..64, must not contain the password                                    | `-name`                 |
| `world`               | string `^[A-Za-z0-9_\- ]{1,32}$`                                                | `-world`                |
| `password`            | string, 5..32 chars, required                                                  | `-password`             |
| `port`                | int 1024..65000; instance uses port..port+2; ranges must not overlap on host    | `-port`                 |
| `public`              | bool                                                                           | `-public 0/1`           |
| `crossplay`           | bool                                                                           | `-crossplay`            |
| `preset`              | `""`, normal, casual, easy, hard, hardcore, immersive, hammer                  | `-preset`               |
| `modifiers`           | object: combat∈{veryeasy,easy,hard,veryhard}, deathpenalty∈{casual,veryeasy,easy,hard,hardcore}, resources∈{muchless,less,more,muchmore,most}, raids∈{none,muchless,less,more,muchmore}, portals∈{casual,hard,veryhard}; omit key = default | `-modifier k v` (repeated) |
| `setkeys`             | subset of {nobuildcost, playerevents, passivemobs, nomap, fire}                      | `-setkey k` (repeated)  |
| `save_interval_sec`   | int, default 1800                                                              | `-saveinterval`         |
| `game_backups`        | int, default 4  (Valheim's own rolling saves)                                  | `-backups`              |
| `game_backup_short_sec` | int, default 7200                                                            | `-backupshort`          |
| `game_backup_long_sec`  | int, default 43200                                                           | `-backuplong`           |
| `extra_args`          | []string, advanced, passed verbatim after generated args                       |                         |
| `bepinex_enabled`     | bool (only honoured when BepInEx is installed)                                 | doorstop env            |
| `backup_keep_last`    | int, default 10 (manager backups, non-manual kinds)                            |                         |
| `backup_keep_days`    | int, default 30                                                                |                         |
| `backup_before_update`| bool, default true                                                             |                         |

Always appended: `-savedir <data>/instances/<id>/save -nographics -batchmode`.
Argument order is deterministic (the order of the table) so `launch.json` diffs are stable.

## 6. Process supervision

```go
// internal/supervisor/supervisor.go
type State string // "stopped" | "starting" | "running" | "stopping" | "failed"

type Status struct {
    State      State
    PID        int
    Since      time.Time   // last state change, zero if unknown
    Autostart  bool
    Detail     string      // e.g. systemd Result= on failure
}

type Supervisor interface {
    Start(ctx context.Context, id string) error
    Stop(ctx context.Context, id string) error      // graceful: SIGINT, wait TimeoutStopSec
    Restart(ctx context.Context, id string) error
    Status(ctx context.Context, id string) (Status, error)
    SetAutostart(ctx context.Context, id string, on bool) error
}
```

**systemd implementation** (production): mutations run
`sudo -n <unitctl_path> <action> <id>`; status runs
`systemctl show valheim@<id>.service --property=ActiveState,SubState,MainPID,ExecMainStartTimestamp,Result,UnitFileState`
without sudo (read-only D-Bus access is allowed for any user). Mapping:
`active/running→running`, `activating→starting`, `deactivating→stopping`,
`failed→failed`, `inactive→stopped`. The service template is installed once by
`install.sh` at `/etc/systemd/system/valheim@.service` and never edited by the
manager; per-instance differences live entirely in `launch.json`.

**direct implementation** (development, tests, non-systemd hosts): the manager
spawns `<self> launch --instance <id>` as a child, redirects stdout/stderr to
`logs/console.log`, stops with SIGINT then SIGKILL after 120 s. Documented
limitation: instances die when the manager exits.

The `instance` service adds one derived state on top: `not_installed` when
`server/valheim_server.x86_64` does not exist. `ready` (world loaded, accepting
players) is derived from the console log, not from systemd.

## 7. Launch contract (`launch.json`)

Written by the manager before every start; read by `valheim-ui launch`, which runs
as the same user with `WorkingDirectory=<server dir>`.

```json
{
  "version": 1,
  "instance_id": "main",
  "server_dir": "/var/lib/valheim/instances/main/server",
  "log_dir": "/var/lib/valheim/instances/main/logs",
  "args": ["-name", "Our Server", "-port", "2456", "-world", "Midgard", "-password", "s3cret",
           "-public", "1", "-savedir", "/var/lib/valheim/instances/main/save", "-crossplay",
           "-modifier", "combat", "hard", "-setkey", "nomap", "-saveinterval", "1800",
           "-backups", "4", "-backupshort", "7200", "-backuplong", "43200",
           "-nographics", "-batchmode"],
  "bepinex": true
}
```

Launcher algorithm:

1. `chdir(server_dir)`; env = process env + `SteamAppId=892970` +
   `LD_LIBRARY_PATH=./linux64:$LD_LIBRARY_PATH`.
2. If `bepinex` and `./start_server_bepinex.sh` exists: parse every `export KEY=VALUE`
   line between the first and second `####` marker lines, expanding `$VAR` / `${VAR}`
   against the env built so far (values may be double-quoted). Apply them. If the
   script is missing, apply the known defaults for BepInEx 5.4.23xx
   (`DOORSTOP_ENABLED=1`, `DOORSTOP_TARGET_ASSEMBLY=./BepInEx/core/BepInEx.Preloader.dll`,
   `LD_LIBRARY_PATH=./doorstop_libs:$LD_LIBRARY_PATH`, `LD_PRELOAD=libdoorstop_x64.so:$LD_PRELOAD`).
   This parsing exists because the variable names changed between pack versions
   (5.4.2202 used `DOORSTOP_ENABLE=TRUE` / `DOORSTOP_INVOKE_DLL_PATH`).
3. Print one line `[valheim-ui] launching valheim_server.x86_64 <args with password masked>`.
4. `syscall.Exec("./valheim_server.x86_64", ["./valheim_server.x86_64", args...], env)`.

Because the launcher `exec`s, systemd's `KillSignal=SIGINT` reaches the game
process directly, which is what triggers Valheim's graceful world save.

## 8. Logs, readiness and players

- The unit has `StandardOutput=append:<instance>/logs/console.log`. Before each
  start the manager rotates `console.log` if it exceeds 20 MB (rename to
  `console-<ts>.log`, keep 5). The file is only ever rotated while the unit is inactive.
- `logs.Tailer` follows the file with fsnotify (poll fallback 1 s), emits
  `instance.log` events and feeds `logs.Parser`.
- The parser is table-driven regex over lines of the form
  `MM/DD/YYYY HH:MM:SS: <message>` (prefix optional). Known messages (best effort,
  Valheim changes them between patches; the parser must never fail on unknown lines):

| Message pattern                                                     | Meaning                            |
|---------------------------------------------------------------------|------------------------------------|
| `Game server connected`                                             | ready=true                         |
| `Session "<name>" with join code <code> and IP <ip>:<port> is active with <n> player(s)` | join code + player count (crossplay) |
| `Got connection SteamID <id>` / `Got handshake from client <id>`    | connection opened (platform id)    |
| `Got character ZDOID from <name> : <a>:<b>`                         | player spawned (name); first one after a connection binds name→id |
| `Closing socket <id>` / `Peer <id> has wrong password`              | connection closed                  |
| `World saved ( <ms>ms )`                                            | save completed                     |
| `Steam game server initialized` / `Load world` / `Server shutdown`  | lifecycle info                     |

- Authoritative player count comes from A2S_INFO on UDP `port+1`, polled every 15 s
  while running (handles the S2C_CHALLENGE round-trip). Names come from the log
  heuristic; the UI shows both and labels the names as best effort.
- Known players are persisted to `players` on every join and exposed for the
  admin/ban/permit editors (one click to add a seen player to a list).

## 9. Jobs

Every long-running or exclusive operation is a `Job`: `install`, `update`,
`backup`, `restore`, `mod_install`, `mod_update`, `mod_uninstall`, `bepinex_install`,
`scheduled_restart`, `thunderstore_refresh`, `world_import`.

- One worker per instance (jobs for the same instance are serialised); global jobs
  (thunderstore_refresh) use instance id `""`. At most 4 workers run concurrently.
- A job writes its output to `jobs/<id>.log` through a `jobs.Logger` that also
  publishes `job.log` events. Status transitions publish `job.updated`.
- Jobs receive a cancellable `context.Context`; SteamCMD and downloads honour it.
- Jobs that need the instance stopped (`update`, `restore`, `world_import`,
  `bepinex_install`) stop it themselves if `stop_if_running` was requested,
  otherwise fail fast with `409 instance_running`. They restart it afterwards if it
  was running before.
- `update` = optional `pre_update` backup → `steamcmd +app_update 896660 validate`
  → refresh `installed_buildid` → start if previously running.

## 10. Backups and worlds

- Backup = zip of `save/worlds_local/<World>.db` + `.fwl` (+ `.db.old`/`.fwl.old` if
  present) + `adminlist.txt` `bannedlist.txt` `permittedlist.txt` + `manifest.json`
  (`instance_id, world, created_at, kind, valheim_buildid, app_version`).
- Copy order is `.fwl` then `.db`; Valheim writes `.new` then renames, so live
  backups are consistent enough (this mirrors Valheim's own `-backups` behaviour).
- Retention runs after every non-manual backup: keep the newest `backup_keep_last`
  and delete anything older than `backup_keep_days`; **manual and uploaded backups
  are never auto-deleted**.
- Restore takes a backup id or an uploaded zip; always creates a `pre_restore`
  backup first; requires the instance stopped.
- Worlds tab lists `worlds_local/*.fwl`, shows which is active, allows upload of a
  `.db`+`.fwl` pair (or a zip containing them), download as zip, delete inactive.

## 11. Scheduling and updates

- `schedules` rows are loaded into one `robfig/cron` instance (cron spec with
  optional seconds disabled; standard 5-field, validated on write, timezone = host).
- Kinds: `restart` (skip if `only_when_empty` and A2S players > 0), `backup`,
  `update` (check remote buildid; if newer, run an `update` job, respecting
  `only_when_empty`).
- Remote build id: `steamcmd +login anonymous +app_info_update 1 +app_info_print 896660 +quit`,
  regex `"public"\s*\{[^}]*?"buildid"\s*"(\d+)"`. Installed build id: `"buildid"\s*"(\d+)"`
  in `server/steamapps/appmanifest_896660.acf`. A global ticker checks every
  `settings.updates.check_interval_minutes` (default 60) and publishes
  `update.available`; applying is only done by schedules or by hand.
- Valheim has no server-side broadcast, so scheduled restarts cannot warn players
  without a mod. The UI states this next to the `only_when_empty` toggle.

## 12. Mods

- **BepInEx**: Thunderstore package `denikson/BepInExPack_Valheim`. The zip contains a
  top-level folder `BepInExPack_Valheim/` whose contents are copied into `server/`.
  `installed` = `server/BepInEx/core/BepInEx.Preloader.dll` exists; version is read
  from the `manifest.json` we store at `server/BepInEx/valheim-ui-pack.json` on install.
  Enabling/disabling is `InstanceConfig.bepinex_enabled` (launch env), no file moves.
- **Thunderstore client**: `GET https://thunderstore.io/c/valheim/api/v1/package/`
  (≈12 MB, ≈10.5k packages) cached on disk, refreshed every
  `settings.thunderstore.index_refresh_hours` (default 6) or on demand. Search is
  in-memory: case-insensitive substring on `full_name`/`name`/`owner`/latest
  description, filters `category`, excludes deprecated by default, sorts
  `rating|downloads|updated|name`, paged 50. Package detail may additionally call
  `https://thunderstore.io/api/experimental/package/{owner}/{name}/`.
  Downloads: `https://thunderstore.io/package/download/{owner}/{name}/{version}/`.
  Set `User-Agent: valheim-server-ui/<version> (+https://github.com/jonasthim/valheim-server-ui)`.
- **Dependency resolution**: BFS over `dependencies` (`owner-name-version` strings);
  `denikson-BepInExPack_Valheim` is satisfied by the BepInEx install step, never
  treated as a mod. Already-installed mods at ≥ required version are skipped;
  lower versions are upgraded. A plan is computed first and shown in the job log
  before any download.
- **Package extraction rules** (mirrors r2modman for Valheim). For each entry in the
  zip, after stripping a single top-level wrapper folder if every entry shares one:
  - `BepInEx/**` → `server/BepInEx/**` (verbatim)
  - `plugins/**`, `patchers/**`, `core/**` → `server/BepInEx/<dir>/<owner>-<name>/**`
  - `config/**` → `server/BepInEx/config/**` (never overwrite an existing cfg)
  - `manifest.json`, `icon.png`, `README.md`, `CHANGELOG.md` → `server/BepInEx/plugins/<owner>-<name>/`
  - anything else → `server/BepInEx/plugins/<owner>-<name>/**`
  Every written path is recorded in `mods.files_json`; uninstall deletes those files
  and then empty parent dirs (never cfg files). Zip-slip is rejected.
- **Enable/disable**: rename each managed `*.dll` to `*.dll.disabled` and back.
- **Config editor**: lists `server/BepInEx/config/*.cfg`. Parser understands BepInEx
  cfg: `[Section]`, `## description` lines, `# Setting type:`, `# Default value:`,
  `# Acceptable values:` comments preceding `Key = Value`. Save rewrites only value
  lines, preserving all comments and order; a raw-text mode exists as fallback.
- Any mod or config mutation while the instance is running sets `pending_restart`.

## 13. Authentication, authorisation, audit

- **Roles**: `viewer` (read-only, secrets masked) < `operator` (run/configure
  instances, backups, mods, players, schedules) < `admin` (+ users, settings,
  create/delete instances). Enforced by `auth.RequireRole(min)` middleware per route;
  the OpenAPI spec annotates each operation with `x-role`.
- **Local login**: argon2id (t=3, m=64 MiB, p=4), constant-time compare, 5 failed
  attempts per username → 5 min lockout (in-memory). Passwords ≥ 10 chars.
- **Sessions**: 32 random bytes, base64url in cookie `vsui_session`
  (HttpOnly, SameSite=Lax, Secure unless `config.insecure_cookies`), stored hashed
  (sha256). Idle expiry 7 d, absolute 30 d. Logout deletes the row.
- **CSRF**: all mutating requests must carry `X-Requested-With: valheim-ui` and an
  `Origin`/`Referer` matching `base_url` when present. Cookies are SameSite=Lax.
- **OIDC**: one provider configured by an admin in Settings (issuer, client id/secret,
  scopes default `openid profile email groups`, provider display name). Auth code
  flow with PKCE, state and nonce in a short-lived cookie. Claims come from the
  verified ID token, supplemented by UserInfo for keys the token lacks (ID token
  wins; UserInfo ignored on subject mismatch). Identity key is `(issuer, sub)`.
  On first login create a user (`auto_create_users`) with role from
  `role_mapping` (groups claim → role, highest wins) else `default_role`; if
  `sync_roles` is on, re-evaluate on every login. Username = `preferred_username`
  or email local part, de-duplicated with a numeric suffix. `local_login_enabled`
  can be switched off once OIDC works; the CLI `valheim-ui admin reset-password`
  is the break-glass path and the last admin cannot be deleted or demoted.
  Operator guide with provider recipes: `docs/OIDC.md`.
- **First run**: while `users` is empty the SPA shows `/setup`, which calls
  `POST /api/v1/auth/setup` to create the first admin. The endpoint 404s afterwards.
- **Audit**: every mutating endpoint writes an `audit_log` row
  (`instance.start`, `instance.config.update`, `backup.restore`, `user.create`, …).
  Viewable by admins at `/audit`.

## 14. HTTP API

Contract: `docs/openapi.yaml` (OpenAPI 3.1). Conventions:

- Base path `/api/v1`, JSON everywhere except file uploads (multipart) and
  downloads (`application/zip`) and `/events` (SSE).
- Errors: `{"error": {"code": "instance_running", "message": "…", "details": {...}}}`
  with codes listed in the spec; validation errors use code `validation_failed` and
  `details.fields[{field, message}]`.
- Long operations return `202 {"job": Job}`; the client follows `job.updated` events.
- `GET /api/v1/events?instance=<id>` is Server-Sent Events. Event names:
  `instance.status`, `instance.log`, `instance.players`, `job.updated`, `job.log`,
  `update.available`, `heartbeat` (every 25 s). Data is JSON. Clients re-fetch
  state on reconnect; there is no replay.
- The Go server serves `web/dist` (embedded) at `/` with SPA fallback to
  `index.html` for non-API paths, `Cache-Control: no-cache` for `index.html` and
  immutable caching for hashed assets.

## 15. Frontend

- Vite + React 19 + TypeScript strict + Mantine 9 + TanStack Query + React Router 7.
- `web/src/api/client.ts` is a thin typed fetch wrapper; types come from
  `web/src/api/schema.d.ts` generated by `npm run gen:api` from `docs/openapi.yaml`.
  Never hand-write API types.
- `web/src/features/<area>/` holds pages and components per area:
  `auth`, `dashboard`, `instances` (overview/config/console/players/worlds/backups/mods/schedules tabs),
  `jobs`, `users`, `settings`, `audit`, `account`.
- One `EventSource` per page session (`useEvents()` hook) dispatching into React
  Query caches (`instance.status` → invalidate/patch instance queries; `job.updated`
  → patch job lists; `instance.log` → ring buffer of 2000 lines for the console).
- Role gating: `useAuth()` exposes `user.role`; buttons the role cannot use are not
  rendered (the API enforces regardless).
- Route map: `/setup`, `/login`, `/` (dashboard), `/instances/new`,
  `/instances/:id/(overview|console|config|players|worlds|backups|mods|schedules)`,
  `/jobs`, `/users`, `/settings`, `/audit`, `/account`.

## 16. Deployment

Install is a single command with nothing else to download by hand:

```
curl -fsSL https://raw.githubusercontent.com/jonasthim/valheim-server-ui/main/deploy/install.sh | sudo bash
```

This works because `deploy/install.sh` is fully self-contained: `unitctl`,
`sudoers.d/valheim-ui`, `valheim-ui.service`, `valheim@.service` and
`config.example.yaml` are embedded in it as heredocs, generated from the
standalone files of the same names by `deploy/build-installer.sh` (template:
`deploy/install.sh.in`, markers `# @@INCLUDE <name>@@`). Those standalone files
remain the source of truth and are what implementers edit; `make deploy-sync`
regenerates `deploy/install.sh` from them, and CI (`make deploy-sync-check`,
also run in the release job) fails the build if the committed file has
drifted. `deploy/install.sh` is committed so the `raw.githubusercontent.com`
URL above always serves a working, current installer.

**Binary layout** (fixed; a concurrent piece of the manager, `self-upgrade`,
depends on it — see §6 and RUNBOOK.md §11):

```
/var/lib/valheim/bin/                 owned root:root, mode 0755
├── valheim-ui                        the real binary, owned root:root, mode 0755
└── valheim-ui.prev                   previous binary, kept for rollback (upgrades only)
/var/lib/valheim/staging/             owned valheim:valheim, mode 0750
├── valheim-ui.new                    a verified download waiting for unitctl apply-upgrade
└── prev.version                      version the last upgrade replaced (for the UI)
/usr/local/bin/valheim-ui             symlink -> /var/lib/valheim/bin/valheim-ui
```

The binary is root-owned so that neither the manager nor a game process
(mods run in it as the same `valheim` user) can rewrite it. Self-upgrade
therefore has two halves: the manager downloads, verifies and stages a release
as `valheim`, and `unitctl apply-upgrade <tag>` (the sudo wrapper, root)
re-verifies the staged file against the digest published in that release's
`SHA256SUMS` before swapping it in. `/usr/local/bin/valheim-ui` — a plain
symlink — is what `ExecStart=`, `sudo -u valheim ... valheim-ui ...` and
everything in RUNBOOK.md actually invoke; `install.sh` creates and refreshes it
on every install/upgrade with `ln -sfn`. Root never executes the managed binary
(the installer runs it as `valheim` to read its version).

`deploy/install.sh` (run as root on Debian 12+/Ubuntu 22.04+, x86_64):

1. `apt-get install` runtime deps: `lib32gcc-s1 lib32stdc++6 libsdl2-2.0-0 libpulse0 libatomic1 ca-certificates curl tar unzip sudo`
   (skippable with `--skip-deps`).
2. Create system user `valheim` (home `/var/lib/valheim`, nologin), directory tree (§3),
   plus root-owned `/var/lib/valheim/bin` (0755) and `valheim`-owned `staging/` (0750).
3. Install the `valheim-ui` binary into the layout above: from a downloaded and
   checksum-verified GitHub release asset (default: latest, or `--version
   vX.Y.Z`; resolved via the redirect of `.../releases/latest`, falling back to
   the GitHub API) or from `--binary <path>` for a local build (skips
   download/verification). `unitctl` to `/usr/local/lib/valheim-ui/unitctl`
   (root:root 0755), sudoers drop-in `/etc/sudoers.d/valheim-ui` (validated
   with `visudo -c`), units `valheim-ui.service` and `valheim@.service`,
   `/etc/valheim-ui/config.yaml` (only if absent).
4. Download SteamCMD (`https://steamcdn-a.akamaihd.net/client/installer/steamcmd_linux.tar.gz`)
   into `/var/lib/valheim/steamcmd` and run it once as `valheim` to self-update.
5. `systemctl daemon-reload && systemctl enable --now valheim-ui`; print the URL and
   the note that first visit creates the admin account.

Idempotent: re-running upgrades the binary (keeping the previous one as
`valheim-ui.prev`) and unit files without touching data. `--uninstall` removes
units, sudoers, the symlink and the bin directory but keeps `/var/lib/valheim`.
`--check` performs no writes: it prints the installed vs. latest/target version
and, for every step above, whether it is already up to date or would change
(composes with `--uninstall` too).

Manager unit essentials: `User=valheim`, `ProtectSystem=strict`,
`ReadWritePaths=/var/lib/valheim`, `NoNewPrivileges=no` (sudo needs it),
`Restart=always`, `RestartSec=3` — a clean `exit(0)` after a self-upgrade swap
is what triggers the restart onto the new binary, so this must not be
`on-failure`. Instance template essentials: `User=valheim`,
`WorkingDirectory=/var/lib/valheim/instances/%i/server`,
`ExecStart=/usr/local/bin/valheim-ui launch --instance %i`,
`StandardOutput=append:/var/lib/valheim/instances/%i/logs/console.log`,
`StandardError=inherit`, `KillSignal=SIGINT`, `TimeoutStopSec=120`,
`Restart=on-failure`, `RestartSec=10`, `LimitNOFILE=100000`. Instance units
carry no `Wants=`/`After=`/`PartOf=` relationship to `valheim-ui.service` in
either direction, and never will: that independence is what lets the manager
restart (on-failure or after a self-upgrade) without touching running game
servers.

Release assets (built by the `release` job in `.github/workflows/ci.yml` on
`v*` tags, version ldflag = the tag): `valheim-ui_linux_amd64.tar.gz` (single
file `valheim-ui`), `deploy.tar.gz` (the `deploy/` directory), `install.sh`
(same file served at the raw URL above, attached directly so
`curl -fsSLO .../releases/latest/download/install.sh` also works), and
`SHA256SUMS` covering all three — this is what `install.sh` downloads and
verifies against.

## 17. Configuration (`/etc/valheim-ui/config.yaml`)

```yaml
listen: "127.0.0.1:8080"        # put a TLS reverse proxy in front for remote access
base_url: "https://valheim.example.com"   # used for OIDC redirect URI and CSRF origin check
data_dir: "/var/lib/valheim"
supervisor: "systemd"            # systemd | direct
unitctl_path: "/usr/local/lib/valheim-ui/unitctl"
steamcmd_path: "/var/lib/valheim/steamcmd/steamcmd.sh"
insecure_cookies: false          # true only for plain-http LAN setups
log_level: "info"
# development only:
# supervisor: direct, fake_server: true, fake_server_path: ./testdata/fake-server.sh,
# dev_no_auth: true (requires listen on 127.0.0.1; injects a synthetic admin)
```

Every key can be overridden with `VALHEIM_UI_<UPPERCASE_KEY>`.

## 18. Testing strategy

- Unit tests per package; no network in unit tests (Thunderstore and SteamCMD are
  behind interfaces with fakes; fixtures under `testdata/`).
- `internal/launcher` tests use a fake `valheim_server.x86_64` shell script in a
  temp dir that echoes its args/env.
- E2E (`make e2e`): start the manager with `supervisor: direct` and
  `VALHEIM_UI_FAKE_SERVER=1`, which makes the launcher exec `testdata/fake-server.sh`
  (prints realistic log lines, answers nothing on UDP). Playwright (`web/e2e/`) drives setup →
  create instance → start → see logs → backup → stop.
- CI (GitHub Actions): `go vet`, `golangci-lint`, `go test -race`, `npm run typecheck`,
  `npm run lint`, `npm run build`, e2e on push/PR; release workflow on `v*` tags builds
  `valheim-ui_linux_amd64.tar.gz` and attaches it.

## 19. Non-goals for v1

Docker runtime, arm64, Valheim Plus, in-game chat/RCON, multi-host, metrics
exporter, HTTPS termination (use a reverse proxy), API tokens for automation.

## Resource metrics

`internal/metrics` samples `/proc` (no cgo, no dependencies): `/proc/stat`, `/proc/meminfo`
and `/proc/loadavg` for the host, `/proc/<pid>/stat` and `statm` for each running game
process. The sampler keeps the previous reading per subject so cumulative CPU time becomes
a percentage (host: all cores, 0-100; process: percent of one core, like `top`), never
computes over a window shorter than one second, and forgets processes not asked about for
five minutes. It is registered as a `StatusEnricher`, so `InstanceStatus` carries
`cpu_percent` and `memory_bytes` while running, and `GET /system` returns `host` metrics for
the dashboard tiles.


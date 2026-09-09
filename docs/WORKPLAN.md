# Work plan

Implementation is split into work packages (WP) sized for a single focused
implementer. Each WP lists the files it owns; do not edit files owned by another
WP without saying so. Waves can run in parallel; a wave starts only when the
previous wave builds and passes `make check`.

Legend: **Depends** = must be merged first. **Owns** = files the WP creates/edits.
**Done when** = acceptance criteria the reviewer verifies.

Wave 0 (scaffold) is done by the architect and includes: repo layout, `go.mod`
with all dependencies, `internal/domain` types, all package interfaces, router
with 501 stubs per resource, migrations `00001_init.sql`, config loader, main
with subcommands, `web/` scaffold with routing shell, generated API types,
Makefile, CI, deploy files.

---

## Wave 1 — backend foundations (parallel)

### WP-01 Database, auth, users, settings, audit
Owns: `internal/db/**`, `internal/auth/**`, `internal/audit/**`,
`internal/api/auth_handlers.go`, `internal/api/users_handlers.go`,
`internal/api/settings_handlers.go`, `internal/api/audit_handlers.go`, `internal/api/middleware.go`.
Depends: wave 0.
Scope:
- `db.Open` (WAL, foreign keys, busy_timeout 5s), goose migrate up, repository
  structs with plain `database/sql` for users, identities, sessions, settings, audit.
- Argon2id hashing (`auth.HashPassword`, `auth.VerifyPassword`), login lockout
  (5 failures / 5 min, in-memory keyed by username).
- Session create/lookup/touch/delete; cookie handling as in ARCHITECTURE §13.
- Middleware: `Authenticate` (loads user into context), `RequireRole(min)`,
  `CSRFGuard` (header + origin check for non-GET), request logging, recoverer.
- Handlers: `/auth/*` (status, setup, login, logout, me, password), `/users*`,
  `/settings*` incl. `/settings/oidc/test`, `/audit`.
- OIDC: `auth/oidc.go` using go-oidc; login/callback handlers; identity linking;
  role mapping; `sync_roles`; provider (re)initialised whenever settings change.
- `settings.Service` with defaults, validation, secret-keeping semantics.
- Audit writer `audit.Record(ctx, action, instanceID, target, details)` reading
  user + IP from context; called from handlers in every WP (helper in `api`).
- CLI `valheim-ui admin create-user|reset-password|list-users` in `cmd/` (edits
  `cmd/valheim-ui/admin.go` only).
Done when: `go test ./internal/auth/... ./internal/db/...` cover hashing, lockout,
session expiry, role mapping (table test with groups→role), settings defaults;
manual curl flow setup→login→me→logout works; OIDC login works against a
mock provider in tests (use `httptest` serving discovery+JWKS+token endpoints).

### WP-02 Instance service, launcher, supervisor
Owns: `internal/instance/**`, `internal/launcher/**`, `internal/supervisor/**`,
`internal/api/instances_handlers.go`, `cmd/valheim-ui/launch.go`,
`testdata/fake-server.sh`.
Depends: wave 0.
Scope:
- `instance.Service`: create (dir tree, DB row, port overlap check), get/list,
  update config with validation (ARCHITECTURE §5.1), delete (refuse if running;
  `delete_files` removes tree), `Paths(id)`, `RenderLaunch(id)` writing
  `launch.json` atomically (0600), pending_restart bookkeeping, `installed`
  detection, autostart passthrough.
- `instance.BuildArgs(cfg, savedir) []string` deterministic, unit-tested for every
  option including modifiers order, setkeys, extra_args, empty preset.
- `launcher.Run(instanceDir)`: as ARCHITECTURE §7 including the `####`-delimited
  export parser with `$VAR`/`${VAR}` expansion (tests with the 5.4.2202 and
  5.4.2333 script variants under `testdata/`). Honour `VALHEIM_UI_FAKE_SERVER=1`
  → exec `testdata/fake-server.sh` (path from env `VALHEIM_UI_FAKE_SERVER_PATH`).
- `supervisor.Systemd` (sudo unitctl + `systemctl show` parsing, unit tests on
  parsed output) and `supervisor.Direct` (child process, SIGINT→SIGKILL, status).
- Console log rotation before start (§8).
- Handlers: instances CRUD, start/stop/restart, status, logs tail/download
  (`GET /logs` reads last N lines efficiently — seek from end).
- `status` composition: supervisor state + installed check + pending_restart +
  fields provided by other WPs through the `domain.StatusEnricher` hook (players,
  a2s, join code, update_available, bepinex).
- `fake-server.sh`: prints realistic startup lines (see `internal/logs/testdata`
  once WP-03 lands; until then use lines from ARCHITECTURE §8), then a
  "Session ... join code" line every 10 s, exits 0 on SIGINT after printing
  `World saved ( 12.3ms )`.
Done when: with `supervisor: direct` and the fake server, `POST /instances` (with
`install:false`), `POST start`, `GET status` (running), `GET logs` (lines
present), `POST stop` (stopped, exit clean) all work; `go test` covers args,
launcher env parsing, systemctl parsing, validation matrix.

### WP-03 Log tailer/parser, A2S query, players & lists
Owns: `internal/logs/**`, `internal/query/**`, `internal/players/**`,
`internal/api/players_handlers.go`.
Depends: wave 0 (integrates with WP-02 via `domain.StatusEnricher` and events).
Scope:
- `logs.Tailer`: follow file from end (or last N lines on attach), fsnotify with
  polling fallback, handles truncation/rotation, emits lines to a callback.
- `logs.Parser`: table-driven regexes for the messages in ARCHITECTURE §8, emits
  typed events (Ready, JoinCode{code, players}, Connected{id}, Spawned{name},
  Disconnected{id}, Saved); tests with fixture log excerpts in `testdata/`.
- `players.Tracker` per instance: maintains online set (heuristic id↔name
  binding), persists known players, publishes `instance.players`.
- `query.A2SInfo(ctx, addr) (Info, error)` with challenge handling and 2 s timeout;
  test against an in-process fake UDP responder.
- `players.Lists`: parse/write `adminlist.txt` etc. preserving `//` comments,
  atomic write; handlers for GET/PUT lists and GET players.
- Register a `StatusEnricher` that fills players_online, max_players, join_code,
  a2s, ready.
Done when: parser tests pass on fixtures; A2S test passes; lists round-trip
preserving comments; running the fake server shows ready=true and join code in
`GET status` within 15 s.

### WP-04 Steam, jobs, events bus, SSE
Owns: `internal/steam/**`, `internal/jobs/**`, `internal/events/**`,
`internal/api/jobs_handlers.go`, `internal/api/events_handlers.go`,
`internal/api/system_handlers.go`.
Depends: wave 0.
Scope:
- `events.Bus`: publish/subscribe with per-subscriber buffered channel (drop
  oldest on overflow, never block publishers), instance filter.
- SSE handler: headers, heartbeat 25 s, flush per event, client disconnect cleanup.
- `jobs.Runner`: per-instance serial queues, max 4 concurrent, persistence in
  `jobs` table, `jobs/<id>.log` writer + `job.log` events, cancel via context,
  recovery on startup (mark `running` rows as `failed: manager restarted`).
  API: `Enqueue(ctx, Job{Type, InstanceID, Title, RequestedBy}, fn func(ctx, *Logger) error) (Job, error)`,
  `Get`, `List`, `Cancel`, `ActiveFor(instanceID)`.
- `steam.Client`: `Install/Update(ctx, installDir, logger)` running steamcmd with
  streamed output, `InstalledBuildID(installDir)`, `LatestBuildID(ctx)` with
  parsing tests on captured `app_info_print` output (fixture in `testdata/`),
  detection of steamcmd missing → `steamcmd_missing` error.
- Handlers: `/jobs*`, `/events`, `/system` (version, disk usage via `syscall.Statfs`).
- Global update-check ticker (interval from settings; 0 = off) storing
  `latest_buildid` and publishing `update.available` per instance whose
  installed build differs.
Done when: jobs run serially per instance and concurrently across instances
(test with sleeping jobs); SSE streams `job.log` lines in `curl -N`; steam parsers
pass fixtures; cancel kills a running fake steamcmd script.

---

## Wave 2 — backend features (parallel; depends on wave 1)

### WP-05 Install/update jobs and update checks wiring
Owns: `internal/instance/jobs.go`, `internal/api/instances_jobs_handlers.go`.
Depends: WP-02, WP-04.
Scope: `POST /install`, `POST /update` (stop_if_running semantics, pre_update
backup via WP-06's `backup.Service` interface — call it, do not implement),
`POST /update-check`; instance create with `install:true` enqueues install;
refresh `installed_buildid` after jobs.
Done when: install job against real SteamCMD in a scratch dir succeeds (manual,
network) and against a fake steamcmd script in tests; update flow restarts the
fake server afterwards.

### WP-06 Backups and worlds
Owns: `internal/backup/**`, `internal/api/backups_handlers.go`, `internal/api/worlds_handlers.go`.
Depends: WP-02, WP-04.
Scope: zip create with manifest, restore (pre_restore first, stopped check,
stop_if_running), retention (ARCHITECTURE §10), upload/download, DB reconcile at
startup and on list, worlds list/upload/download/delete, `world_import` job.
Done when: backup→delete world→restore round-trips byte-identical files; retention
tests with synthetic timestamps; upload of a zip missing `.fwl` is rejected 422.

### WP-07 Scheduler
Owns: `internal/scheduler/**`, `internal/api/schedules_handlers.go`.
Depends: WP-02, WP-04, WP-06 (uses backup service), WP-05 (uses update job).
Scope: cron validation, load/reload on change, `only_when_empty` via players
count provider, `run` now endpoint, last_result/next_run bookkeeping, restart as a
`scheduled_restart` job for auditability.
Done when: a `* * * * *` backup schedule in tests (clock injected) fires and
produces a backup row; `only_when_empty` skip records `skipped`.

### WP-08 Mods: BepInEx, Thunderstore, installer, config editor
Owns: `internal/mods/**`, `internal/api/mods_handlers.go`, `internal/api/thunderstore_handlers.go`.
Depends: WP-02, WP-04.
Scope: everything in ARCHITECTURE §12. Split internally into `thunderstore.go`
(client+cache+search+categories), `bepinex.go`, `installer.go` (extraction rules,
files tracking, enable/disable, uninstall), `resolver.go` (dependency plan),
`cfg.go` (BepInEx cfg parse/update). All HTTP behind an interface with a fake
for tests; fixtures: a hand-made mini index JSON, a sample package zip built in
the test, real `start_server_bepinex.sh` variants.
Done when: install of a package with two dependencies from the fake index
produces the expected tree and DB rows; uninstall removes exactly those files;
cfg update preserves comments; search returns paged/sorted results; enabling
BepInEx sets `pending_restart` while running.

---

## Wave 3 — frontend (parallel; depends on wave 1 API being live; can start
against the OpenAPI spec with a running wave-1 backend)

Shared (wave 0): app shell, router, auth guard, `useAuth`, `useEvents`, API
client, notifications, theme.

### WP-10 Auth, setup, account, users, settings, audit pages
Owns: `web/src/features/auth/**`, `web/src/features/account/**`,
`web/src/features/users/**`, `web/src/features/settings/**`, `web/src/features/audit/**`.
Done when: the first-run wizard at /setup is reachable on an empty database, creates the admin (username, display name, optional email, password with confirmation and strength hints) and logs them straight in, and is unreachable afterwards; local login, OIDC button when enabled,
change password, users CRUD with last-admin guard messaging, settings form with
OIDC test button and redirect URI copy field, audit table with filters.

### WP-11 Dashboard, instance create, overview, config, console
Owns: `web/src/features/dashboard/**`, `web/src/features/instances/{InstancePage,OverviewTab,ConfigTab,ConsoleTab,CreateInstancePage}.tsx` and their subfolders.
Done when: dashboard cards live-update from SSE; create wizard validates like the
API (port overlap error surfaced); overview shows state, ready, players, join
code, build id, update banner, pending-restart banner with restart button; config
form covers every InstanceConfig field with help text; console shows live log
with pause/follow, filter, download, and player join/leave chips.

### WP-12 Players, worlds, backups, schedules tabs
Owns: `web/src/features/instances/{PlayersTab,WorldsTab,BackupsTab,SchedulesTab}.tsx` and subfolders.
Done when: list editors with add-from-known-player, backups table with
create/restore/download/delete and job progress, world upload dropzone, schedule
editor with cron presets (every day 04:00 etc.) and human-readable next run.

### WP-13 Mods tab and Thunderstore browser
Owns: `web/src/features/instances/ModsTab*.tsx`, `web/src/features/mods/**`.
Done when: BepInEx card (install/upgrade/enable), installed mods table with
enable/update/uninstall, Thunderstore search modal with categories/sort/paging
and install button showing the dependency plan from the job log, cfg editor with
typed inputs (bool→switch, enum→select, number→NumberInput) and raw fallback.

### WP-14 Jobs page and job drawer
Owns: `web/src/features/jobs/**`.
Done when: jobs list filters by instance/status, drawer streams live log, cancel
button for running jobs, global activity indicator in the header.

---

## Wave 4 — integration, e2e, packaging

### WP-20 E2E and CI hardening
Owns: `e2e/**`, `.github/workflows/ci.yml`.
Playwright: setup → login → create instance (install:false) → start (fake) →
console shows lines → backup → restore → stop → users CRUD → logout.

### WP-21 Install script verification and runbook
Owns: `deploy/**`, `docs/RUNBOOK.md`.
Test `install.sh` in a Debian 12 systemd container (or VM) end to end including
real SteamCMD install of one instance; document upgrade, uninstall, backup of the
manager DB, reverse proxy examples (Caddy, nginx), OIDC examples (Authelia,
Keycloak), troubleshooting (sudo, ports, SteamCMD deps).

---

## Review checklist (used by the architect for every WP)

1. `make check` clean; new tests present and meaningful.
2. No new dependencies; no edits outside owned files (or justified).
3. Spec drift: handlers match `docs/openapi.yaml` shapes and status codes.
4. Errors use the documented codes; secrets never logged; paths validated.
5. Audit entries for every mutation; role gates on every route.
6. Behaviour verified by running the fake-server flow, not only by tests.

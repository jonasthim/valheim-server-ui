# Security

This document is the threat model of Valheim Server UI, the controls that follow from it,
and the record of the September 2026 security review: every finding, its severity, and
whether it was fixed or accepted. Report new issues privately to the maintainer through
GitHub's security advisories for this repository.

## Threat model

Deployment: one Linux host, the manager as the unprivileged `valheim` user behind a TLS
reverse proxy on loopback, administered by whoever installed it. Three roles: `viewer`
(read-only, low trust), `operator` (day-to-day server operations, semi-trusted) and
`admin` (everything).

What the design must survive:

1. **Anonymous internet traffic** reaching the login page: brute force, enumeration,
   resource exhaustion, CSRF, clickjacking.
2. **A viewer** trying to read secrets or reach operator actions.
3. **An operator** trying to escape an instance directory or the `valheim` user. Note that
   operators can upload mod DLLs, and mods run inside the game process as `valheim`, so
   *code execution as `valheim` is an operator-level capability by design*. The boundary
   that matters is between `valheim` and root, and between one instance's files and
   everything else the user owns.
4. **Code running as `valheim`** (a malicious or compromised mod): it must not become root,
   must not be able to replace the manager binary with an unpublished one, and must not
   persist through the recovery actions an administrator would take.
5. **The supply chain**: GitHub releases, GitHub Actions, npm and Go modules, SteamCMD,
   Thunderstore.

Out of scope: a compromised GitHub account of the maintainer (releases are checksummed, not
signed; see accepted risks), physical access to the host, and the Valheim server binary
itself.

## Controls

**Process and host boundary**
- Manager and game servers run as the system user `valheim` (nologin, home
  `/var/lib/valheim`). The manager's only privilege is `sudo -n /usr/local/lib/valheim-ui/unitctl`,
  a root-owned bash wrapper that accepts a closed set of verbs, validates the instance id
  against `^[a-z0-9][a-z0-9-]{0,31}$`, calls `systemctl` by absolute path with an argv array,
  and is installed from a `visudo`-validated drop-in with `env_reset` and `secure_path`.
- `valheim@.service` (the game) is sandboxed: `NoNewPrivileges`, `ProtectSystem=strict`,
  `ProtectHome`, `PrivateTmp`, `PrivateDevices`, protected kernel tunables/modules/logs,
  empty capability bounding set, `RestrictSUIDSGID`, namespace and realtime restrictions,
  `RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6 AF_NETLINK`, `UMask=0027`. A mod
  cannot call sudo, cannot read `/home`, cannot write outside `/var/lib/valheim`.
- `valheim-ui.service` (the manager) has the same set except `NoNewPrivileges` and
  `RestrictSUIDSGID`, which the setuid `sudo` needs.
- The manager binary is root-owned in a root-owned directory. Self-upgrade stages a
  checksum-verified, sanity-run download as `valheim` and `unitctl apply-upgrade <tag>`
  re-verifies the staged file's digest against the release's `SHA256SUMS` before installing
  it. Root never executes the managed binary; the installer reads its version as `valheim`.
- No shell anywhere on the execution path: `syscall.Exec` for the game with an argv built
  from validated config, `exec.Command` with argv slices for steamcmd and systemctl, both
  resolved from fixed system directories. `extra_args` may not repeat or override the flags
  the manager controls (`-savedir`, `-password`, `-logFile`, ...).
- Every instance path is derived from `domain.PathsFor`; every user-supplied filename is a
  validated plain basename; archives are extracted with zip-slip checks, a declared-size
  budget per archive and a per-entry cap that fails when an entry inflates past its header.

**Authentication and sessions**
- argon2id (t=3, m=64 MiB, p=4) with per-hash random salts; constant-time comparison; the
  unknown-user path spends the same work as a real verification so timing does not reveal
  which usernames exist, and a disabled account answers like a wrong password unless the
  password is right.
- Sessions: 256-bit random tokens, only the SHA-256 stored; `HttpOnly`, `SameSite=Lax`,
  `Secure` cookies; idle (7 d) and absolute (30 d) lifetimes enforced server-side; a fresh
  token on every login; the user row re-read on every request so role changes and
  disables apply immediately; all other sessions revoked on password change and reset.
- Lockout per username (5 failures / 5 min) and per client IP (30 / 5 min), bounded table.
- OIDC with PKCE (S256), `state` and `nonce`, issuer/audience/signature verification by
  go-oidc, identity keyed by `(issuer, subject)`; linking to a local account by email only
  with `email_verified: true` (opt-in for providers that omit the claim, never for admins);
  post-login redirects restricted to same-origin paths.
- CSRF: a custom `X-Requested-With` header on every state-changing request (forces a CORS
  preflight nobody answers) plus an Origin/Referer check against `base_url`.
- Response headers: CSP (`default-src 'self'`, inline styles only, https images),
  `X-Frame-Options: DENY`, `X-Content-Type-Options: nosniff`, `Referrer-Policy`, and
  `Cache-Control: no-store` on every JSON response. HSTS is set by the reverse proxy
  (RUNBOOK §2).

**Data and secrets**
- The SQLite file is `0600`; the data directory `0750`. The game password and the OIDC
  client secret are stored in plaintext because both must be recoverable at runtime; the
  API masks the game password for viewers and never returns the client secret; the audit
  diff masks any `password`, `client_secret`, `secret` or `token` field; nothing secret is
  logged.
- Backups contain world files and player lists only, never the instance config.
- The audit log is append-only through the API and records who did what, from which
  address, and for updates exactly which fields changed.

**Supply chain**
- CI: `permissions: contents: read` by default, pinned tool versions, the OpenAPI type
  generator (run through `npx`, outside the lockfile) confined to a job that produces no
  release artifact and runs with scripts disabled, `npm audit` and `govulncheck` gates.
- Thunderstore downloads only over https from `thunderstore.io` hosts (re-checked on every
  redirect), bounded by the size the index declares.
- SteamCMD is downloaded over TLS and unpacked as `valheim` with archive ownership and
  modes ignored; its digest is printed for the operator's records (Valve publishes none).

## Review, September 2026

Four read-only audits (authentication and sessions; API authorization and input handling;
host boundary and supply chain; data, secrets, frontend and dependencies). The role table
of all 76 API operations matched the contract; no route was missing authentication.

| id | severity | finding | status |
|---|---|---|---|
| H-1 | High | Game unit had no sandboxing; a mod inherited the manager's whole trust domain | Fixed: hardened `valheim@.service` (ADR-019) |
| H-2 | High | `install.sh --check` executed the `valheim`-writable binary as root | Fixed: binary is root-owned; the installer runs it as `valheim` (ADR-018) |
| M-1 | Medium | Installer wrote into a `valheim`-owned `bin/` without symlink guards | Fixed: root-owned `bin/`, symlink refused |
| M-2 | Medium | Releases checksummed but not signed; `SHA256SUMS` from the same release | Accepted (see below); mitigated by root-side re-verification and no-downgrade |
| M-3 | Medium | CI actions pinned by mutable tags, `@latest` tools, no default `permissions` | Partly fixed: tools pinned, least-privilege token, generator isolated; SHA-pinning of actions is a follow-up |
| M-4 | Medium | Upgrade `version` interpolated unvalidated into the GitHub URL | Fixed: tag regex + path escaping |
| M-5 | Medium | Thunderstore download without host allowlist or size cap | Fixed |
| M-6 | Medium | SteamCMD unpacked by root without integrity check | Fixed: unpacked as `valheim`, safe tar flags, digest printed |
| M-7 | Medium | Manager unit missing capability/namespace restrictions | Fixed (all that coexist with sudo) |
| M-8 | Medium | Zip extraction without decompression limits (zip bomb) | Fixed: per-archive budget and per-entry declared-size cap |
| A-01 | Medium | Password change/reset kept other sessions alive | Fixed: sessions revoked (the changing tab is kept) |
| A-02 | Medium | Username enumeration by error code and timing | Fixed |
| A-03 | Medium | Lockout per username only; remote DoS of the admin login | Fixed: per-IP counter added, table bounded, CLI reset clears the lock |
| A-04 | Medium | OIDC linked by email when `email_verified` was absent | Fixed: opt-in setting, never for admins |
| F-1 | Medium | Unbounded audit rows and lockout keys from the login endpoint | Fixed: username length cap before any state, audited target truncated |
| A-05 | Low | Open redirect via `next=/\evil.com` | Fixed (server and client) |
| A-06 | Low | No security response headers | Fixed |
| A-07 | Low | `X-Real-IP` trusted though Caddy does not set it | Fixed: last `X-Forwarded-For` element only |
| A-09 | Low | No `Cache-Control: no-store` on JSON | Fixed |
| F-4 | Low | SQLite file created with the process umask | Fixed: `0600` |
| F-6 | Low | Third-party URLs rendered as `href` without scheme check | Fixed: http(s) only |
| F-7 | Low | Release notes body unbounded | Fixed: 1 MiB document, 32 KiB notes |
| L-1 | Low | `sudo`/`systemctl` resolved through `PATH` | Fixed: fixed system directories |
| L-2 | Low | Self-update HTTP client without timeout | Fixed |
| L-3 | Low | No downgrade protection | Fixed |
| L-4 | Low | World/backup uploads without a request body cap | Fixed: 4 GiB |
| L-5 | Low | sudoers relied on inherited `env_reset` | Fixed: explicit |
| L-6 | Low | Archive file modes taken from the zip | Fixed: `0640`/`0750` |
| L-7 | Low | `extra_args` could pass `-logFile`, a second `-savedir` | Fixed: reserved flags refused |
| B-2 | Low | Integer overflow in package search paging (500 for viewers) | Fixed |
| B-3 | Low | Newline injection into player list comments | Fixed |
| B-4 | Low | World name and job id validation gaps at the API layer | Fixed |
| I-1 | Info | Viewers could read BepInEx config files (often hold tokens) | Fixed: operator |
| A-12 | Info | Expired sessions never purged; no "sign out everywhere" | Open (hygiene) |
| A-14 | Info | First-run setup is first-come, first-served | Accepted: documented; expose the URL after setup |
| F-8 | Info | Viewers see host paths, PIDs and join code | Accepted: viewer is a trusted-friend role |
| F-9 | Info | Game password appears on the game process argv | Accepted: Valheim requires it |

Verified safe by the audits (not exhaustive): argon2id parameters and constant-time
comparison; hashed session storage; `RequireRole` on every route; CSRF header enforcement
and cookie flags; PKCE/state/nonce handling; `unitctl` argument validation against
newline, dash, space, case and length attacks; zip-slip checks in mods and backups;
`removeTree` refusing paths outside `instances/`; parameterised SQL throughout;
`Content-Disposition` quoting; no `dangerouslySetInnerHTML` or HTML rendering of
third-party text in the frontend; `npm audit` and `govulncheck` clean (one module-level
advisory for an unused `x/crypto` package).

## Accepted risks

- **Release authenticity rests on the GitHub account.** Assets are checksummed, and the
  root-side installer re-verifies the digest, but the digest comes from the same release.
  Signing releases (cosign keyless) is the planned next step; until then keep two-factor
  authentication on the GitHub account and treat `RELEASE_TOKEN`, if configured, as a
  production credential.
- **Operators can run code as `valheim`** by uploading a mod. That is the feature. The
  sandbox on the game unit and the root-owned binary bound what that code can reach.
- **Third-party apt sources and SteamCMD** are trusted at install time.

## Reporting

Use GitHub security advisories on the repository. Include the version (Settings →
Application), the role you were using and the request that triggered the behaviour.

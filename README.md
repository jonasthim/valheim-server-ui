# valheim-server-ui

Web UI and configuration management for Valheim dedicated servers on Linux.
One static binary, native SteamCMD installs, systemd-supervised instances,
multi-user login (local and OIDC), backups, schedules and BepInEx/Thunderstore mods.

- Install and operate: [`docs/RUNBOOK.md`](docs/RUNBOOK.md)
- Design: [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md), decisions in [`docs/DECISIONS.md`](docs/DECISIONS.md)
- API contract: [`docs/openapi.yaml`](docs/openapi.yaml)
- Contributing / implementer guide: [`CLAUDE.md`](CLAUDE.md), work packages in [`docs/WORKPLAN.md`](docs/WORKPLAN.md)

## Quick start (Debian 12+ / Ubuntu 22.04+, x86_64)

```bash
curl -fsSLO https://github.com/jonasthim/valheim-server-ui/releases/latest/download/deploy.tar.gz
tar -xzf deploy.tar.gz && sudo ./deploy/install.sh
```

Open `http://127.0.0.1:8080` (put a TLS reverse proxy in front for remote access).
The first visit runs the setup wizard that creates the administrator account.

## Development

```bash
make deps        # go mod download + npm ci
make build       # web build + go build → bin/valheim-ui
make check       # vet, lint, tests, typecheck
make e2e         # Playwright suite against the fake game server
make dev-backend # backend on :8080 with the direct supervisor and fake server
make dev-web     # Vite dev server on :5173
```

Requirements: Go 1.26, Node 22. Dependencies are kept on latest majors with no
known vulnerabilities (`govulncheck` and `npm audit` run in CI).

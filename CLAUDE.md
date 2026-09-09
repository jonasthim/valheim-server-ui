# valheim-server-ui — guide for implementers (humans and agents)

Read `docs/ARCHITECTURE.md` first. It is the source of truth; `docs/openapi.yaml` is
the API contract; `docs/WORKPLAN.md` lists the work packages and who owns which files.

## Ground rules

- **Stay inside your work package's file ownership.** If you must touch a shared file
  (router wiring, `go.mod`, `web/src/App.tsx`), do the minimum and say so in your report.
- **No new dependencies** without approval. `go.mod`/`package.json` already contain
  everything the plan needs. If something is missing, stop and report; do not `go get`.
- **Contract first.** If the OpenAPI spec is wrong or incomplete for your package, fix
  the spec in the same change and regenerate frontend types (`make gen`).
- **Never write** `panic`, `log.Fatal` outside `main`, or `os.Exit` inside packages.
  Return errors; wrap with `fmt.Errorf("context: %w", err)`.
- **Context everywhere.** Every I/O function takes `ctx context.Context` first.
- **Logging** with `log/slog` via the logger passed in; never `fmt.Println` in packages.
- **Paths**: only build instance paths through `instance.Paths` helpers. Reject any
  user-supplied path segment that is not a plain filename (`filepath.Base(x) == x`,
  no `..`, not empty). Zip extraction must guard against zip-slip.
- **Secrets**: the server password and OIDC client secret are never logged; the API
  masks them for the `viewer` role and returns `""` for client secret on read.
- **Errors to clients** use `api.WriteError(w, status, code, msg)` with codes from the
  spec. Validation errors use `api.WriteValidation(w, fields)`.
- **Tests**: table-driven, no network, fixtures in `testdata/`. Every package with logic
  has tests. Run `make check` before reporting done; paste the output summary.
- **Frontend**: TypeScript strict, no `any`, all API types imported from
  `web/src/api/schema.d.ts`. Mantine components only; no other UI kit or CSS framework.
  Every mutation shows a notification on success/failure. Role-gate buttons with `useAuth()`.
- **Commits**: conventional messages (`feat(mods): thunderstore search`), one work
  package per commit where possible.

## Commands

```
make deps        # go mod download + npm ci
make gen         # regenerate web/src/api/schema.d.ts from docs/openapi.yaml
make build       # web build + go build → bin/valheim-ui
make check       # go vet + golangci-lint + go test -race + npm typecheck/lint
make dev         # runs backend (direct supervisor, ./devdata) and vite dev server
make e2e         # playwright against a dev server with the fake game server
```

## Repository map

See `docs/ARCHITECTURE.md` §4. One resource per `internal/api/*_handlers.go`, one
package per concern under `internal/`, one feature folder per UI area under
`web/src/features/`.

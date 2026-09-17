# Design system — "The Longhouse Table"

Everything is Mantine (no other UI kit or CSS framework); the look comes from
`web/src/theme/theme.ts`, `web/src/index.css` and the primitives in `web/src/ui/`.

> **Direction: the longhouse table.** A warm, lamplit, dark-timber workspace on which the parchment map is the object of attention. Material and typography carry the identity; no filigree, runes-as-decoration, or ornate borders. The map is the memorable thing; everything else is quiet. Two materials only: timber chrome and parchment paper. One accent (ember) reserved for the primary action, active nav and focus. Hierarchy by size and type, never by identical tiles. Two type voices: Valheim Display (Cormorant Garamond) for names and titles, Source Sans 3 for everything else, JetBrains Mono for logs.
>
> | Token | Dark | Light | Role |
> |---|---|---|---|
> | body (`hearth`) | `#17130F` | `#E9DFC4` | page base |
> | surface (`timber`) | `#241D16` | `#F3ECD8` | cards |
> | raised (`oak`) | `#32281E` | `#FAF5E6` | hover / active |
> | sidebar | `#120F0B` | `#DED2B3` | navbar + header |
> | parchment | `#B9AD8C` | `#D8CCAA` | paper surfaces: map, empty states, login backdrop |
> | ink | `#EFE6CC` on dark, `#1A120A` on parchment | | text |
> | ember | `#F5A20F` | `#c97c05` | the single accent |
> | moss / blood / frost | unchanged | | status hues |
>
> Contrast: ink on timber ≈ 12:1, ember on timber ≈ 8:1, ink on parchment ≈ 9:1.

## Principles

1. **Calm surfaces, one warm accent.** Warm timber neutrals; ember gold is reserved
   for primary actions, the active nav item and focus. Never use ember for
   decoration or status.
2. **Status has a colour language.** running/success = `moss` (green),
   failed/destructive = `blood` (red), informational = `frost` (blue),
   pending/warning = `straw` (yellow), mods/secondary = `spirit` (violet).
   The stock names `green red blue yellow orange violet` are re-tinted to these,
   so `color="green"` keeps working.
3. **Both schemes work.** Default is dark; light is a first-class toggle in the
   header. Never hard-code hex colours in components: use theme colour names or
   the `--vh-*` tokens below.
4. **Density like a dashboard, not a landing page.** 14px body, 12px meta,
   tables edge to edge inside cards, one page title per page.
5. **Same words, better clothes.** A restyle keeps every label, button text,
   aria-label and placeholder; the e2e suite and screen readers depend on them.

## Tokens (CSS variables from the resolver)

| token | use |
|---|---|
| `--vh-surface`, `--vh-surface-2` | card background, input/hover background |
| `--vh-sidebar` | sidebar and header background |
| `--vh-border`, `--vh-border-strong` | hairlines, dashed empty states |
| `--vh-accent-soft` | ember tint (active nav, focus ring, icon tiles) |
| `--vh-text-soft` | eyebrow / secondary text |
| `--vh-ember --vh-frost --vh-moss --vh-blood --vh-spirit` | accent colours |
| `--vh-text` / `--vh-text-soft` | Ink text on timber, and its dimmed variant |
| `--vh-parchment` / `--vh-parchment-2` | Paper surfaces: the map, empty states, the login backdrop |
| `--vh-ink` | Text on parchment |
| `--vh-elevation` | The one card shadow (borderless surfaces) |

Radius: cards `lg` (14px), controls `md` (10px). Spacing: page `xl`, inside
cards `lg`, between stacked cards `md`.

## Primitives (`web/src/ui`)

- `PageHeader` — eyebrow, title (+ `titleAddon` such as a `StatusPill`),
  description, `actions`. One per page, first thing in the page.
- `StatTile` — KPI tile (label, big value, hint, icon, optional `accent` bar).
  Use in a `SimpleGrid cols={{ base: 2, md: 4 }}` at the top of overview pages.
- `StatusPill` / `StatusDot` — state indicator with a glowing dot; `pulse` for
  live states (running, starting). Replaces `Badge variant="light"` for states.
- `SectionCard` — titled card (title, description, actions, `flush` for tables).
  Replaces bare `Paper` + `Title order={4}` blocks.
- `EmptyState` — dashed panel with icon tile, title, hint, action.
- `BrandMark` — the ember "V" mark.
- `.vh-eyebrow` — global class for small uppercase labels.

## Patterns

- **Tables** live in `SectionCard flush` and use `Table.ScrollContainer`;
  headers are uppercase and muted automatically. Row actions go right-aligned
  in the last cell as `ActionIcon variant="subtle"`.
- **Forms** are grouped into `SectionCard`s with a description; the submit
  button sits in a right-aligned `Group` at the bottom.
- **Destructive actions** use `color="red"` with `variant="light"`/`outline`
  and a confirm modal. Primary actions are the filled ember button; there is
  at most one per view.
- **Loading** uses `Skeleton` in the final layout's shape, not a spinner.
- **Notifications** on every mutation (`notifySuccess`/`notifyError`).
- **Mobile**: grids collapse to one column; tables scroll inside their container.

## Screenshots

`make build && PW_CHROMIUM=/path/to/chromium node web/e2e/screenshots.mjs`
seeds a realistic state against the fake game server and writes
`docs/screenshots/*.png` (add `--scheme light` for the light set).

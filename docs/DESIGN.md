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

## Accessibility

Target: WCAG 2.2 AA — verified with an automated pass (axe-core) over every
page in both colour schemes, plus a handful of manual fixes automated tools
can't catch (keyboard access, motion, hover-only content).

### Running the check

```
cd web && npm install --no-save --no-audit --no-fund axe-core@4.10.3   # once; never saved to package.json/lockfile
make build
node web/e2e/screenshots.mjs --axe --scheme dark  --out /tmp/axe-dark
node web/e2e/screenshots.mjs --axe --scheme light --out /tmp/axe-light
```

`--axe` runs axe-core (tags `wcag2a wcag2aa wcag21a wcag21aa wcag22aa`)
against every page right after its screenshot, prints a per-page violation
summary, writes `<out>/axe-report.json`, and exits 1 if any violation was
found anywhere. It needs axe-core installed as above — the script still runs
fine without `--axe` (and without axe-core present) since the import only
happens behind the flag. The app serves a strict `script-src 'self'` CSP
(`internal/api/router.go`), so the `--axe` Playwright contexts set
`bypassCSP: true` to let axe-core's `addScriptTag` inject at all; this has no
effect on rendering or the screenshots themselves.

### Contrast changes (WCAG 1.4.3)

`theme.ts` turns on `autoContrast: true` / `luminanceThreshold: 0.3`, so
filled buttons/badges pick black or white text from the swatch's own
luminance instead of a fixed colour. That alone cleared every *filled*-variant
failure — no filled shade in any colour tuple needed darkening.

What was left were Mantine's per-colour **outline**/**text** variants, which
default to shade 6 in the light scheme (the same shade `primaryShade.light`
uses for filled buttons) — fine against white, not against our warm
`--vh-surface` cards. Fixed by re-pointing the derived CSS variable at a
darker shade that already exists in the same tuple — no tuple hex values were
added or changed, light scheme only (dark scheme's own defaults already
passed the axe run untouched):

| CSS variable (light scheme) | old shade → new | old → new hex |
|---|---|---|
| `--mantine-color-dimmed` | stock gray-6 → `--vh-text-soft` (ink @ 65%) | `#868e96` → *(token, ≥4.7:1 on every surface)* |
| `--mantine-color-ember-outline` | ember-6 → ember-9 | `#e8950a` → `#7a4900` |
| `--mantine-color-red-outline` / `-blood-outline` | 6 → 7 | `#d93737` → `#c12a2a` |
| `--mantine-color-gray-outline` | 6 → 7 | `#868e96` → `#495057` |
| `--mantine-color-green-text` / `-moss-text` | 6 → 9 | `#48a45e` → `#216e37` |

`index.css`'s `.mantine-Anchor-root`: no frost shade clears 4.5:1 against
every surface a link can sit on (the parchment map card included), so links
now inherit the surrounding ink colour (already ≥4.7:1 everywhere) and get a
**permanent** underline instead of relying on colour for the hover-only one
(fixes `link-in-text-block` too). The same file's plain-link reset is now
`a:where(:not(.mantine-Anchor-root))` — `:where()` zeroes its specificity so
it can no longer outrank a Button/ActionIcon rendered `component={Link}` and
clobber its own (autoContrast-computed) text colour, which is what kept the
"New instance"/"Restart" buttons white-on-ember even after autoContrast
landed. `OverviewTab.tsx`'s map `Paper` (parchment background) now sets
`color: var(--vh-ink)` explicitly, mirroring `WorldCard`'s `PARCHMENT_VARS` —
without it, its text/links inherited the dark-scheme page ink instead of an
ink that works on parchment.

### The eight manual items

1. **Motion (2.3.3)** — `.statusDot[data-pulse]`'s pulse respects
   `prefers-reduced-motion: reduce`.
2. **Target size (2.5.8)** — the known-players note pencil and the
   global-key remove `CloseButton` are now `size="md"` (28px, ≥24px).
3. **Hover-only info (1.4.13/2.1.1)** — `Sparkline` takes an optional
   `label`; when given, it gets `role="img"`/`aria-label="<label>: <last
   value>"` instead of `aria-hidden`. `StatTile` and the Overview history
   rows pass their own label through, and the history rows also print the
   formatted last value as visible text next to the sparkline.
   `AccountPage`'s session browser column shows the full user agent
   (wrapped, `maw` capped) instead of a 60-char truncation + tooltip.
4. **Map keyboard access (2.1.1) / text alternative (1.1.1)** — the map
   viewport is `tabIndex={0} role="application"`; arrow keys pan, +/- zoom, 0
   resets, reusing the same `clamp`/`zoomAt` state and helpers the pointer
   handlers use (no pan/zoom maths changed). A visible `:focus-visible`
   outline was added. Each marker gets `role="img"` + `aria-label` (the
   tooltip text) so hover isn't the only way to identify one;
   `MapPlayerList` remains the text alternative for the player list.
5. **Skip link (2.4.1)** — `Shell.tsx` renders one as the first child inside
   `JobDrawerHost`; off-screen until focused, then pinned top-left above the
   header (`Shell.module.css`'s `.skipLink`); `AppShell.Main` got
   `id="main-content"` and `tabIndex={-1}`.
6. `ActivityIndicator`'s unread-jobs `Popover.Target` wrapped the decorative
   `Indicator` div instead of the `ActionIcon`, so the `aria-expanded`
   Popover adds landed on a roleless div (axe `aria-allowed-attr`) instead of
   the actual button; swapped which one wraps which.
7. `JobDrawer` passed both `title` and a redundant `aria-label`; Mantine
   already wires `title` to the dialog via `aria-labelledby`, and the extra
   `aria-label` landed on the roleless Drawer root div instead
   (axe `aria-prohibited-attr`).
8. `OverviewTab`'s two `Table.ScrollContainer`s (Recent jobs/events) get
   `scrollAreaProps={{ viewportProps: { tabIndex: 0 } }}` so the
   horizontally-scrolling region is keyboard-reachable when it overflows on
   narrow viewports (axe `scrollable-region-focusable` — a plain `tabIndex`
   prop lands on the `ScrollArea` root, not the inner viewport div that
   actually scrolls).

### Exceptions

Both schemes are axe-clean (0 violations) and there are no open exceptions.

One pattern axe cannot see — WCAG 1.4.13, content on hover or focus — was
found by surveying every `<Tooltip>` in `web/src`: a `Badge` whose tooltip is
the only place a text appears (the off-site backup failure reason in
`BackupsTable`, the "Not saved yet" explanation in `WorldsTable`, the delivery
error in `NotificationsCard`). The rule for that pattern: a non-interactive
element carrying tooltip-only information gets `tabIndex={0}` (so the tooltip
also opens on keyboard focus) and an `aria-label` that includes the text, or
the text is shown outright. A tooltip that merely repeats an icon button's
`aria-label` needs nothing.

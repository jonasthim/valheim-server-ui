# Design system — "Ashlands"

Everything is Mantine (no other UI kit or CSS framework); the look lives in
`web/src/theme/theme.ts`, `web/src/index.css` and the primitives in `web/src/ui/`.

> **Direction: Ashlands.** Named after the biome of black ash and embers: near-black neutral
> surfaces, one ember accent, and the parchment map as the single piece of material —
> everything else is quiet product chrome. No gradients, no glows, no card shadows (borders
> carry the structure); shadows are reserved for floating layers (menus, popovers, modals, the
> command palette, toasts). Geist for every chrome type, Geist Mono for logs, ids, code and
> addresses — no serif anywhere. Ember is exactly one filled button per view, the active nav
> bar, focus rings and link hover; never decoration, never status.

## Tokens

CSS variables from `theme.ts`'s `cssVariablesResolver`, values as shipped (dark first; light
mirrors it):

| Token | Dark | Light | Role |
|---|---|---|---|
| `--vh-bg` | `#0A0A0B` | `#FAFAFA` | page background |
| `--vh-surface` | `#111113` | `#FFFFFF` | cards, sidebar, top bar, modals |
| `--vh-surface-2` | `#18181B` | `#F4F4F5` | hover, raised rows, inputs |
| `--vh-border` | `#27272A` | `#E4E4E7` | every hairline (solid 1px, no alpha) |
| `--vh-border-strong` | `#3F3F46` | `#D4D4D8` | hovered/focused borders, strong dividers |
| `--vh-text` | `#FAFAFA` | `#18181B` | primary text |
| `--vh-text-soft` | `#A1A1AA` | `#52525B` | secondary text, labels |
| `--vh-text-faint` | `#8A8A94` | `#6B6B74` | meta, placeholders |
| `--vh-ember` | `#F5A20F` (ember-5) | `#C97C05` (ember-7) | rings, bars, icons, filled buttons (dark text on the fill) |
| `--vh-ember-text` | `#F5A20F` | `#7A4900` (ember-9) | ember used as text |
| `--vh-accent-soft` | `rgba(245,162,15,0.14)` | `rgba(201,124,5,0.14)` | focus glow, selection |
| `--vh-parchment` / `--vh-ink` | `#B9AD8C` / `#1A120A` | `#D8CCAA` / `#1A120A` | only inside the map viewport and map thumbnails |
| status hues | `--vh-moss #55A969`, `--vh-blood #DB4343`, `--vh-frost #3AAED5`, `--vh-straw #FFCF1F`, `--vh-spirit #7650E5` | same tuples | pills, dots, alerts (unchanged tuples) |

`--vh-text-faint` shipped as `#8A8A94` / `#6B6B74`, not the first draft's `#7A7A85` (4.18:1 on
surface-2) / `#71717A` (4.40:1 on surface-2) — both first-draft values fell short of the 4.5:1
AA text floor. The shipped pair clears ≥4.8:1 on surface-2 in both schemes (see Contrast below).

Rules: no gradients, no glows, no card shadows (borders carry structure); shadows only on
floating layers (menus, popovers, modals, palette, toasts). Ember: exactly one filled button per
view, the active nav bar, focus rings, link hover.

## Contrast (measured)

WCAG 2.x formula.

**Dark scheme**

| Foreground | Background | Ratio |
|---|---|---|
| `--vh-text` | surface | 18.1:1 |
| `--vh-text-soft` | surface | 7.4:1 |
| `--vh-text-soft` | surface-2 | 6.9:1 |
| `--vh-text-faint` | surface | 5.5:1 |
| `--vh-text-faint` | surface-2 | 5.2:1 |
| `--vh-text-faint` | hover (`#1F1F23`) | 4.8:1 |
| `--vh-ember-text` | surface | 9.0:1 |
| `--vh-ember-text` | bg | 9.5:1 |
| autoContrast dark text | ember-5 fill | 9.5:1 |

**Light scheme**

| Foreground | Background | Ratio |
|---|---|---|
| `--vh-text` | surface (white) | 17.7:1 |
| `--vh-text-soft` | surface | 7.7:1 |
| `--vh-text-soft` | surface-2 | 7.0:1 |
| `--vh-text-faint` | surface | 5.3:1 |
| `--vh-text-faint` | surface-2 | 4.8:1 |
| `--vh-ember-text` (ember-9) | surface | 7.6:1 |
| `--vh-ember-text` (ember-9) | surface-2 | 6.9:1 |
| ember-7 ring (non-text UI only) | surface | 3.3:1 |
| autoContrast dark text | ember-6 fill | 8.2:1 |

**Parchment** (fixed ink, both schemes)

| Foreground | Background | Ratio |
|---|---|---|
| `--vh-ink` | `#B9AD8C` (dark-scheme parchment) | 8.3:1 |
| `--vh-ink` | `#D8CCAA` (light-scheme parchment) | 11.6:1 |

## Type

Geist (variable, weights 100–900) for everything in the chrome; Geist Mono for logs, ids, code,
addresses. No serif anywhere, no uppercase labels, no letter-spaced eyebrows.

| Use | Size / weight | Notes |
|---|---|---|
| Body | 14 / 400 | |
| Secondary | 13 / 400 | |
| Meta | 12 / 400 | |
| Page title | 20 / 600 | `letter-spacing -0.01em` |
| Section title | 14 / 600 | sentence case |
| Stat value | 20 / 600 | tabular figures |
| Hero status | 18 / 600 | |
| Heading h1 | 20 | |
| Heading h2 | 18 | |
| Heading h3 | 16 | |
| Heading h4 | 14 | |

## Surfaces, radius, spacing, density

Radius: controls 6px (`sm`), cards 8px (`md`), modals/palette 10px (`lg`), pills 999px. 4px
grid. Page padding 24px (16px on phone). Card padding 16px; card header 12px 16px with a
hairline below. Section gap 16px. Table rows 36px (header 32px). Top bar 48px. Sidebar 232px,
collapsible to a 56px icon rail. Content max-width 1320px; forms ≤ 720px. Inputs and buttons
32px (`size="sm"`), `xs` = 28px inside table rows.

## Principles

1. Neutral surfaces, one accent.
2. Borders, not shadows.
3. One sans at 14/13/12, semibold titles in sentence case.
4. Product density: 36px rows, no empty cards, strips instead of tiles.
5. Keyboard-first: ⌘K, shortcuts, visible focus rings, `aria-label`s preserved.
6. The map is the only material.

## Primitives (`web/src/ui`)

- `PageHeader` — title, `titleAddon`, description, `actions`, `rule`. One per page, first thing
  in the page.
- `SectionCard` — titled card (title, description, actions, `flush` for tables). Replaces bare
  `Paper` + `Title order={4}` blocks.
- `StatStrip` (`web/src/ui/StatStrip.tsx`) —
  `StatStrip({ items, cols?, minCellWidth = 140, loading?, className? })`; item
  `{ key?: string; label: string; value: ReactNode; hint?: ReactNode; icon?: ReactNode; action?:
  ReactNode; tone?: 'default'|'accent'|'danger'|'success'; accent?: string; spark?: number[];
  sparkFormat?: (v: number) => string }`. `tone` maps to a 3px left bar (`accent` →
  `var(--vh-ember)`, `danger` → `var(--vh-blood)`, `success` → `var(--vh-moss)`); `accent` is a
  raw CSS colour override. `cols` fixes the column count; otherwise cells auto-fit at
  `minCellWidth`. `loading` renders skeleton cells. `action` renders right of the label (an
  `ActionIcon size="sm"`), replacing `icon`. Supersedes a `SimpleGrid` of `StatTile`s on
  overview-style pages.
- `StatTile` (legacy) — KPI tile (label, big value, hint, icon, optional `accent` bar); kept
  where a page hasn't moved to `StatStrip` yet.
- `StatusPill` / `StatusDot` — state indicator with a glowing dot; `pulse` for live states
  (running, starting). Replaces `Badge variant="light"` for states.
- `EmptyState` — dashed neutral panel: icon tile, one sentence, optional action. No variants.
- `LoadError` — inline failed-fetch state with a retry action.
- `Sparkline` — small inline trend line; takes an optional `label` for an accessible name (see
  Accessibility).
- `BrandMark` — the ember "V" mark.
- `ErrorBoundary` — top-level render-error fallback.

### Shipped in Tracks B/C

- `DataTable` (Track C)
- `Toolbar` (Track C)
- `StickySaveBar` (Track B)

## Patterns

- **Tables** live in a `SectionCard flush` and render via `DataTable`.
- **Forms**: the page owns the form (`useInstanceConfigForm` / `useForm<Settings>`) and renders
  a `<form id="instance-config-form">` / `<form id="settings-form">` element; the save control
  sits outside the form, associated via `form={formId}`. Grouped into `SectionCard`s ≤ 720px.
  The interim `FormFooter` is replaced by `StickySaveBar` + `useUnsavedChanges` once it ships.
- **Banners** = `Alert` defaults: light variant, 8% tint, 1px hue border, neutral text, 16px
  icon. Hue mapping: info → `color="frost"`, warning → `"straw"`, danger → `"blood"`, success →
  `"moss"`, accent → `"ember"`. Actions inside an `Alert`: `<Group justify="space-between"
  align="center" wrap="wrap"><Text size="sm">…</Text><Group gap="xs">…buttons…</Group></Group>`
  as `children`.
- **Toasts** only via `web/src/lib/notify.ts` (`notifySuccess`, `notifyError`, `notifyWarning`,
  `notifyInfo`); provider bottom-right, 3.5 s.
- **Loading** = `Skeleton`.
- **Empty** = `EmptyState`.
- **Links** inherit the surrounding text colour with a permanent underline; ember on hover.
- **Focus** = 2px ember ring.
- **Motion** honours `prefers-reduced-motion` globally.

## Screenshots

```
make build && PW_CHROMIUM=/path/to/chromium node web/e2e/screenshots.mjs
```

seeds a realistic state against the fake game server and writes `docs/screenshots/*.png` (add
`--scheme light` for the light set, `--width 390` for the phone-width set).

## Accessibility

Target: WCAG 2.2 AA — verified with an automated pass (axe-core) over every page in both colour
schemes, plus a handful of manual fixes automated tools can't catch (keyboard access, motion,
hover-only content).

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

`theme.ts` turns on `autoContrast: true` / `luminanceThreshold: 0.28` (lowered from an initial
0.3 so moss-6, `#48a45e` at L 0.287, gets dark text — 5.7:1 — instead of white at 3.1:1), so
filled buttons/badges pick black or white text from the swatch's own luminance instead of a
fixed colour.

That covers every *filled*-variant pairing. What's left are Mantine's per-colour **outline**/
**text** variants (which default to a fixed shade — fine against white, not against
`--vh-surface`), `dimmed`/`placeholder`, and, in the dark scheme, `gray`'s `light`/`light-hover`
tint and the shared `error` colour. `cssVariablesResolver` repoints these per scheme:

**Light scheme remap**

| CSS variable | Value |
|---|---|
| `--mantine-color-default-border` | `var(--vh-border)` |
| `--mantine-color-default-hover` | `var(--vh-surface-2)` |
| `--mantine-color-placeholder` | `var(--vh-text-faint)` |
| `--mantine-color-dimmed` | `var(--vh-text-soft)` |
| `--mantine-color-ember-outline` / `--mantine-color-orange-outline` | `var(--mantine-color-ember-9)` |
| `--mantine-color-ember-text` / `--mantine-color-orange-text` | `var(--mantine-color-ember-9)` |
| `--mantine-color-red-outline` / `--mantine-color-blood-outline` | `var(--mantine-color-red-7)` |
| `--mantine-color-red-text` / `--mantine-color-blood-text` | `var(--mantine-color-red-7)` |
| `--mantine-color-gray-outline` | `var(--mantine-color-gray-7)` |
| `--mantine-color-green-text` / `--mantine-color-moss-text` | `var(--mantine-color-green-9)` |
| `--mantine-color-blue-text` / `--mantine-color-frost-text` | `var(--mantine-color-blue-9)` |
| `--mantine-color-blue-outline` / `--mantine-color-frost-outline` | `var(--mantine-color-blue-9)` |
| `--mantine-color-straw-text` / `--mantine-color-yellow-text` | `var(--mantine-color-ember-9)` |
| `--mantine-color-straw-outline` / `--mantine-color-yellow-outline` | `var(--mantine-color-ember-9)` |
| `--mantine-color-straw-light-color` / `--mantine-color-yellow-light-color` | `var(--mantine-color-ember-9)` |
| `--mantine-color-error` | `var(--mantine-color-red-7)` |

No straw shade clears 4.5:1 on white (straw-9 `#ae8900` is 3.3:1), so straw/yellow's text,
outline and light-variant colour all route to ember-9, which reads as a dark amber.

**Dark scheme block**

| CSS variable | Value | Why |
|---|---|---|
| `--mantine-color-gray-light` | `#1F1F23` | Mantine's dark `light` variant darkens zinc-9 to near-black, invisible on `#111113` cards |
| `--mantine-color-gray-light-hover` | `#27272A` | pairs with the above |
| `--mantine-color-gray-outline` | `var(--mantine-color-gray-4)` | same reasoning as the light-scheme gray-outline fix |
| `--mantine-color-error` | `var(--mantine-color-red-4)` | dark scheme's error, red-8 `#ac2323`, is 2.7:1 on `#111113`; red-4 is 5.2:1 |

### The eight manual items

1. **Motion (2.3.3)** — a global `@media (prefers-reduced-motion: reduce)` block in `index.css`
   zeroes animation/transition duration and disables smooth scrolling for every element
   (`*, *::before, *::after`), covering `.statusDot[data-pulse]`'s pulse along with all other
   motion.
2. **Target size (2.5.8)** — the known-players note pencil and the global-key remove
   `CloseButton` are now `size="md"` (28px, ≥24px).
3. **Hover-only info (1.4.13/2.1.1)** — `Sparkline` takes an optional `label`; when given, it
   gets `role="img"`/`aria-label="<label>: <last value>"` instead of `aria-hidden`. `StatTile`
   and the Overview history rows pass their own label through, and the history rows also print
   the formatted last value as visible text next to the sparkline. `AccountPage`'s session
   browser column shows the full user agent (wrapped, `maw` capped) instead of a 60-char
   truncation + tooltip.
4. **Map keyboard access (2.1.1) / text alternative (1.1.1)** — the map viewport is
   `tabIndex={0} role="application"`; arrow keys pan, +/- zoom, 0 resets, reusing the same
   `clamp`/`zoomAt` state and helpers the pointer handlers use (no pan/zoom maths changed). A
   visible `:focus-visible` outline was added. Each marker gets `role="img"` + `aria-label` (the
   tooltip text) so hover isn't the only way to identify one; `MapPlayerList` remains the text
   alternative for the player list.
5. **Skip link (2.4.1)** — `Shell.tsx` renders one as the first child inside `JobDrawerHost`;
   off-screen until focused, then pinned top-left above the header (`Shell.module.css`'s
   `.skipLink`); `AppShell.Main` got `id="main-content"` and `tabIndex={-1}`.
6. `ActivityIndicator`'s unread-jobs `Popover.Target` wrapped the decorative `Indicator` div
   instead of the `ActionIcon`, so the `aria-expanded` Popover adds landed on a roleless div
   (axe `aria-allowed-attr`) instead of the actual button; swapped which one wraps which.
7. `JobDrawer` passed both `title` and a redundant `aria-label`; Mantine already wires `title` to
   the dialog via `aria-labelledby`, and the extra `aria-label` landed on the roleless Drawer
   root div instead (axe `aria-prohibited-attr`).
8. `OverviewTab`'s two `Table.ScrollContainer`s (Recent jobs/events) get
   `scrollAreaProps={{ viewportProps: { tabIndex: 0 } }}` so the horizontally-scrolling region is
   keyboard-reachable when it overflows on narrow viewports (axe `scrollable-region-focusable` —
   a plain `tabIndex` prop lands on the `ScrollArea` root, not the inner viewport div that
   actually scrolls).

### Exceptions

Both schemes are axe-clean (0 violations) and there are no open exceptions.

One pattern axe cannot see — WCAG 1.4.13, content on hover or focus — was found by surveying
every `<Tooltip>` in `web/src`: a `Badge` whose tooltip is the only place a text appears (the
off-site backup failure reason in `BackupsTable`, the "Not saved yet" explanation in
`WorldsTable`, the delivery error in `NotificationsCard`). The rule for that pattern: a
non-interactive element carrying tooltip-only information gets `tabIndex={0}` (so the tooltip
also opens on keyboard focus) and an `aria-label` that includes the text, or the text is shown
outright. A tooltip that merely repeats an icon button's `aria-label` needs nothing.

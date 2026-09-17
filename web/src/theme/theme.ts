// Design system for Valheim Server UI: a dark-first, modern SaaS look with
// Valheim accents (ember gold as primary, frost blue, moss green, blood red,
// spirit purple) on warm timber neutrals. See docs/DESIGN.md.
import {
  ActionIcon,
  Badge,
  Button,
  Card,
  Modal,
  NumberInput,
  Paper,
  Select,
  Table,
  Tabs,
  TextInput,
  Title,
  Tooltip,
  createTheme,
  type CSSVariablesResolver,
  type MantineColorsTuple,
} from '@mantine/core'
import uiClasses from '../ui/ui.module.css'

// Ember: Valheim's logo gold / torchlight. Primary action colour.
const ember: MantineColorsTuple = [
  '#fff7e6',
  '#ffedc7',
  '#ffd98f',
  '#ffc453',
  '#ffb226',
  '#f5a20f',
  '#e8950a',
  '#c97c05',
  '#a26302',
  '#7a4900',
]

// Iron: warm timber and peat neutrals — the longhouse palette — used for the
// dark scheme (Mantine reads the
// `dark` tuple for body, surfaces, borders and dimmed text in dark mode).
const iron: MantineColorsTuple = [
  '#EFE6CC', // 0 text
  '#C9C0A9', // 1
  '#A39A86', // 2 dimmed text
  '#7E7464', // 3
  '#584E41', // 4 borders
  '#32281E', // 5 hover surfaces
  '#241D16', // 6 surfaces / cards
  '#17130F', // 7 body
  '#120F0B', // 8 sidebar / header
  '#0E0C08', // 9
]

// Frost: Valheim frost / Mistlands glow. Informational.
const frost: MantineColorsTuple = [
  '#e9f7fc',
  '#d3edf6',
  '#a5dbec',
  '#75c8e2',
  '#4fb8da',
  '#3aaed5',
  '#2ca9d4',
  '#1c93bc',
  '#0a83a8',
  '#007193',
]

// Moss: mossy meadows green. Success / running.
const moss: MantineColorsTuple = [
  '#eaf8ee',
  '#d9edde',
  '#b4d9bd',
  '#8bc59a',
  '#6ab47c',
  '#55a969',
  '#48a45e',
  '#398f4e',
  '#2f7f44',
  '#216e37',
]

// Blood: failure / destructive.
const blood: MantineColorsTuple = [
  '#ffebeb',
  '#fbd6d6',
  '#f1acac',
  '#e77f7f',
  '#df5a5a',
  '#db4343',
  '#d93737',
  '#c12a2a',
  '#ac2323',
  '#961818',
]

// Spirit: wraith purple. Used sparingly for "mods" and secondary accents.
const spirit: MantineColorsTuple = [
  '#f3efff',
  '#e3dbfb',
  '#c4b4f4',
  '#a48aee',
  '#8867e8',
  '#7650e5',
  '#6d44e4',
  '#5c36cb',
  '#522fb6',
  '#4627a0',
]

// Straw: pending / warning (distinct from the ember primary).
const straw: MantineColorsTuple = [
  '#fffbe1',
  '#fff6cc',
  '#ffeb9b',
  '#ffe066',
  '#ffd63b',
  '#ffcf1f',
  '#ffcb0b',
  '#e3b300',
  '#ca9f00',
  '#ae8900',
]

export const theme = createTheme({
  primaryColor: 'ember',
  primaryShade: { light: 6, dark: 5 },
  // WCAG 1.4.3: pick dark or light text on filled swatches (buttons, badges,
  // filled ActionIcons) by the swatch's own luminance instead of always
  // using the theme's primary-contrast colour. See docs/DESIGN.md
  // "Accessibility" for the values this was tuned against.
  autoContrast: true,
  luminanceThreshold: 0.3,
  colors: {
    ember,
    dark: iron,
    frost,
    moss,
    blood,
    spirit,
    straw,
    // Re-tint the stock names so existing `color="green"` etc. keep working
    // but sit in the same palette.
    green: moss,
    red: blood,
    blue: frost,
    yellow: straw,
    orange: ember,
    violet: spirit,
  },
  fontFamily: '"Source Sans 3", system-ui, sans-serif',
  fontFamilyMonospace: '"JetBrains Mono", ui-monospace, monospace',
  headings: {
    fontFamily: '"Valheim Display", "Source Sans 3", serif',
    fontWeight: '700',
    sizes: {
      h1: { fontSize: '30px', lineHeight: '1.15' },
      h2: { fontSize: '24px', lineHeight: '1.2' },
      h3: { fontSize: '19px', lineHeight: '1.3', fontWeight: '600' },
      h4: { fontSize: '1rem', lineHeight: '1.4' },
    },
  },
  fontSizes: { xs: '12px', sm: '13px', md: '15px', lg: '17px', xl: '19px' },
  defaultRadius: 'md',
  radius: { xs: '4px', sm: '6px', md: '10px', lg: '14px', xl: '20px' },
  cursorType: 'pointer',
  components: {
    Paper: Paper.extend({
      classNames: { root: uiClasses.elevated },
      defaultProps: { radius: 'lg', withBorder: false },
    }),
    Card: Card.extend({
      classNames: { root: uiClasses.elevated },
      defaultProps: { radius: 'lg', withBorder: false, padding: 'lg' },
    }),
    Modal: Modal.extend({ defaultProps: { radius: 'lg', overlayProps: { backgroundOpacity: 0.6, blur: 4 } } }),
    Button: Button.extend({ defaultProps: { radius: 'sm' } }),
    ActionIcon: ActionIcon.extend({ defaultProps: { radius: 'md' } }),
    Badge: Badge.extend({ defaultProps: { radius: 'sm' }, styles: { root: { textTransform: 'none', fontWeight: 600, letterSpacing: 0 } } }),
    TextInput: TextInput.extend({ defaultProps: { radius: 'sm' } }),
    Select: Select.extend({ defaultProps: { radius: 'sm' } }),
    NumberInput: NumberInput.extend({ defaultProps: { radius: 'sm' } }),
    Tooltip: Tooltip.extend({ defaultProps: { withArrow: true, openDelay: 300 } }),
    Tabs: Tabs.extend({ defaultProps: { radius: 'md' } }),
    Table: Table.extend({ defaultProps: { verticalSpacing: 'sm', horizontalSpacing: 'md', highlightOnHover: true } }),
    Title: Title.extend({
      styles: (theme, props) => (props.order === 3 ? { root: { fontFamily: theme.fontFamily } } : {}),
    }),
  },
})

// Scheme-specific tokens the components and CSS rely on.
export const cssVariablesResolver: CSSVariablesResolver = (t) => ({
  variables: {
    '--vh-ember': t.colors.ember[5],
    '--vh-frost': t.colors.frost[5],
    '--vh-moss': t.colors.moss[5],
    '--vh-blood': t.colors.blood[5],
    '--vh-spirit': t.colors.spirit[5],
  },
  light: {
    '--mantine-color-body': '#E9DFC4',
    '--mantine-color-text': '#1A120A',
    '--vh-surface': '#F3ECD8',
    '--vh-surface-2': '#FAF5E6',
    '--vh-sidebar': '#DED2B3',
    '--vh-text': '#1A120A',
    '--vh-text-soft': 'rgba(26,18,10,0.65)',
    '--vh-border': 'rgba(26,18,10,0.12)',
    '--vh-border-strong': 'rgba(26,18,10,0.22)',
    '--vh-parchment': '#D8CCAA',
    '--vh-parchment-2': '#E4DABB',
    '--vh-ink': '#1A120A',
    '--vh-elevation': '0 8px 20px -14px rgba(26,18,10,0.35), 0 0 0 1px var(--vh-border)',
    '--vh-accent-soft': 'rgba(232, 149, 10, 0.12)',
    // WCAG 1.4.3: Mantine's own default (stock gray-6, ~2-3:1 here) is too
    // light against every light-scheme surface; reuse the already-tuned
    // --vh-text-soft ink (≥4.7:1 on body/surface/parchment — see
    // docs/DESIGN.md). Dark scheme's default (our iron[2]) already passes.
    '--mantine-color-dimmed': 'var(--vh-text-soft)',
    // WCAG 1.4.3: Mantine's per-colour "outline"/"text" variants default to
    // shade 6 in light mode (the same shade `primaryShade.light` uses for
    // filled buttons), which only clears 4.5:1 on our warm --vh-surface /
    // --vh-surface-2 card backgrounds for ember at shade 9 and blood/gray at
    // shade 7 — moss needs 9 even for the plain (non-outline) text colour.
    // Re-pointing to an existing, darker index in the same tuple (no new hex
    // values); dark scheme's shade-4/5 equivalents already pass. See
    // docs/DESIGN.md "Accessibility".
    '--mantine-color-ember-outline': 'var(--mantine-color-ember-9)',
    '--mantine-color-red-outline': 'var(--mantine-color-red-7)',
    '--mantine-color-blood-outline': 'var(--mantine-color-blood-7)',
    '--mantine-color-gray-outline': 'var(--mantine-color-gray-7)',
    '--mantine-color-green-text': 'var(--mantine-color-green-9)',
    '--mantine-color-moss-text': 'var(--mantine-color-moss-9)',
  },
  dark: {
    '--mantine-color-body': iron[7],
    '--vh-surface': iron[6],
    '--vh-surface-2': iron[5],
    '--vh-sidebar': iron[8],
    '--vh-text': '#EFE6CC',
    '--vh-text-soft': 'rgba(239,230,204,0.62)',
    '--vh-border': 'rgba(239,230,204,0.10)',
    '--vh-border-strong': 'rgba(239,230,204,0.18)',
    '--vh-parchment': '#B9AD8C',
    '--vh-parchment-2': '#CFC3A3',
    '--vh-ink': '#1A120A',
    '--vh-elevation': '0 10px 24px -14px rgba(0,0,0,0.6), 0 0 0 1px var(--vh-border)',
    '--vh-accent-soft': 'rgba(245, 162, 15, 0.14)',
  },
})

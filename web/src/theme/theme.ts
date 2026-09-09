// Design system for Valheim Server UI: a dark-first, modern SaaS look with
// Valheim accents (ember gold as primary, frost blue, moss green, blood red,
// spirit purple) on cool "iron" neutrals. See docs/DESIGN.md.
import {
  ActionIcon,
  Badge,
  Button,
  Card,
  Modal,
  Paper,
  Table,
  Tabs,
  Tooltip,
  createTheme,
  type CSSVariablesResolver,
  type MantineColorsTuple,
} from '@mantine/core'

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

// Iron: cool slate neutrals used for the dark scheme (Mantine reads the
// `dark` tuple for body, surfaces, borders and dimmed text in dark mode).
const iron: MantineColorsTuple = [
  '#d5dbe3', // 0 text
  '#b9c2cd', // 1
  '#8d98a7', // 2 dimmed text
  '#66717f', // 3
  '#2a3340', // 4 borders
  '#1f2733', // 5 hover surfaces
  '#151b24', // 6 surfaces / cards
  '#0e1219', // 7 body
  '#0a0d13', // 8 sidebar / header
  '#06080c', // 9
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
  fontFamily:
    'Inter, "SF Pro Text", "Segoe UI Variable", "Segoe UI", system-ui, -apple-system, Roboto, "Helvetica Neue", Arial, sans-serif',
  fontFamilyMonospace: '"JetBrains Mono", ui-monospace, SFMono-Regular, Menlo, Consolas, monospace',
  headings: {
    fontFamily:
      'Inter, "SF Pro Display", "Segoe UI Variable Display", "Segoe UI", system-ui, -apple-system, Roboto, sans-serif',
    fontWeight: '650',
    sizes: {
      h1: { fontSize: '2rem', lineHeight: '1.2' },
      h2: { fontSize: '1.5rem', lineHeight: '1.25' },
      h3: { fontSize: '1.125rem', lineHeight: '1.3' },
      h4: { fontSize: '1rem', lineHeight: '1.4' },
    },
  },
  defaultRadius: 'md',
  radius: { xs: '4px', sm: '6px', md: '10px', lg: '14px', xl: '20px' },
  cursorType: 'pointer',
  components: {
    Paper: Paper.extend({ defaultProps: { radius: 'lg', withBorder: true } }),
    Card: Card.extend({ defaultProps: { radius: 'lg', withBorder: true, padding: 'lg' } }),
    Modal: Modal.extend({ defaultProps: { radius: 'lg', overlayProps: { backgroundOpacity: 0.6, blur: 4 } } }),
    Button: Button.extend({ defaultProps: { radius: 'md' } }),
    ActionIcon: ActionIcon.extend({ defaultProps: { radius: 'md' } }),
    Badge: Badge.extend({ defaultProps: { radius: 'sm' }, styles: { root: { textTransform: 'none', fontWeight: 600, letterSpacing: 0 } } }),
    Tooltip: Tooltip.extend({ defaultProps: { withArrow: true, openDelay: 300 } }),
    Tabs: Tabs.extend({ defaultProps: { radius: 'md' } }),
    Table: Table.extend({ defaultProps: { verticalSpacing: 'sm', horizontalSpacing: 'md', highlightOnHover: true } }),
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
    '--mantine-color-body': '#f5f6f8',
    '--vh-surface': '#ffffff',
    '--vh-surface-2': '#f0f2f5',
    '--vh-sidebar': '#ffffff',
    '--vh-border': 'rgba(16, 24, 40, 0.10)',
    '--vh-border-strong': 'rgba(16, 24, 40, 0.18)',
    '--vh-glow-a': 'rgba(245, 162, 15, 0.14)',
    '--vh-glow-b': 'rgba(44, 169, 212, 0.10)',
    '--vh-accent-soft': 'rgba(232, 149, 10, 0.12)',
    '--vh-text-soft': '#5b6778',
  },
  dark: {
    '--mantine-color-body': iron[7],
    '--vh-surface': iron[6],
    '--vh-surface-2': iron[5],
    '--vh-sidebar': iron[8],
    '--vh-border': 'rgba(213, 219, 227, 0.08)',
    '--vh-border-strong': 'rgba(213, 219, 227, 0.16)',
    '--vh-glow-a': 'rgba(245, 162, 15, 0.13)',
    '--vh-glow-b': 'rgba(44, 169, 212, 0.10)',
    '--vh-accent-soft': 'rgba(245, 162, 15, 0.14)',
    '--vh-text-soft': iron[2],
  },
})

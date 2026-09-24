// Design system for Valheim Server UI: "Ashlands" — near-black neutral
// surfaces, one ember accent, and the parchment map as the only warm
// material; everything else is quiet product chrome. Geist for chrome
// type, Geist Mono for logs, ids and code. See docs/DESIGN.md.
import {
  ActionIcon,
  Alert,
  Autocomplete,
  Badge,
  Button,
  Card,
  Menu,
  Modal,
  MultiSelect,
  NativeSelect,
  Notification,
  NumberInput,
  Paper,
  PasswordInput,
  Popover,
  SegmentedControl,
  Select,
  Skeleton,
  Table,
  Tabs,
  TagsInput,
  Textarea,
  TextInput,
  Tooltip,
  createTheme,
  type CSSVariablesResolver,
  type MantineColorsTuple,
} from '@mantine/core'

const FONT_SANS = '"Geist", system-ui, -apple-system, "Segoe UI", sans-serif'
const FONT_MONO = '"Geist Mono", ui-monospace, SFMono-Regular, Menlo, monospace'

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

// Ash: dark-scheme neutrals. Mantine reads fixed indexes of `dark`:
// 0 text, 2 dimmed, 3 placeholder, 4 default border, 5 default hover,
// 6 default control/input background, 7 body.
const ash: MantineColorsTuple = [
  '#FAFAFA', // 0 text           (--vh-text)
  '#D4D4D8', // 1
  '#A1A1AA', // 2 dimmed         (--vh-text-soft)
  '#8A8A94', // 3 placeholder    (--vh-text-faint)
  '#27272A', // 4 default border (--vh-border)
  '#1F1F23', // 5 default hover
  '#18181B', // 6 default/input  (--vh-surface-2)
  '#0A0A0B', // 7 body           (--vh-bg)
  '#111113', // 8 surface        (--vh-surface)
  '#050506', // 9
]
// Zinc: the same neutral family for color="gray" and Mantine's light-scheme
// defaults (gray-5 placeholder, gray-6 dimmed, gray-9 tooltip).
const zinc: MantineColorsTuple = [
  '#FAFAFA', '#F4F4F5', '#E4E4E7', '#D4D4D8', '#A1A1AA',
  '#71717A', '#52525B', '#3F3F46', '#27272A', '#18181B',
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
  // WCAG 1.4.3: filled swatches pick dark/light text from their own luminance.
  // 0.28 (was 0.3) so moss-6 (#48a45e, L 0.287) gets dark text (5.7:1), not white (3.1:1).
  autoContrast: true,
  luminanceThreshold: 0.28,
  respectReducedMotion: true,
  black: '#18181B',
  colors: {
    ember, dark: ash, gray: zinc, frost, moss, blood, spirit, straw,
    green: moss, red: blood, blue: frost, yellow: straw, orange: ember, violet: spirit,
  },
  fontFamily: FONT_SANS,
  fontFamilyMonospace: FONT_MONO,
  headings: {
    fontFamily: FONT_SANS,
    fontWeight: '600',
    sizes: {
      h1: { fontSize: '20px', lineHeight: '1.3' },
      h2: { fontSize: '18px', lineHeight: '1.3' },
      h3: { fontSize: '16px', lineHeight: '1.4' },
      h4: { fontSize: '14px', lineHeight: '1.45' },
      h5: { fontSize: '13px', lineHeight: '1.45' },
      h6: { fontSize: '12px', lineHeight: '1.45' },
    },
  },
  fontSizes: { xs: '12px', sm: '13px', md: '14px', lg: '16px', xl: '18px' },
  lineHeights: { xs: '1.4', sm: '1.45', md: '1.5', lg: '1.5', xl: '1.5' },
  defaultRadius: 'md',
  radius: { xs: '4px', sm: '6px', md: '8px', lg: '10px', xl: '12px' },
  cursorType: 'pointer',
  components: {
    Paper: Paper.extend({ defaultProps: { radius: 'md', withBorder: true, shadow: 'none' } }),
    Card: Card.extend({ defaultProps: { radius: 'md', withBorder: true, shadow: 'none', padding: 'md' } }),
    Modal: Modal.extend({ defaultProps: { radius: 'lg', shadow: 'xl', centered: true, overlayProps: { backgroundOpacity: 0.6 } } }),
    Menu: Menu.extend({ defaultProps: { shadow: 'md', radius: 'md' } }),
    Popover: Popover.extend({ defaultProps: { shadow: 'md', radius: 'md' } }),
    Button: Button.extend({ defaultProps: { radius: 'sm', size: 'sm' }, styles: { root: { fontWeight: 500 } } }),
    ActionIcon: ActionIcon.extend({ defaultProps: { radius: 'sm' } }),
    Badge: Badge.extend({
      defaultProps: { radius: 'sm', variant: 'light' },
      styles: { root: { textTransform: 'none', fontWeight: 500, letterSpacing: 0 } },
    }),
    TextInput: TextInput.extend({ defaultProps: { radius: 'sm', size: 'sm' } }),
    PasswordInput: PasswordInput.extend({ defaultProps: { radius: 'sm', size: 'sm' } }),
    NumberInput: NumberInput.extend({ defaultProps: { radius: 'sm', size: 'sm' } }),
    Select: Select.extend({ defaultProps: { radius: 'sm', size: 'sm' } }),
    NativeSelect: NativeSelect.extend({ defaultProps: { radius: 'sm', size: 'sm' } }),
    MultiSelect: MultiSelect.extend({ defaultProps: { radius: 'sm', size: 'sm' } }),
    Autocomplete: Autocomplete.extend({ defaultProps: { radius: 'sm', size: 'sm' } }),
    TagsInput: TagsInput.extend({ defaultProps: { radius: 'sm', size: 'sm' } }),
    Textarea: Textarea.extend({ defaultProps: { radius: 'sm', size: 'sm' } }),
    SegmentedControl: SegmentedControl.extend({ defaultProps: { radius: 'sm' } }),
    Skeleton: Skeleton.extend({ defaultProps: { radius: 'sm' } }),
    Tooltip: Tooltip.extend({ defaultProps: { withArrow: false, openDelay: 300, radius: 'sm' } }),
    Tabs: Tabs.extend({ defaultProps: { radius: 'sm' } }),
    Table: Table.extend({ defaultProps: { verticalSpacing: 'xs', horizontalSpacing: 'md', highlightOnHover: true } }),
    Notification: Notification.extend({ defaultProps: { withBorder: true, radius: 'md' } }),
    // Banners: tinted 8% with a 1px border of the hue. Title and body stay neutral
    // text (straw-9 is only 3.3:1 on white); the icon carries the hue: shade 9 in
    // light, shade 4 in dark (every tuple's 4 is ≥ 4.6:1 on #111113).
    Alert: Alert.extend({
      defaultProps: { variant: 'light', radius: 'md' },
      vars: (theme, props) => {
        const c = props.color && theme.colors[props.color] ? props.color : theme.primaryColor
        return {
          root: {
            '--alert-bg': `color-mix(in srgb, var(--mantine-color-${c}-filled) 8%, var(--vh-surface))`,
            '--alert-bd': `1px solid color-mix(in srgb, var(--mantine-color-${c}-filled) 40%, var(--vh-border))`,
            '--alert-color': `light-dark(var(--mantine-color-${c}-9), var(--mantine-color-${c}-4))`,
          },
        }
      },
      styles: {
        label: { color: 'var(--mantine-color-text)' },
        message: { color: 'var(--mantine-color-text)' },
      },
    }),
  },
})

// Scheme-specific tokens the components and CSS rely on.
export const cssVariablesResolver: CSSVariablesResolver = (t) => ({
  variables: {
    '--vh-frost': t.colors.frost[5],
    '--vh-moss': t.colors.moss[5],
    '--vh-blood': t.colors.blood[5],
    '--vh-spirit': t.colors.spirit[5],
    '--vh-straw': t.colors.straw[5],
  },
  light: {
    '--mantine-color-body': '#FAFAFA',
    '--mantine-color-text': '#18181B',
    '--vh-bg': '#FAFAFA',
    '--vh-surface': '#FFFFFF',
    '--vh-surface-2': '#F4F4F5',
    '--vh-sidebar': '#FFFFFF',
    '--vh-border': '#E4E4E7',
    '--vh-border-strong': '#D4D4D8',
    '--vh-text': '#18181B',
    '--vh-text-soft': '#52525B',
    '--vh-text-faint': '#6B6B74',
    '--vh-ember': t.colors.ember[7],
    '--vh-ember-text': t.colors.ember[9],
    '--vh-accent-soft': 'rgba(201, 124, 5, 0.14)',
    '--vh-parchment': '#D8CCAA',
    '--vh-parchment-2': '#E4DABB',
    '--vh-ink': '#1A120A',
    '--mantine-color-default-border': 'var(--vh-border)',
    '--mantine-color-default-hover': 'var(--vh-surface-2)',
    '--mantine-color-placeholder': 'var(--vh-text-faint)',
    '--mantine-color-dimmed': 'var(--vh-text-soft)',
    // WCAG 1.4.3 remaps: Mantine's per-colour text/outline variants default to
    // shade 6 in light (too light on white). See docs/DESIGN.md.
    '--mantine-color-ember-outline': 'var(--mantine-color-ember-9)',
    '--mantine-color-orange-outline': 'var(--mantine-color-ember-9)',
    '--mantine-color-ember-text': 'var(--mantine-color-ember-9)',
    '--mantine-color-orange-text': 'var(--mantine-color-ember-9)',
    '--mantine-color-red-outline': 'var(--mantine-color-red-7)',
    '--mantine-color-blood-outline': 'var(--mantine-color-blood-7)',
    '--mantine-color-red-text': 'var(--mantine-color-red-7)',
    '--mantine-color-blood-text': 'var(--mantine-color-blood-7)',
    '--mantine-color-gray-outline': 'var(--mantine-color-gray-7)',
    '--mantine-color-green-text': 'var(--mantine-color-green-9)',
    '--mantine-color-moss-text': 'var(--mantine-color-moss-9)',
    '--mantine-color-blue-text': 'var(--mantine-color-blue-9)',
    '--mantine-color-frost-text': 'var(--mantine-color-frost-9)',
    '--mantine-color-blue-outline': 'var(--mantine-color-blue-9)',
    '--mantine-color-frost-outline': 'var(--mantine-color-frost-9)',
    // No straw shade clears 4.5:1 on white (straw-9 #ae8900 is 3.3:1); ember-9 reads as dark amber.
    '--mantine-color-straw-text': 'var(--mantine-color-ember-9)',
    '--mantine-color-yellow-text': 'var(--mantine-color-ember-9)',
    '--mantine-color-straw-outline': 'var(--mantine-color-ember-9)',
    '--mantine-color-yellow-outline': 'var(--mantine-color-ember-9)',
    '--mantine-color-straw-light-color': 'var(--mantine-color-ember-9)',
    '--mantine-color-yellow-light-color': 'var(--mantine-color-ember-9)',
    '--mantine-color-error': 'var(--mantine-color-red-7)',
  },
  dark: {
    '--mantine-color-body': '#0A0A0B',
    '--mantine-color-text': '#FAFAFA',
    '--vh-bg': '#0A0A0B',
    '--vh-surface': '#111113',
    '--vh-surface-2': '#18181B',
    '--vh-sidebar': '#111113',
    '--vh-border': '#27272A',
    '--vh-border-strong': '#3F3F46',
    '--vh-text': '#FAFAFA',
    '--vh-text-soft': '#A1A1AA',
    '--vh-text-faint': '#8A8A94',
    '--vh-ember': t.colors.ember[5],
    '--vh-ember-text': t.colors.ember[5],
    '--vh-accent-soft': 'rgba(245, 162, 15, 0.14)',
    '--vh-parchment': '#B9AD8C',
    '--vh-parchment-2': '#CFC3A3',
    '--vh-ink': '#1A120A',
    // Mantine's dark "light" variant for gray darkens zinc-9 to near-black,
    // invisible on #111113 cards; give gray light/subtle a visible tint.
    '--mantine-color-gray-light': '#1F1F23',
    '--mantine-color-gray-light-hover': '#27272A',
    '--mantine-color-gray-outline': 'var(--mantine-color-gray-4)',
    // Mantine's dark error (red-8 #ac2323) is 2.7:1 on #111113; red-4 is 5.2:1.
    '--mantine-color-error': 'var(--mantine-color-red-4)',
  },
})

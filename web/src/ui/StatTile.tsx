import type { CSSProperties, ReactNode } from 'react'
import { Group, Paper, Text } from '@mantine/core'
import { Sparkline } from './Sparkline'
import classes from './ui.module.css'

/**
 * KPI tile: eyebrow label, big value, optional hint line, icon and trend.
 * `accent` is any CSS colour (use the --vh-* tokens) drawn as a left bar.
 * `compact` shrinks vertical padding and the value's font size (inline
 * style only, so it composes with any background/color the caller applies).
 * `spark`/`sparkFormat` draw a history sparkline (F-1.3) beneath the value;
 * omit `spark` for tiles with no history to show.
 */
export function StatTile({
  label,
  value,
  hint,
  icon,
  accent,
  compact = false,
  spark,
  sparkFormat,
}: {
  label: ReactNode
  value: ReactNode
  hint?: ReactNode
  icon?: ReactNode
  accent?: string
  compact?: boolean
  spark?: number[]
  sparkFormat?: (v: number) => string
}) {
  const rootStyle = {
    ...(accent ? { '--tile-accent': accent } : {}),
    ...(compact ? { padding: '8px 12px', minHeight: 'unset', gap: 4 } : {}),
  } as CSSProperties
  return (
    <Paper
      className={classes.statTile}
      data-accent={accent ? '' : undefined}
      style={Object.keys(rootStyle).length > 0 ? rootStyle : undefined}
    >
      <Group justify="space-between" wrap="nowrap" align="flex-start">
        <span className="vh-eyebrow">{label}</span>
        {icon && (
          <Text c="dimmed" component="span" style={{ display: 'inline-flex' }}>
            {icon}
          </Text>
        )}
      </Group>
      <div>
        <div
          className={classes.statValue}
          data-long={typeof value === 'string' && value.length > 12 ? '' : undefined}
          style={compact ? { fontSize: '1.1rem' } : undefined}
        >
          {value}
        </div>
        {hint && (
          <Text size="xs" c="dimmed" mt={2}>
            {hint}
          </Text>
        )}
        {spark && (
          <div style={{ marginTop: 6 }}>
            <Sparkline
              values={spark}
              format={sparkFormat}
              width="100%"
              height={compact ? 20 : 24}
              label={typeof label === 'string' ? label : undefined}
            />
          </div>
        )}
      </div>
    </Paper>
  )
}

// One labelled row inside the Overview "Status" card: a fixed-width label
// column (with an optional hint underneath) and a flexible body column for
// pills, buttons and text. Stacks to a single column on narrow screens (see
// .row in OverviewTab.module.css).
import type { ReactNode } from 'react'
import { Text } from '@mantine/core'
import classes from './OverviewTab.module.css'

export function StatusRow({ label, hint, children }: { label: ReactNode; hint?: ReactNode; children: ReactNode }) {
  return (
    <div className={classes.row}>
      <div className={classes.rowLabel}>
        <Text size="sm" fw={600}>
          {label}
        </Text>
        {hint && (
          <Text size="xs" c="dimmed">
            {hint}
          </Text>
        )}
      </div>
      <div className={classes.rowBody}>{children}</div>
    </div>
  )
}

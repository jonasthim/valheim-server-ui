import type { ReactNode } from 'react'
import { Group, Text, Title } from '@mantine/core'
import { BrandMark } from '../../ui'
import classes from './Auth.module.css'

/**
 * Centred shell for /login and /setup: a 360px column with the brand
 * header on the neutral page background.
 */
export function AuthLayout({ intro, children }: { intro?: string; children: ReactNode }) {
  return (
    <div className={classes.page}>
      <main className={classes.column}>
        <Group gap="sm" wrap="nowrap">
          <BrandMark size={28} />
          <Title order={1} className={classes.wordmark}>
            Valheim Server UI
          </Title>
        </Group>
        {intro && (
          <Text size="sm" c="dimmed">
            {intro}
          </Text>
        )}
        {children}
      </main>
    </div>
  )
}

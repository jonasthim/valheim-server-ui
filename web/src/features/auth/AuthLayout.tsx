import type { ReactNode } from 'react'
import { Group, Stack, Text } from '@mantine/core'
import { IconCloudUpload, IconDeviceGamepad2, IconPuzzle } from '@tabler/icons-react'
import { BrandMark } from '../../ui'
import classes from './Auth.module.css'

const FEATURES: { icon: typeof IconDeviceGamepad2; text: string }[] = [
  { icon: IconDeviceGamepad2, text: 'Spin up instances and watch players join in real time' },
  { icon: IconCloudUpload, text: 'Automatic backups and restart schedules' },
  { icon: IconPuzzle, text: 'One-click mods from Thunderstore' },
]

/**
 * Two-column shell for /login and /setup: a dark ember-to-iron hero panel
 * with the brand and a short pitch on md+ screens, the form on the right
 * (and the only thing shown on mobile).
 */
export function AuthLayout({ children }: { children: ReactNode }) {
  return (
    <div className={classes.page}>
      <div className={classes.hero} aria-hidden>
        <div className={classes.heroInner}>
          <Group gap="sm">
            <BrandMark size={34} />
            <Text fw={700} size="lg" c="white">
              Valheim Server UI
            </Text>
          </Group>

          <Text size="xl" fw={650} c="white" mt="xl" maw={340} className={classes.tagline}>
            Run your Valheim servers like a pro.
          </Text>

          <Stack gap="md" mt="xl" maw={340}>
            {FEATURES.map(({ icon: Icon, text }) => (
              <Group key={text} gap="sm" wrap="nowrap" align="flex-start">
                <div className={classes.featureIcon}>
                  <Icon size={16} />
                </div>
                <Text size="sm" c="white" opacity={0.85}>
                  {text}
                </Text>
              </Group>
            ))}
          </Stack>
        </div>
      </div>

      <div className={classes.formSide}>{children}</div>
    </div>
  )
}

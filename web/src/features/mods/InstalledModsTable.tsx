// Table of mods installed on an instance: enable toggle, update/uninstall
// actions, and a dependency count popover. Disabled with a call-to-action
// when BepInEx itself is not installed yet.
import { useState } from 'react'
import { safeHref } from '../../lib/format'
import {
  ActionIcon,
  Alert,
  Anchor,
  Avatar,
  Badge,
  Button,
  Group,
  List,
  Loader,
  Popover,
  Skeleton,
  Switch,
  Table,
  Text,
  Tooltip,
} from '@mantine/core'
import { modals } from '@mantine/modals'
import { IconExternalLink, IconPackage, IconRefresh, IconTrash } from '@tabler/icons-react'
import { useAuth } from '../../auth/useAuth'
import { useJobDrawer } from '../jobs'
import type { Mod } from '../../api/types'
import { EmptyState, SectionCard } from '../../ui'
import { useModsOverview, useSetModEnabled, useUninstallMod, useUpdateMod } from './useMods'

export function InstalledModsTable({ id }: { id: string }) {
  const { hasRole } = useAuth()
  const overview = useModsOverview(id)
  const { openJob } = useJobDrawer()
  const setEnabled = useSetModEnabled(id)
  const updateMod = useUpdateMod(id)
  const uninstallMod = useUninstallMod(id)
  const [updatingAll, setUpdatingAll] = useState(false)

  const canOperate = hasRole('operator')
  const bepinexInstalled = !!overview.data?.bepinex.installed
  const mods = overview.data?.mods ?? []
  const updatable = mods.filter((m) => m.update_available)

  function confirmUninstall(mod: Mod) {
    modals.openConfirmModal({
      title: 'Uninstall mod',
      children: (
        <Text size="sm">
          Uninstall <strong>{mod.name}</strong> ({mod.owner})? Its files will be removed.
        </Text>
      ),
      labels: { confirm: 'Uninstall', cancel: 'Cancel' },
      confirmProps: { color: 'red' },
      onConfirm: () =>
        uninstallMod.mutate(mod.id, {
          onSuccess: (res) => openJob(res.job.id),
        }),
    })
  }

  function requestUpdate(mod: Mod) {
    updateMod.mutate(
      { modId: mod.id },
      {
        onSuccess: (res) => openJob(res.job.id),
      },
    )
  }

  async function updateAll() {
    setUpdatingAll(true)
    try {
      for (const mod of updatable) {
        // Sequential on purpose: the backend resolves/plans dependencies per
        // mod and we don't want overlapping mod_update jobs racing each other.
        await updateMod.mutateAsync({ modId: mod.id })
      }
    } finally {
      setUpdatingAll(false)
    }
  }

  if (overview.isLoading) {
    return (
      <SectionCard title="Installed mods">
        <Skeleton height={140} />
      </SectionCard>
    )
  }

  if (!bepinexInstalled) {
    return (
      <Alert color="gray" icon={<IconPackage size={16} />} title="BepInEx required">
        Install BepInEx above before installing or managing mods.
      </Alert>
    )
  }

  return (
    <SectionCard
      title="Installed mods"
      actions={
        canOperate &&
        updatable.length > 0 && (
          <Button
            size="xs"
            variant="light"
            leftSection={<IconRefresh size={14} />}
            loading={updatingAll}
            onClick={updateAll}
          >
            Update all ({updatable.length})
          </Button>
        )
      }
      flush
    >
      {mods.length === 0 ? (
        <div style={{ padding: 'var(--mantine-spacing-lg)' }}>
          <EmptyState
            icon={<IconPackage size={22} />}
            title="No mods installed yet. Browse Thunderstore or upload a mod below."
          />
        </div>
      ) : (
        <Table.ScrollContainer minWidth={720}>
          <Table verticalSpacing="xs">
            <Table.Thead>
              <Table.Tr>
                <Table.Th>Mod</Table.Th>
                <Table.Th>Version</Table.Th>
                <Table.Th>Source</Table.Th>
                <Table.Th>Dependencies</Table.Th>
                <Table.Th>Enabled</Table.Th>
                <Table.Th />
              </Table.Tr>
            </Table.Thead>
            <Table.Tbody>
              {mods.map((mod) => (
                <Table.Tr key={mod.id}>
                  <Table.Td>
                    <Group gap="xs" wrap="nowrap">
                      <Avatar src={mod.icon_url || undefined} size="sm" radius="sm">
                        <IconPackage size={14} />
                      </Avatar>
                      <div>
                        <Group gap={4}>
                          <Text size="sm" fw={500}>
                            {mod.name}
                          </Text>
                          {mod.website_url && (
                            <Anchor href={safeHref(mod.website_url)} target="_blank" rel="noreferrer" size="xs">
                              <IconExternalLink size={12} />
                            </Anchor>
                          )}
                        </Group>
                        <Text size="xs" c="dimmed">
                          {mod.owner}
                        </Text>
                      </div>
                    </Group>
                  </Table.Td>
                  <Table.Td>
                    <Group gap={4} wrap="nowrap">
                      <Text size="sm">{mod.version}</Text>
                      {mod.update_available && (
                        <Badge color="frost" variant="light" size="sm">
                          {mod.latest_version} available
                        </Badge>
                      )}
                    </Group>
                  </Table.Td>
                  <Table.Td>
                    <Badge variant="outline" size="sm">
                      {mod.source}
                    </Badge>
                  </Table.Td>
                  <Table.Td>
                    {mod.dependencies && mod.dependencies.length > 0 ? (
                      <Popover width={260} withArrow shadow="md">
                        <Popover.Target>
                          <Badge variant="light" style={{ cursor: 'pointer' }}>
                            {mod.dependencies.length}
                          </Badge>
                        </Popover.Target>
                        <Popover.Dropdown>
                          <List size="xs" spacing={2}>
                            {(mod.dependencies ?? []).map((dep) => (
                              <List.Item key={dep}>{dep}</List.Item>
                            ))}
                          </List>
                        </Popover.Dropdown>
                      </Popover>
                    ) : (
                      <Text size="sm" c="dimmed">
                        -
                      </Text>
                    )}
                  </Table.Td>
                  <Table.Td>
                    <Switch
                      checked={mod.enabled}
                      disabled={!canOperate || setEnabled.isPending}
                      onChange={(e) => setEnabled.mutate({ modId: mod.id, enabled: e.currentTarget.checked })}
                      aria-label={`Enable ${mod.name}`}
                    />
                  </Table.Td>
                  <Table.Td>
                    {canOperate && (
                      <Group gap={4} wrap="nowrap" justify="flex-end">
                        {mod.source === 'thunderstore' && (
                          <Tooltip label={mod.update_available ? 'Update to latest' : 'Already up to date'}>
                            <ActionIcon
                              variant="subtle"
                              aria-label={`Update ${mod.name}`}
                              disabled={!mod.update_available || updateMod.isPending}
                              onClick={() => requestUpdate(mod)}
                            >
                              {updateMod.isPending ? <Loader size={14} /> : <IconRefresh size={16} />}
                            </ActionIcon>
                          </Tooltip>
                        )}
                        <Tooltip label="Uninstall">
                          <ActionIcon
                            variant="subtle"
                            color="red"
                            aria-label={`Uninstall ${mod.name}`}
                            onClick={() => confirmUninstall(mod)}
                          >
                            <IconTrash size={16} />
                          </ActionIcon>
                        </Tooltip>
                      </Group>
                    )}
                  </Table.Td>
                </Table.Tr>
              ))}
            </Table.Tbody>
          </Table>
        </Table.ScrollContainer>
      )}
    </SectionCard>
  )
}

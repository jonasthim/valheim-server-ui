// Gameplay card on the Mods tab: install state for the Valheim UI Gameplay
// plugin (bundled with the agent, see ModLoaderCard), then Autofeed and
// Raids forms backed by its BepInEx config file. Settings apply live, no
// restart. See /home/thim/.claude/plans/gameplay-plugin-2026-10.md TASK U-1.
import { useEffect } from 'react'
import { useForm } from '@mantine/form'
import { Alert, Button, Group, NumberInput, SimpleGrid, Skeleton, Stack, Switch, Text, TextInput } from '@mantine/core'
import { useAuth } from '../../auth/useAuth'
import { openConfirmInstallAgent } from '../agent/openConfirmInstallAgent'
import { useAgent, useAgentCatalog, useInstallAgent } from '../agent/useAgent'
import { useInstance } from '../instances/useInstance'
import { notifyError, notifySuccess } from '../../lib/notify'
import { useUnsavedChanges } from '../../lib/useUnsavedChanges'
import { useModConfig, useSaveModConfig } from '../mods/useModConfig'
import { useModsOverview } from '../mods/useMods'
import { DataTable, SectionCard, StatusPill, StickySaveBar } from '../../ui'
import type { DataTableColumn } from '../../ui'
import { GAMEPLAY_CFG, GAMEPLAY_MOD, fromEntries, mergeRaidNames, toUpdates, type GameplayValues } from './gameplayConfig'
import { VANILLA_RAIDS } from './raids'

const EMPTY_VALUES: GameplayValues = fromEntries([])

export function GameplayCard({ id }: { id: string }) {
  const { hasRole } = useAuth()
  const canEdit = hasRole('operator')
  const overview = useModsOverview(id)
  const instance = useInstance(id)
  const agent = useAgent(id)
  const installAgent = useInstallAgent(id)

  const row = overview.data?.mods.find((m) => m.owner === GAMEPLAY_MOD.owner && m.name === GAMEPLAY_MOD.name)
  const installed = !!row
  const catalog = useAgentCatalog(id, !!agent.data?.connected)
  const cfg = useModConfig(id, installed ? GAMEPLAY_CFG : undefined)
  const saveCfg = useSaveModConfig(id, GAMEPLAY_CFG, { notify: false })

  const form = useForm<GameplayValues>({ initialValues: EMPTY_VALUES })
  const unsaved = useUnsavedChanges(form, { enabled: canEdit })

  useEffect(() => {
    if (!cfg.data) return
    if (!form.initialized) {
      form.initialize(fromEntries(cfg.data.entries))
      return
    }
    // A later re-fetch should never clobber an in-progress edit.
    if (!form.isDirty()) {
      const next = fromEntries(cfg.data.entries)
      form.setValues(next)
      form.resetDirty(next)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [cfg.data])

  if (overview.isLoading || agent.isLoading) {
    return (
      <SectionCard title="Gameplay">
        <Skeleton height={88} />
      </SectionCard>
    )
  }

  const isRunning = ['running', 'starting', 'stopping'].includes(instance.data?.status.state ?? '')
  const bepinexInstalled = !!overview.data?.bepinex.installed
  const updateAvailable = !!row && !!agent.data?.bundled_version && row.version !== agent.data.bundled_version

  let pill: { color: string; label: string }
  if (!row) pill = { color: 'gray', label: 'not installed' }
  else if (updateAvailable) pill = { color: 'frost', label: 'update available' }
  else pill = { color: 'moss', label: `installed v${row.version}` }

  const catalogNames = catalog.data?.events.map((e) => e.name) ?? []
  const usingFallback = catalogNames.length === 0
  const disabledRaids = form.values.disabledRaids ?? []
  const raidNames = mergeRaidNames(usingFallback ? VANILLA_RAIDS.map((r) => r.name) : catalogNames, disabledRaids)

  function toggleRaid(name: string, allowed: boolean) {
    const next = new Set(disabledRaids)
    if (allowed) next.delete(name)
    else next.add(name)
    form.setFieldValue('disabledRaids', [...next])
  }

  function save(values: GameplayValues) {
    saveCfg.mutate(toUpdates(values), {
      onSuccess: () => {
        unsaved.markClean()
        notifySuccess('Gameplay settings saved, applied live')
      },
      onError: (err) => notifyError(err, 'Could not save gameplay settings'),
    })
  }

  const raidColumns: DataTableColumn<string>[] = [
    {
      key: 'raid',
      header: 'Raid',
      render: (name) => {
        const known = VANILLA_RAIDS.find((r) => r.name === name)
        return (
          <Stack gap={0}>
            <Text size="sm" ff="monospace">
              {name}
            </Text>
            {known && (
              <Text size="xs" c="dimmed">
                {known.label}
              </Text>
            )}
          </Stack>
        )
      },
    },
    {
      key: 'allowed',
      header: 'Allowed',
      render: (name) => (
        <Switch
          aria-label={`Allow ${name}`}
          checked={!disabledRaids.includes(name)}
          onChange={(e) => toggleRaid(name, e.currentTarget.checked)}
          disabled={!canEdit || !form.values.raidsEnabled}
        />
      ),
    },
  ]

  return (
    <SectionCard
      title="Gameplay"
      description="Optional server-side helpers from the Valheim UI Gameplay plugin. Settings apply live, no restart."
      actions={
        <Group gap="xs" wrap="nowrap">
          <StatusPill color={pill.color}>{pill.label}</StatusPill>
          {canEdit && bepinexInstalled && (!installed || updateAvailable) && (
            <Button
              size="xs"
              variant="default"
              loading={installAgent.isPending}
              onClick={() =>
                openConfirmInstallAgent({
                  update: installed,
                  isRunning,
                  onConfirm: (stop) => installAgent.mutate({ stop_if_running: stop }),
                })
              }
            >
              {installed ? 'Update plugin' : 'Install plugin'}
            </Button>
          )}
        </Group>
      }
    >
      {!installed ? (
        <Alert color="frost">
          Install the Gameplay plugin to use autofeed and raid control. It is installed together with the Valheim UI
          Agent.
        </Alert>
      ) : cfg.isLoading ? (
        <Skeleton height={220} />
      ) : cfg.isError ? (
        <Alert color="frost">Start the server once to create the settings file.</Alert>
      ) : (
        <>
          <form id="gameplay-form" onSubmit={form.onSubmit(save)}>
            <Stack gap="lg">
              {!canEdit && (
                <Text size="xs" c="dimmed">
                  You have read-only access.
                </Text>
              )}
              <Stack gap="sm">
                <Switch
                  label="Autofeed"
                  description="Feed hungry tamed animals from chests marked with a sign"
                  disabled={!canEdit}
                  {...form.getInputProps('autofeedEnabled', { type: 'checkbox' })}
                />
                <TextInput
                  label="Sign prefix"
                  description="A chest is a feed chest when a sign within the sign radius starts with this text (case-insensitive). Empty marks every chest in range."
                  disabled={!canEdit}
                  {...form.getInputProps('signPrefix')}
                />
                <SimpleGrid cols={{ base: 1, sm: 3 }}>
                  <NumberInput
                    label="Sign radius (m)"
                    min={0.5}
                    max={6}
                    step={0.5}
                    disabled={!canEdit}
                    {...form.getInputProps('signRadius')}
                  />
                  <NumberInput label="Range (m)" min={1} max={100} disabled={!canEdit} {...form.getInputProps('range')} />
                  <NumberInput
                    label="Interval (s)"
                    min={5}
                    max={600}
                    disabled={!canEdit}
                    {...form.getInputProps('intervalSeconds')}
                  />
                </SimpleGrid>
              </Stack>

              <Stack gap="sm">
                <Switch
                  label="Raid control"
                  description="Keep the raids switched off below from starting on their own. The Start event action still works."
                  disabled={!canEdit}
                  {...form.getInputProps('raidsEnabled', { type: 'checkbox' })}
                />
                <DataTable
                  aria-label="Raids"
                  columns={raidColumns}
                  rows={raidNames}
                  rowKey={(name) => name}
                  empty="No raids reported yet. Start the server to read the world's raid list; the vanilla list is shown meanwhile."
                />
                {usingFallback && (
                  <Text size="xs" c="dimmed">
                    Showing the vanilla raid list; connect the agent for this world's raids.
                  </Text>
                )}
              </Stack>
            </Stack>
          </form>

          <StickySaveBar
            formId="gameplay-form"
            show={unsaved.dirty}
            dirty={unsaved.dirty}
            saveLabel="Save gameplay settings"
            saving={saveCfg.isPending}
            onDiscard={unsaved.discard}
          />
        </>
      )}
    </SectionCard>
  )
}

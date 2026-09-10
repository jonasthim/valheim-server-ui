// BepInEx plugin config (.cfg) editor: a typed "Form" view (grouped by
// section, one input per entry) and a "Raw" textarea fallback.
// See docs/ARCHITECTURE.md §12 for the on-disk format this mirrors.
import { useMemo, useState, type ReactNode } from 'react'
import {
  Accordion,
  ActionIcon,
  Alert,
  Badge,
  Box,
  Button,
  Code,
  Grid,
  Group,
  NavLink,
  NumberInput,
  ScrollArea,
  SegmentedControl,
  Select,
  Skeleton,
  Stack,
  Switch,
  Text,
  Textarea,
  TextInput,
  Tooltip,
} from '@mantine/core'
import { IconAlertTriangle, IconDeviceFloppy, IconFileText, IconRestore } from '@tabler/icons-react'
import { useAuth } from '../../auth/useAuth'
import { fmtAgo, fmtBytes } from '../../lib/format'
import type { ConfigEntry } from '../../api/types'
import { EmptyState, SectionCard } from '../../ui'
import { entryKey, isBooleanEntry, isClientSideSetting, isKeybindEntry, isNumericEntry } from './helpers'
import { useConfigFiles, useModConfig, useSaveModConfig } from './useModConfig'
import { useModsOverview } from './useMods'

type Mode = 'form' | 'raw'

const EMPTY_ENTRIES: ConfigEntry[] = []

export function ConfigEditor({ id }: { id: string }) {
  const { hasRole } = useAuth()
  const readOnly = !hasRole('operator')
  const overview = useModsOverview(id)
  const filesQuery = useConfigFiles(id)
  const [explicitSelection, setExplicitSelection] = useState<string | undefined>()
  // Default to the first file until the user picks one explicitly, without an
  // effect: derive it during render instead of syncing state to state.
  const selected = explicitSelection ?? filesQuery.data?.[0]?.name

  if (!overview.data?.bepinex.installed) return null

  return (
    <SectionCard title="Mod config files">
      {filesQuery.isLoading ? (
        <Skeleton height={160} />
      ) : !filesQuery.data || filesQuery.data.length === 0 ? (
        <EmptyState
          icon={<IconFileText size={22} />}
          title="No config files yet. They appear here once a mod with a BepInEx config is installed and has run at least once."
        />
      ) : (
        <Grid>
          <Grid.Col span={{ base: 12, sm: 3 }}>
            <ScrollArea.Autosize mah={420}>
              <Stack gap={2}>
                {filesQuery.data.map((f) => (
                  <NavLink
                    key={f.name}
                    label={f.name}
                    description={`${fmtBytes(f.size_bytes)} · ${fmtAgo(f.modified_at)}`}
                    active={f.name === selected}
                    onClick={() => setExplicitSelection(f.name)}
                    styles={{ label: { fontSize: 13, wordBreak: 'break-all' } }}
                  />
                ))}
              </Stack>
            </ScrollArea.Autosize>
          </Grid.Col>
          <Grid.Col span={{ base: 12, sm: 9 }}>
            {/* Keyed by fileName so switching files remounts with fresh (non-dirty) local state. */}
            {selected && <ConfigFileEditor key={selected} id={id} fileName={selected} readOnly={readOnly} />}
          </Grid.Col>
        </Grid>
      )}
    </SectionCard>
  )
}

function ConfigFileEditor({ id, fileName, readOnly }: { id: string; fileName: string; readOnly: boolean }) {
  const fileQuery = useModConfig(id, fileName)
  const overview = useModsOverview(id)
  const save = useSaveModConfig(id, fileName)
  const [mode, setMode] = useState<Mode>('form')
  const [pending, setPending] = useState<Record<string, string>>({})
  const [rawDraft, setRawDraft] = useState<string | undefined>(undefined)

  const entries = fileQuery.data?.entries ?? EMPTY_ENTRIES
  // Server-synced keys lead the file; client-side ("[Not Synced with
  // Server]") keys collapse into one group, since on a dedicated server they
  // are not pushed to players and may not take effect.
  const { serverSections, clientSections, clientCount } = useMemo(() => {
    const group = () => ({ order: [] as string[], by: new Map<string, ConfigEntry[]>() })
    const server = group()
    const client = group()
    for (const e of entries) {
      const g = isClientSideSetting(e) ? client : server
      if (!g.by.has(e.section)) {
        g.by.set(e.section, [])
        g.order.push(e.section)
      }
      g.by.get(e.section)!.push(e)
    }
    const shape = (g: ReturnType<typeof group>) =>
      g.order.map((section) => ({ section, entries: g.by.get(section)! }))
    const shapedClient = shape(client)
    return {
      serverSections: shape(server),
      clientSections: shapedClient,
      clientCount: shapedClient.reduce((n, sec) => n + sec.entries.length, 0),
    }
  }, [entries])

  const formDirty = Object.keys(pending).length > 0
  const rawDirty = rawDraft !== undefined && rawDraft !== fileQuery.data?.raw
  const dirty = mode === 'form' ? formDirty : rawDirty

  function setEntryValue(entry: ConfigEntry, value: string) {
    setPending((prev) => ({ ...prev, [entryKey(entry.section, entry.key)]: value }))
  }

  function resetEntry(entry: ConfigEntry) {
    setPending((prev) => {
      const next = { ...prev }
      delete next[entryKey(entry.section, entry.key)]
      return next
    })
  }

  const renderEntry = (entry: ConfigEntry) => (
    <ConfigEntryField
      key={entryKey(entry.section, entry.key)}
      entry={entry}
      value={pending[entryKey(entry.section, entry.key)]}
      onChange={(v) => setEntryValue(entry, v)}
      onReset={() => resetEntry(entry)}
      readOnly={readOnly}
    />
  )

  function handleSave() {
    if (mode === 'raw') {
      save.mutate({ raw: rawDraft ?? fileQuery.data?.raw ?? '' }, { onSuccess: () => setRawDraft(undefined) })
      return
    }
    const values = Object.entries(pending)
      .map(([key, value]) => {
        const [section, ...rest] = key.split('::')
        return { section, key: rest.join('::'), value }
      })
      .filter((v) => {
        const original = entries.find((e) => e.section === v.section && e.key === v.key)
        return original ? original.value !== v.value : true
      })
    if (values.length === 0) return
    save.mutate({ values }, { onSuccess: () => setPending({}) })
  }

  if (fileQuery.isLoading || !fileQuery.data) {
    return <Skeleton height={200} />
  }

  return (
    <Stack gap="sm">
      <Group justify="space-between" wrap="wrap">
        <SegmentedControl
          size="xs"
          value={mode}
          onChange={(v) => setMode(v as Mode)}
          data={[
            { label: 'Form', value: 'form' },
            { label: 'Raw', value: 'raw' },
          ]}
        />
        <Group gap="xs">
          {dirty && (
            <Badge color="straw" variant="light">
              Unsaved changes
            </Badge>
          )}
          {!readOnly && (
            <Button
              size="xs"
              leftSection={<IconDeviceFloppy size={14} />}
              disabled={!dirty}
              loading={save.isPending}
              onClick={handleSave}
            >
              Save
            </Button>
          )}
        </Group>
      </Group>

      {overview.data?.pending_restart && (
        <Alert color="straw" icon={<IconAlertTriangle size={16} />} title="Restart required">
          Restart the instance from the Overview tab to apply the latest config changes.
        </Alert>
      )}

      {mode === 'raw' ? (
        <Textarea
          value={rawDraft ?? fileQuery.data.raw}
          onChange={(e) => setRawDraft(e.currentTarget.value)}
          autosize
          minRows={10}
          maxRows={30}
          disabled={readOnly}
          styles={{ input: { fontFamily: 'var(--mantine-font-family-monospace)', fontSize: 12 } }}
        />
      ) : serverSections.length === 0 && clientSections.length === 0 ? (
        <Text c="dimmed" size="sm">
          No parsed entries in this file — use Raw mode to edit it directly.
        </Text>
      ) : (
        <Accordion
          multiple
          defaultValue={
            serverSections.length > 0 ? serverSections.map((s) => s.section) : ['__client-side__']
          }
        >
          {serverSections.map(({ section, entries: sectionEntries }) => (
            <Accordion.Item key={section} value={section}>
              <Accordion.Control>
                <Text size="sm" fw={500}>
                  {section || '(root)'}
                </Text>
              </Accordion.Control>
              <Accordion.Panel>
                <Stack gap="sm">{sectionEntries.map(renderEntry)}</Stack>
              </Accordion.Panel>
            </Accordion.Item>
          ))}

          {clientSections.length > 0 && (
            <Accordion.Item value="__client-side__">
              <Accordion.Control>
                <Group gap="xs" wrap="nowrap">
                  <IconAlertTriangle
                    size={15}
                    color="var(--mantine-color-straw-6)"
                    style={{ flexShrink: 0 }}
                  />
                  <Text size="sm" fw={500}>
                    Client-side settings
                  </Text>
                  <Badge size="sm" variant="light" color="gray">
                    {clientCount}
                  </Badge>
                </Group>
              </Accordion.Control>
              <Accordion.Panel>
                <Stack gap="md">
                  <Alert color="straw" variant="light" icon={<IconAlertTriangle size={16} />}>
                    These keys are marked <Code>[Not Synced with Server]</Code>. On a dedicated server they
                    are not pushed to connected players and may have no effect; one-shot actions like presets
                    reset themselves to their default on the next restart. Edit them only if you also play
                    from this machine.
                  </Alert>
                  {clientSections.map(({ section, entries: sectionEntries }) => (
                    <Stack key={section} gap="xs">
                      <Text size="xs" fw={600} c="dimmed" tt="uppercase">
                        {section || '(root)'}
                      </Text>
                      {sectionEntries.map(renderEntry)}
                    </Stack>
                  ))}
                </Stack>
              </Accordion.Panel>
            </Accordion.Item>
          )}
        </Accordion>
      )}
    </Stack>
  )
}

function ConfigEntryField({
  entry,
  value,
  onChange,
  onReset,
  readOnly,
}: {
  entry: ConfigEntry
  value: string | undefined
  onChange: (value: string) => void
  onReset: () => void
  readOnly: boolean
}) {
  const current = value ?? entry.value
  const isDirty = value !== undefined && value !== entry.value
  const canReset = entry.default_value !== undefined && current !== entry.default_value

  const hint = entry.default_value !== undefined ? `Default: ${entry.default_value}` : undefined
  const description = [entry.description, hint].filter(Boolean).join(' — ') || undefined

  let control: ReactNode
  if (isBooleanEntry(entry)) {
    control = (
      <Switch
        label={entry.key}
        description={description}
        checked={current.toLowerCase() === 'true'}
        onChange={(e) => onChange(e.currentTarget.checked ? 'true' : 'false')}
        disabled={readOnly}
      />
    )
  } else if (!isKeybindEntry(entry) && entry.acceptable_values && entry.acceptable_values.length > 0) {
    // Mantine Select throws on duplicate option values; some mod cfgs list a
    // value more than once. Dedupe, and make sure the current value is always
    // selectable even if the mod dropped it from its acceptable list.
    const options = Array.from(new Set([...entry.acceptable_values, current].filter((v) => v !== '')))
    control = (
      <Select
        label={entry.key}
        description={description}
        data={options}
        value={current}
        onChange={(v) => onChange(v ?? entry.value)}
        disabled={readOnly}
        allowDeselect={false}
      />
    )
  } else if (isNumericEntry(entry)) {
    const min = entry.range?.min !== undefined && entry.range.min !== '' ? Number(entry.range.min) : undefined
    const max = entry.range?.max !== undefined && entry.range.max !== '' ? Number(entry.range.max) : undefined
    const num = Number(current)
    control = (
      <NumberInput
        label={entry.key}
        description={description}
        value={current === '' || Number.isNaN(num) ? '' : num}
        min={min}
        max={max}
        onChange={(v) => onChange(String(v))}
        disabled={readOnly}
      />
    )
  } else {
    control = (
      <TextInput
        label={entry.key}
        description={description}
        value={current}
        onChange={(e) => onChange(e.currentTarget.value)}
        disabled={readOnly}
      />
    )
  }

  return (
    <Group align="flex-end" wrap="nowrap" gap="xs">
      <Box style={{ flex: 1, minWidth: 0 }}>{control}</Box>
      {!readOnly && entry.default_value !== undefined && (
        <Tooltip label={`Reset to default (${entry.default_value})`}>
          <ActionIcon
            variant="subtle"
            aria-label={`Reset ${entry.key} to default`}
            disabled={!canReset && !isDirty}
            onClick={onReset}
          >
            <IconRestore size={16} />
          </ActionIcon>
        </Tooltip>
      )}
    </Group>
  )
}

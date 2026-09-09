import { useMemo, useState } from 'react'
import { Anchor, Button, Code, Group, Popover, Skeleton, Stack, Table, Text, TextInput } from '@mantine/core'
import { useDebouncedValue } from '@mantine/hooks'
import { useInfiniteQuery } from '@tanstack/react-query'
import { IconHistory, IconSearch } from '@tabler/icons-react'
import { api } from '../../api/client'
import type { AuditEntry } from '../../api/types'
import { fmtTime } from '../../lib/format'
import { EmptyState, PageHeader, SectionCard } from '../../ui'

interface AuditResponse {
  entries: AuditEntry[]
}

interface AuditChange {
  path: string
  from?: unknown
  to?: unknown
}

function isChange(x: unknown): x is AuditChange {
  return (
    !!x &&
    typeof x === 'object' &&
    typeof (x as Record<string, unknown>).path === 'string'
  )
}

/** `details.changes`, if it is a non-empty array of field-level diffs. */
function getChanges(details: AuditEntry['details']): AuditChange[] | null {
  const raw = details?.changes
  if (!Array.isArray(raw)) return null
  const changes = raw.filter(isChange)
  return changes.length > 0 ? changes : null
}

/** Whether `details` has any keys besides `changes` worth showing raw. */
function hasOtherDetails(details: AuditEntry['details']): boolean {
  if (!details) return false
  return Object.keys(details).some((k) => k !== 'changes')
}

function formatValue(v: unknown): string {
  if (v === undefined) return 'unset'
  if (v === null) return 'null'
  if (typeof v === 'string') return v
  // Lists of scalars (set keys, admin ids) read better as "fire, nomap".
  if (Array.isArray(v) && v.every((x) => typeof x !== 'object' || x === null)) {
    return v.length === 0 ? 'none' : v.map((x) => (typeof x === 'string' ? x : JSON.stringify(x))).join(', ')
  }
  return JSON.stringify(v)
}

const VISIBLE_CHANGES = 4

function ChangeLine({ change }: { change: AuditChange }) {
  return (
    <Group gap={6} wrap="wrap" align="baseline" style={{ rowGap: 0 }}>
      <Text size="xs" ff="monospace" c="dimmed" style={{ whiteSpace: 'nowrap' }}>
        {change.path}:
      </Text>
      <Text size="xs" ff="monospace" c="dimmed" td="line-through" style={{ overflowWrap: 'anywhere' }}>
        {formatValue(change.from)}
      </Text>
      <Text size="xs" ff="monospace" c="dimmed">
        →
      </Text>
      <Text size="xs" ff="monospace" c="green" style={{ overflowWrap: 'anywhere' }}>
        {formatValue(change.to)}
      </Text>
    </Group>
  )
}

function ChangesCell({ entry }: { entry: AuditEntry }) {
  const changes = getChanges(entry.details)
  const [expanded, setExpanded] = useState(false)

  if (changes) {
    const visible = expanded ? changes : changes.slice(0, VISIBLE_CHANGES)
    const hiddenCount = changes.length - visible.length
    return (
      <Stack gap={2}>
        {visible.map((c, i) => (
          <ChangeLine key={`${c.path}-${i}`} change={c} />
        ))}
        {hiddenCount > 0 && (
          <Anchor component="button" type="button" size="xs" onClick={() => setExpanded(true)}>
            +{hiddenCount} more
          </Anchor>
        )}
        {expanded && changes.length > VISIBLE_CHANGES && (
          <Anchor component="button" type="button" size="xs" onClick={() => setExpanded(false)}>
            Show less
          </Anchor>
        )}
      </Stack>
    )
  }

  if (hasOtherDetails(entry.details)) {
    return (
      <Popover width={360} position="left" withArrow shadow="md">
        <Popover.Target>
          <Anchor component="button" type="button" size="xs">
            Details
          </Anchor>
        </Popover.Target>
        <Popover.Dropdown>
          <Code block>{JSON.stringify(entry.details, null, 2)}</Code>
        </Popover.Dropdown>
      </Popover>
    )
  }

  return (
    <Text size="sm" c="dimmed">
      -
    </Text>
  )
}

const PAGE_SIZE = 100

export function AuditPage() {
  const [instance, setInstance] = useState('')
  const [username, setUsername] = useState('')
  const [debouncedInstance] = useDebouncedValue(instance, 300)
  const [debouncedUsername] = useDebouncedValue(username, 300)

  const filters = { instance: debouncedInstance.trim(), user: debouncedUsername.trim() }

  const query = useInfiniteQuery({
    queryKey: ['audit', filters],
    queryFn: ({ pageParam }) =>
      api.get<AuditResponse>('/audit', {
        instance: filters.instance || undefined,
        user: filters.user || undefined,
        limit: PAGE_SIZE,
        before: pageParam,
      }),
    initialPageParam: undefined as number | undefined,
    getNextPageParam: (lastPage) =>
      lastPage.entries.length === PAGE_SIZE ? lastPage.entries[lastPage.entries.length - 1]?.id : undefined,
  })

  const entries = useMemo(() => query.data?.pages.flatMap((p) => p.entries) ?? [], [query.data])

  return (
    <Stack gap="lg">
      <PageHeader eyebrow="Administration" title="Audit log" description="Every action taken through this UI, newest first." />

      <Group gap="sm" wrap="wrap">
        <TextInput
          placeholder="Filter by instance id"
          leftSection={<IconSearch size={14} />}
          value={instance}
          onChange={(e) => setInstance(e.currentTarget.value)}
          w={220}
          aria-label="Filter by instance id"
        />
        <TextInput
          placeholder="Filter by username"
          leftSection={<IconSearch size={14} />}
          value={username}
          onChange={(e) => setUsername(e.currentTarget.value)}
          w={220}
          aria-label="Filter by username"
        />
      </Group>

      <SectionCard flush>
        <Table.ScrollContainer minWidth={900}>
          <Table verticalSpacing="xs">
            <Table.Thead>
              <Table.Tr>
                <Table.Th>Time</Table.Th>
                <Table.Th>User</Table.Th>
                <Table.Th>Action</Table.Th>
                <Table.Th>Instance</Table.Th>
                <Table.Th>Target</Table.Th>
                <Table.Th>Changes</Table.Th>
              </Table.Tr>
            </Table.Thead>
            <Table.Tbody>
              {query.isLoading &&
                Array.from({ length: 5 }).map((_, i) => (
                  <Table.Tr key={i}>
                    <Table.Td colSpan={6}>
                      <Skeleton height={18} />
                    </Table.Td>
                  </Table.Tr>
                ))}
              {!query.isLoading && entries.length === 0 && (
                <Table.Tr>
                  <Table.Td colSpan={6}>
                    <EmptyState icon={<IconHistory size={22} />} title="No audit entries match these filters." />
                  </Table.Td>
                </Table.Tr>
              )}
              {entries.map((e) => (
                <Table.Tr key={e.id}>
                  <Table.Td>
                    <Text size="sm">{fmtTime(e.ts)}</Text>
                  </Table.Td>
                  <Table.Td>{e.username || '-'}</Table.Td>
                  <Table.Td>
                    <Text size="sm" ff="monospace">
                      {e.action}
                    </Text>
                  </Table.Td>
                  <Table.Td>{e.instance_id || '-'}</Table.Td>
                  <Table.Td>{e.target || '-'}</Table.Td>
                  <Table.Td>
                    <ChangesCell entry={e} />
                  </Table.Td>
                </Table.Tr>
              ))}
            </Table.Tbody>
          </Table>
        </Table.ScrollContainer>
      </SectionCard>

      <Group justify="center">
        {query.hasNextPage && (
          <Button variant="default" onClick={() => query.fetchNextPage()} loading={query.isFetchingNextPage}>
            Load older
          </Button>
        )}
      </Group>
    </Stack>
  )
}

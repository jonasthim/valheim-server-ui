import { useMemo, useState } from 'react'
import { Badge, Button, Code, Group, Popover, Skeleton, Table, Text, TextInput, Title } from '@mantine/core'
import { useDebouncedValue } from '@mantine/hooks'
import { useInfiniteQuery } from '@tanstack/react-query'
import { IconSearch } from '@tabler/icons-react'
import { api } from '../../api/client'
import type { AuditEntry } from '../../api/types'
import { fmtTime } from '../../lib/format'

interface AuditResponse {
  entries: AuditEntry[]
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
    <>
      <Title order={2} mb="md">
        Audit log
      </Title>

      <Group mb="md">
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

      <Table.ScrollContainer minWidth={900}>
        <Table verticalSpacing="xs" highlightOnHover>
          <Table.Thead>
            <Table.Tr>
              <Table.Th>Time</Table.Th>
              <Table.Th>User</Table.Th>
              <Table.Th>Action</Table.Th>
              <Table.Th>Instance</Table.Th>
              <Table.Th>Target</Table.Th>
              <Table.Th>Details</Table.Th>
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
                  <Text c="dimmed" ta="center" py="md">
                    No audit entries match these filters.
                  </Text>
                </Table.Td>
              </Table.Tr>
            )}
            {entries.map((e) => (
              <Table.Tr key={e.id}>
                <Table.Td>{fmtTime(e.ts)}</Table.Td>
                <Table.Td>{e.username || '-'}</Table.Td>
                <Table.Td>
                  <Badge variant="light">{e.action}</Badge>
                </Table.Td>
                <Table.Td>{e.instance_id || '-'}</Table.Td>
                <Table.Td>{e.target || '-'}</Table.Td>
                <Table.Td>
                  {e.details && Object.keys(e.details).length > 0 ? (
                    <Popover width={360} position="left" withArrow shadow="md">
                      <Popover.Target>
                        <Button variant="subtle" size="compact-xs">
                          View
                        </Button>
                      </Popover.Target>
                      <Popover.Dropdown>
                        <Code block>{JSON.stringify(e.details, null, 2)}</Code>
                      </Popover.Dropdown>
                    </Popover>
                  ) : (
                    '-'
                  )}
                </Table.Td>
              </Table.Tr>
            ))}
          </Table.Tbody>
        </Table>
      </Table.ScrollContainer>

      <Group justify="center" mt="md">
        {query.hasNextPage && (
          <Button variant="default" onClick={() => query.fetchNextPage()} loading={query.isFetchingNextPage}>
            Load older
          </Button>
        )}
      </Group>
    </>
  )
}

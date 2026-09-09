import { Badge, Group, Skeleton, Stack, Table, Text } from '@mantine/core'
import type { PlayersResponse } from '../../../api/types'
import { fmtAgo } from '../../../lib/format'
import { SectionCard, StatusPill } from '../../../ui'
import { COUNT_SOURCE_LABELS } from './constants'

export function PlayersOnlinePanel({
  data,
  isLoading,
}: {
  data: PlayersResponse | undefined
  isLoading: boolean
}) {
  const online = data?.online ?? []
  const count = data?.online_count ?? online.length
  const source = data?.count_source ?? 'none'

  return (
    <SectionCard
      title="Online"
      flush
      actions={
        <Group gap="xs">
          <StatusPill color={count > 0 ? 'moss' : 'gray'}>{count} online</StatusPill>
          <Badge size="sm" color="gray" variant="outline">
            {COUNT_SOURCE_LABELS[source]}
          </Badge>
        </Group>
      }
    >
      {isLoading && (
        <div style={{ padding: 'var(--mantine-spacing-lg)' }}>
          <Skeleton height={60} />
        </div>
      )}

      {!isLoading && online.length === 0 && (
        <Text c="dimmed" size="sm" p="lg">
          No players are connected right now.
        </Text>
      )}

      {!isLoading && online.length > 0 && (
        <Table.ScrollContainer minWidth={480}>
          <Table verticalSpacing="xs">
            <Table.Thead>
              <Table.Tr>
                <Table.Th>Name</Table.Th>
                <Table.Th>Platform id</Table.Th>
                <Table.Th>Connected</Table.Th>
              </Table.Tr>
            </Table.Thead>
            <Table.Tbody>
              {online.map((p, i) => (
                <Table.Tr key={p.platform_id ?? `${p.name}-${i}`}>
                  <Table.Td>{p.name}</Table.Td>
                  <Table.Td>
                    <Text ff="monospace" size="sm">
                      {p.platform_id ?? '-'}
                    </Text>
                  </Table.Td>
                  <Table.Td>{fmtAgo(p.connected_at)}</Table.Td>
                </Table.Tr>
              ))}
            </Table.Tbody>
          </Table>
        </Table.ScrollContainer>
      )}

      <Stack gap={2} p="lg" pt={online.length > 0 ? 0 : undefined}>
        <Text size="xs" c="dimmed">
          Player names come from the console log (best effort); the count above comes {COUNT_SOURCE_LABELS[source]}.
        </Text>
      </Stack>
    </SectionCard>
  )
}

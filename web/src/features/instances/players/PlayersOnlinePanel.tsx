import { Badge, Card, Group, Skeleton, Stack, Table, Text, Title } from '@mantine/core'
import type { PlayersResponse } from '../../../api/types'
import { fmtAgo } from '../../../lib/format'
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
    <Card withBorder>
      <Group justify="space-between" mb="sm">
        <Title order={4}>Online</Title>
        <Group gap="xs">
          <Badge size="lg" variant="light">
            {count} online
          </Badge>
          <Badge size="sm" color="gray" variant="outline">
            {COUNT_SOURCE_LABELS[source]}
          </Badge>
        </Group>
      </Group>

      {isLoading && <Skeleton height={60} />}

      {!isLoading && online.length === 0 && (
        <Text c="dimmed" size="sm">
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

      <Stack gap={2} mt="sm">
        <Text size="xs" c="dimmed">
          Player names come from the console log (best effort); the count above comes {COUNT_SOURCE_LABELS[source]}.
        </Text>
      </Stack>
    </Card>
  )
}

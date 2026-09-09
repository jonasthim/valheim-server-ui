import { Card, Stack, Tabs, Title } from '@mantine/core'
import { useAuth } from '../../auth/useAuth'
import { ListEditor } from './players/ListEditor'
import { KnownPlayersTable } from './players/KnownPlayersTable'
import { PlayersOnlinePanel } from './players/PlayersOnlinePanel'
import { LIST_KINDS, LIST_KIND_LABELS } from './players/constants'
import { useAddPlayerToList, usePlayers } from './players/usePlayers'

// Owned by WP-12 (docs/WORKPLAN.md). Props: the instance id.
export function PlayersTab({ id }: { id: string }) {
  const { hasRole } = useAuth()
  const canManage = hasRole('operator')
  const playersQ = usePlayers(id)
  const addToList = useAddPlayerToList(id)

  return (
    <Stack gap="md">
      <PlayersOnlinePanel data={playersQ.data} isLoading={playersQ.isLoading} />

      <KnownPlayersTable
        players={playersQ.data?.known ?? []}
        isLoading={playersQ.isLoading}
        canManageLists={canManage}
        onAddToList={(kind, playerId) => addToList.mutate({ kind, playerId })}
      />

      <Card withBorder>
        <Title order={4} mb="sm">
          Lists
        </Title>
        <Tabs defaultValue="admin" keepMounted={false}>
          <Tabs.List>
            {LIST_KINDS.map((kind) => (
              <Tabs.Tab key={kind} value={kind}>
                {LIST_KIND_LABELS[kind]}
              </Tabs.Tab>
            ))}
          </Tabs.List>
          {LIST_KINDS.map((kind) => (
            <Tabs.Panel key={kind} value={kind} pt="md">
              <ListEditor id={id} kind={kind} canEdit={canManage} />
            </Tabs.Panel>
          ))}
        </Tabs>
      </Card>
    </Stack>
  )
}

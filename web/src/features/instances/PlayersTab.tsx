import { Stack, Tabs } from '@mantine/core'
import { useAuth } from '../../auth/useAuth'
import { SectionCard } from '../../ui'
import { ListEditor } from './players/ListEditor'
import { KnownPlayersTable } from './players/KnownPlayersTable'
import { PlayersOnlinePanel } from './players/PlayersOnlinePanel'
import { LIST_KINDS, LIST_KIND_LABELS } from './players/constants'
import { useAddPlayerToList, usePlayers } from './players/usePlayers'
import { useAgent, useAgentCommand } from '../agent'

// Owned by WP-12 (docs/WORKPLAN.md). Props: the instance id.
export function PlayersTab({ id }: { id: string }) {
  const { hasRole } = useAuth()
  const canManage = hasRole('operator')
  const playersQ = usePlayers(id)
  const addToList = useAddPlayerToList(id)
  const agent = useAgent(id)
  const agentCommand = useAgentCommand(id)
  const canKick = canManage && !!agent.data?.connected

  return (
    <Stack gap="md">
      <PlayersOnlinePanel
        data={playersQ.data}
        isLoading={playersQ.isLoading}
        onKick={
          canKick
            ? (p) => agentCommand.mutate({ command: 'kick', target: p.platform_id ?? p.name })
            : undefined
        }
        kickPending={agentCommand.isPending}
      />

      <KnownPlayersTable
        players={playersQ.data?.known ?? []}
        isLoading={playersQ.isLoading}
        canManageLists={canManage}
        onAddToList={(kind, playerId) => addToList.mutate({ kind, playerId })}
      />

      <SectionCard title="Lists" description="Admins, banned and permitted players for this instance.">
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
      </SectionCard>
    </Stack>
  )
}

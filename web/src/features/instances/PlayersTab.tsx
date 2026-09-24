import { SegmentedControl, Stack } from '@mantine/core'
import { useSearchParams } from 'react-router-dom'
import { useAuth } from '../../auth/useAuth'
import type { ListKind } from '../../api/types'
import { LoadError, SectionCard } from '../../ui'
import { ListEditor } from './players/ListEditor'
import { KnownPlayersTable } from './players/KnownPlayersTable'
import { PlayersOnlinePanel } from './players/PlayersOnlinePanel'
import { LIST_KINDS, LIST_KIND_LABELS } from './players/constants'
import { useAddPlayerToList, usePlayers } from './players/usePlayers'
import { AgentSetupNotice, useAgent, useAgentCommand } from '../agent'

function isListKind(v: string | null): v is ListKind {
  return v !== null && (LIST_KINDS as string[]).includes(v)
}

// Owned by WP-12 (docs/WORKPLAN.md). Props: the instance id.
export function PlayersTab({ id }: { id: string }) {
  const { hasRole } = useAuth()
  const canManage = hasRole('operator')
  const playersQ = usePlayers(id)
  const addToList = useAddPlayerToList(id)
  const agent = useAgent(id)
  const agentCommand = useAgentCommand(id)
  const canKick = canManage && !!agent.data?.connected

  const [params, setParams] = useSearchParams()
  const rawList = params.get('list')
  const list: ListKind = isListKind(rawList) ? rawList : 'admin'
  const setList = (v: string) =>
    setParams(
      (prev) => {
        const n = new URLSearchParams(prev)
        n.set('list', v)
        return n
      },
      { replace: true },
    )

  if (playersQ.isError) {
    return (
      <Stack gap="md">
        <LoadError error={playersQ.error} title="Could not load players" onRetry={() => playersQ.refetch()} />
      </Stack>
    )
  }

  return (
    <Stack gap="md">
      <AgentSetupNotice id={id} context="players" />

      <PlayersOnlinePanel
        data={playersQ.data}
        isLoading={playersQ.isLoading}
        onKick={
          canKick
            ? (p) => agentCommand.mutate({ command: 'kick', target: p.platform_id ?? p.name })
            : undefined
        }
        kickPending={agentCommand.isPending}
        instanceId={id}
        agentConnected={!!agent.data?.connected}
        canOperate={canManage}
      />

      <KnownPlayersTable
        id={id}
        players={playersQ.data?.known ?? []}
        isLoading={playersQ.isLoading}
        canManageLists={canManage}
        onAddToList={(kind, playerId) => addToList.mutate({ kind, playerId })}
      />

      <SectionCard title="Lists" description="Admins, banned and permitted players for this instance.">
        <Stack gap="sm">
          <SegmentedControl
            size="xs"
            aria-label="Player list"
            value={list}
            onChange={setList}
            w={{ base: '100%', sm: 320 }}
            data={LIST_KINDS.map((k) => ({ value: k, label: LIST_KIND_LABELS[k] }))}
          />
          <ListEditor key={list} id={id} kind={list} canEdit={canManage} />
        </Stack>
      </SectionCard>
    </Stack>
  )
}

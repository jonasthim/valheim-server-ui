import { Skeleton, Stack, Text } from '@mantine/core'
import { useAuth } from '../../auth/useAuth'
import { useInstance } from './useInstance'
import { useDeleteWorld, useSetActiveWorld, useWorlds } from './worlds/useWorlds'
import { WorldsTable } from './worlds/WorldsTable'
import { WorldUploadCard } from './worlds/WorldUploadCard'

// Owned by WP-12 (docs/WORKPLAN.md). Props: the instance id.
export function WorldsTab({ id }: { id: string }) {
  const { hasRole } = useAuth()
  const canManage = hasRole('operator')
  const instanceQ = useInstance(id)
  const worldsQ = useWorlds(id)
  const setActive = useSetActiveWorld(id)
  const deleteWorld = useDeleteWorld(id)

  if (instanceQ.isLoading) return <Skeleton height={200} />

  const config = instanceQ.data?.config

  return (
    <Stack gap="md">
      <WorldsTable
        id={id}
        worlds={worldsQ.data ?? []}
        isLoading={worldsQ.isLoading}
        canManage={canManage}
        makeActivePending={setActive.isPending}
        deletePending={deleteWorld.isPending}
        onMakeActive={(name) => {
          if (!config) return
          setActive.mutate({ ...config, world: name })
        }}
        onDelete={(name) => deleteWorld.mutate(name)}
      />
      <Text size="xs" c="dimmed">
        Changing the active world requires a restart to take effect.
      </Text>

      {canManage && <WorldUploadCard id={id} />}
    </Stack>
  )
}

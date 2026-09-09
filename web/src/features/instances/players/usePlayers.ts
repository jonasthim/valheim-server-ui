// Query hooks for the players tab: online/known players and the three list
// editors (admin/permitted/banned). Kept separate from components per
// CLAUDE.md conventions.
import { useEffect } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../../../api/client'
import type { ListKind, PlayerList, PlayersResponse } from '../../../api/types'
import { onEvent } from '../../../events/useEvents'
import { notifyError, notifySuccess } from '../../../lib/notify'
import { LIST_KIND_LABELS } from './constants'

/**
 * GET /instances/{id}/players, refetched every 15s and patched immediately on
 * the `instance.players` SSE event (useEvents() also invalidates this exact
 * query key, which triggers the background refetch that fills in
 * online_count/count_source/connected_at once it lands).
 */
export function usePlayers(id: string) {
  const qc = useQueryClient()
  const query = useQuery({
    queryKey: ['instances', id, 'players'],
    queryFn: () => api.get<PlayersResponse>(`/instances/${id}/players`),
    enabled: !!id,
    refetchInterval: 15_000,
  })

  useEffect(() => {
    return onEvent('instance.players', (ev) => {
      if (ev.instance_id !== id) return
      qc.setQueryData<PlayersResponse | undefined>(['instances', id, 'players'], (prev) => {
        if (!prev) return prev
        const prevByKey = new Map(prev.online.map((p) => [p.platform_id ?? p.name, p]))
        const online = ev.online.map((p) => {
          const key = p.platform_id ?? p.name
          const existing = prevByKey.get(key)
          return { platform_id: p.platform_id, name: p.name, connected_at: existing?.connected_at }
        })
        return { ...prev, online, online_count: online.length }
      })
    })
  }, [id, qc])

  return query
}

function listKey(id: string, kind: ListKind) {
  return ['instances', id, 'lists', kind] as const
}

/** GET /instances/{id}/lists/{kind}. */
export function usePlayerList(id: string, kind: ListKind) {
  return useQuery({
    queryKey: listKey(id, kind),
    queryFn: () => api.get<PlayerList>(`/instances/${id}/lists/${kind}`),
    enabled: !!id,
  })
}

/** PUT /instances/{id}/lists/{kind} — replaces the whole list. */
export function useSavePlayerList(id: string, kind: ListKind) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (list: PlayerList) => api.put<PlayerList>(`/instances/${id}/lists/${kind}`, list),
    onSuccess: (data) => {
      qc.setQueryData(listKey(id, kind), data)
      notifySuccess(`${LIST_KIND_LABELS[kind]} list saved`)
    },
    onError: (err) => notifyError(err, `Could not save ${LIST_KIND_LABELS[kind]} list`),
  })
}

/**
 * One-click "add this known player to a list": GETs the current list (or
 * reuses the cached copy) and PUTs it back with the id appended, a no-op if
 * it is already present.
 */
export function useAddPlayerToList(id: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ kind, playerId }: { kind: ListKind; playerId: string }) => {
      const current =
        qc.getQueryData<PlayerList>(listKey(id, kind)) ?? (await api.get<PlayerList>(`/instances/${id}/lists/${kind}`))
      if (current.entries.some((e) => e.id === playerId)) return { data: current, kind, added: false }
      const next: PlayerList = { kind, entries: [...current.entries, { id: playerId }] }
      const data = await api.put<PlayerList>(`/instances/${id}/lists/${kind}`, next)
      return { data, kind, added: true }
    },
    onSuccess: ({ data, kind, added }) => {
      qc.setQueryData(listKey(id, kind), data)
      notifySuccess(added ? `Added to ${LIST_KIND_LABELS[kind]}` : `Already in ${LIST_KIND_LABELS[kind]}`)
    },
    onError: (err) => notifyError(err, 'Could not update list'),
  })
}

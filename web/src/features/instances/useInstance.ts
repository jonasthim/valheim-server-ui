import { useQuery } from '@tanstack/react-query'
import { api } from '../../api/client'
import type { Instance } from '../../api/types'

// Shared query for one instance; tabs reuse it (WP-11 owns).
export function useInstance(id: string) {
  return useQuery({
    queryKey: ['instances', id, 'detail'],
    queryFn: () => api.get<{ instance: Instance }>(`/instances/${id}`).then((r) => r.instance),
    enabled: !!id,
  })
}


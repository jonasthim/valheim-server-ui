// Query/mutation hooks for the (instance-independent) Thunderstore index:
// search, package detail, categories, and the on-demand index refresh.
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../../api/client'
import type { Job, Package, PackageSearchResult } from '../../api/types'
import { notifyError, notifySuccess } from '../../lib/notify'

export interface ThunderstoreSearchParams {
  q?: string
  category?: string
  sort?: 'rating' | 'downloads' | 'updated' | 'name'
  include_deprecated?: boolean
  page?: number
  page_size?: number
}

/** GET /thunderstore/packages */
export function useThunderstoreSearch(params: ThunderstoreSearchParams) {
  return useQuery({
    queryKey: ['thunderstore', 'packages', params],
    queryFn: () =>
      api.get<PackageSearchResult>('/thunderstore/packages', {
        q: params.q,
        category: params.category,
        sort: params.sort,
        include_deprecated: params.include_deprecated,
        page: params.page ?? 1,
        page_size: params.page_size ?? 30,
      }),
    placeholderData: (prev) => prev,
  })
}

/** GET /thunderstore/packages/{owner}/{name} — full detail with versions. */
export function usePackage(owner: string | undefined, name: string | undefined) {
  return useQuery({
    queryKey: ['thunderstore', 'package', owner, name],
    queryFn: () => api.get<Package>(`/thunderstore/packages/${owner}/${name}`),
    enabled: !!owner && !!name,
  })
}

/** GET /thunderstore/categories */
export function useCategories() {
  return useQuery({
    queryKey: ['thunderstore', 'categories'],
    queryFn: () => api.get<{ categories: string[] }>('/thunderstore/categories').then((r) => r.categories),
    staleTime: 5 * 60_000,
  })
}

/** POST /thunderstore/refresh — re-fetch the Thunderstore package index. */
export function useRefreshThunderstoreIndex() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () => api.post<{ job: Job }>('/thunderstore/refresh'),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['thunderstore'] })
      notifySuccess('Thunderstore index refresh queued')
    },
    onError: (err) => notifyError(err, 'Could not refresh index'),
  })
}

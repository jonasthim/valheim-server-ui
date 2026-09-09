// Thunderstore package browser: search/filter/paginate the cached index and
// install a package (with its dependency plan resolved server-side and
// printed into the resulting job's log). See docs/ARCHITECTURE.md §12.
import { useState } from 'react'
import {
  Alert,
  Anchor,
  Avatar,
  Badge,
  Box,
  Button,
  Card,
  Checkbox,
  Group,
  List,
  Modal,
  Pagination,
  Popover,
  ScrollArea,
  SegmentedControl,
  Select,
  Skeleton,
  Stack,
  Text,
  TextInput,
  Tooltip,
} from '@mantine/core'
import { useDebouncedValue } from '@mantine/hooks'
import { IconDatabase, IconExternalLink, IconRefresh, IconSearch } from '@tabler/icons-react'
import { useAuth } from '../../auth/useAuth'
import { useJobDrawer } from '../jobs'
import { fmtAgo } from '../../lib/format'
import type { Mod, PackageSummary } from '../../api/types'
import { findInstalledMod, SORT_OPTIONS } from './helpers'
import { useCategories, usePackage, useRefreshThunderstoreIndex, useThunderstoreSearch } from './useThunderstore'
import { useInstallPackage } from './useMods'

const PAGE_SIZE = 30

export function ThunderstoreBrowser({
  id,
  opened,
  onClose,
  installedMods,
}: {
  id: string
  opened: boolean
  onClose: () => void
  installedMods: Mod[]
}) {
  const { hasRole } = useAuth()
  const canOperate = hasRole('operator')
  const { openJob } = useJobDrawer()

  const [query, setQuery] = useState('')
  const [debouncedQuery] = useDebouncedValue(query, 300)
  const [category, setCategory] = useState<string>('')
  const [sort, setSort] = useState<'rating' | 'downloads' | 'updated' | 'name'>('rating')
  const [includeDeprecated, setIncludeDeprecated] = useState(false)
  const [page, setPage] = useState(1)

  // Any filter change jumps back to page 1, applied at the event that caused
  // it rather than as an effect keyed on the (debounced) search value.
  function updateQuery(v: string) {
    setQuery(v)
    setPage(1)
  }
  function updateCategory(v: string) {
    setCategory(v)
    setPage(1)
  }
  function updateSort(v: typeof sort) {
    setSort(v)
    setPage(1)
  }
  function updateIncludeDeprecated(v: boolean) {
    setIncludeDeprecated(v)
    setPage(1)
  }

  const categoriesQuery = useCategories()
  const searchQuery = useThunderstoreSearch({
    q: debouncedQuery || undefined,
    category: category || undefined,
    sort,
    include_deprecated: includeDeprecated,
    page,
    page_size: PAGE_SIZE,
  })
  const refreshIndex = useRefreshThunderstoreIndex()

  const result = searchQuery.data
  const totalPages = result ? Math.max(1, Math.ceil(result.total / (result.page_size || PAGE_SIZE))) : 1
  const emptyIndex = !!result && result.total === 0 && !debouncedQuery && !category

  function requestRefresh() {
    refreshIndex.mutate(undefined, { onSuccess: (res) => openJob(res.job.id) })
  }

  return (
    <Modal opened={opened} onClose={onClose} title="Browse Thunderstore" size="xl">
      <Stack gap="sm">
        <Group align="flex-end" wrap="wrap" gap="sm">
          <TextInput
            label="Search"
            placeholder="Package or author name"
            leftSection={<IconSearch size={14} />}
            value={query}
            onChange={(e) => updateQuery(e.currentTarget.value)}
            style={{ flex: 1, minWidth: 200 }}
          />
          <Select
            label="Category"
            placeholder="All categories"
            data={categoriesQuery.data ?? []}
            value={category || null}
            onChange={(v) => updateCategory(v ?? '')}
            clearable
            w={180}
          />
          <Box>
            <Text size="sm" fw={500} mb={4}>
              Sort by
            </Text>
            <SegmentedControl
              size="xs"
              value={sort}
              onChange={(v) => updateSort(v as typeof sort)}
              data={SORT_OPTIONS.map((o) => ({ value: o.value, label: o.label }))}
            />
          </Box>
          <Checkbox
            label="Include deprecated"
            checked={includeDeprecated}
            onChange={(e) => updateIncludeDeprecated(e.currentTarget.checked)}
            mb={4}
          />
        </Group>

        <Group justify="space-between">
          <Text size="xs" c="dimmed">
            {result ? `Index last updated ${fmtAgo(result.index_updated_at)}` : ' '}
          </Text>
          {canOperate && (
            <Button
              size="xs"
              variant="subtle"
              leftSection={<IconRefresh size={14} />}
              loading={refreshIndex.isPending}
              onClick={requestRefresh}
            >
              Refresh index
            </Button>
          )}
        </Group>

        {searchQuery.isLoading ? (
          <Stack gap="xs">
            {Array.from({ length: 4 }).map((_, i) => (
              <Skeleton key={i} height={72} />
            ))}
          </Stack>
        ) : emptyIndex ? (
          <Alert color="gray" icon={<IconDatabase size={16} />} title="Thunderstore index is empty">
            <Stack gap="xs">
              <Text size="sm">
                No cached packages yet. {canOperate ? 'Refresh the index to fetch the Thunderstore catalog.' : 'Ask an operator to refresh the index.'}
              </Text>
              {canOperate && (
                <Button size="xs" loading={refreshIndex.isPending} onClick={requestRefresh}>
                  Refresh index
                </Button>
              )}
            </Stack>
          </Alert>
        ) : !result || result.packages.length === 0 ? (
          <Text c="dimmed" ta="center" py="xl">
            No packages match these filters.
          </Text>
        ) : (
          <ScrollArea.Autosize mah={460}>
            <Stack gap="xs">
              {result.packages.map((pkg) => (
                <PackageRow
                  key={pkg.full_name}
                  id={id}
                  pkg={pkg}
                  installed={findInstalledMod(installedMods, pkg.owner, pkg.name)}
                  canOperate={canOperate}
                />
              ))}
            </Stack>
          </ScrollArea.Autosize>
        )}

        {result && totalPages > 1 && (
          <Group justify="center">
            <Pagination total={totalPages} value={page} onChange={setPage} size="sm" />
          </Group>
        )}
      </Stack>
    </Modal>
  )
}

function PackageRow({
  id,
  pkg,
  installed,
  canOperate,
}: {
  id: string
  pkg: PackageSummary
  installed: Mod | undefined
  canOperate: boolean
}) {
  return (
    <Card withBorder padding="sm">
      <Group justify="space-between" align="flex-start" wrap="nowrap">
        <Group align="flex-start" wrap="nowrap" gap="sm" style={{ flex: 1, minWidth: 0 }}>
          <Avatar src={pkg.icon_url || undefined} size={44} radius="sm" />
          <Box style={{ flex: 1, minWidth: 0 }}>
            <Group gap={6} wrap="wrap">
              <Text fw={600} size="sm">
                {pkg.name}
              </Text>
              <Text size="xs" c="dimmed">
                by {pkg.owner}
              </Text>
              {pkg.is_deprecated && (
                <Badge color="red" size="xs" variant="light">
                  deprecated
                </Badge>
              )}
              {installed && (
                <Badge color="green" size="xs" variant="light">
                  Installed v{installed.version}
                </Badge>
              )}
              {pkg.package_url && (
                <Anchor href={pkg.package_url} target="_blank" rel="noreferrer" size="xs">
                  <Group gap={2} wrap="nowrap">
                    Thunderstore <IconExternalLink size={10} />
                  </Group>
                </Anchor>
              )}
            </Group>
            {pkg.description && (
              <Text size="xs" c="dimmed" lineClamp={2}>
                {pkg.description}
              </Text>
            )}
            <Group gap={4} mt={4} wrap="wrap">
              {pkg.categories.slice(0, 5).map((c) => (
                <Badge key={c} size="xs" variant="outline">
                  {c}
                </Badge>
              ))}
            </Group>
            <Group gap="md" mt={4}>
              <Text size="xs" c="dimmed">
                ★ {pkg.rating_score}
              </Text>
              <Text size="xs" c="dimmed">
                {pkg.total_downloads.toLocaleString()} downloads
              </Text>
              <Text size="xs" c="dimmed">
                updated {fmtAgo(pkg.date_updated)}
              </Text>
            </Group>
          </Box>
        </Group>
        {canOperate && <InstallButton id={id} pkg={pkg} />}
      </Group>
    </Card>
  )
}

function InstallButton({ id, pkg }: { id: string; pkg: PackageSummary }) {
  const [opened, setOpened] = useState(false)
  // The user's explicit pick, if any; otherwise default to the package's
  // latest version once its detail has loaded — derived during render so no
  // effect is needed to sync query data into local state.
  const [explicitVersion, setExplicitVersion] = useState<string | undefined>(undefined)
  const { openJob } = useJobDrawer()
  const install = useInstallPackage(id)
  const detailQuery = usePackage(opened ? pkg.owner : undefined, opened ? pkg.name : undefined)
  const version = explicitVersion ?? detailQuery.data?.latest_version

  function toggle() {
    setOpened((o) => !o)
  }

  function confirmInstall() {
    if (!version) return
    install.mutate(
      { owner: pkg.owner, name: pkg.name, version },
      {
        onSuccess: (res) => {
          setOpened(false)
          setExplicitVersion(undefined)
          openJob(res.job.id)
        },
      },
    )
  }

  const selectedVersion = detailQuery.data?.versions.find((v) => v.version === version)

  return (
    <Popover opened={opened} onChange={setOpened} width={300} withArrow shadow="md" position="bottom-end">
      <Popover.Target>
        <Button size="xs" onClick={toggle}>
          Install
        </Button>
      </Popover.Target>
      <Popover.Dropdown>
        <Stack gap="xs">
          <Text size="sm" fw={600}>
            Install {pkg.name}
          </Text>
          {detailQuery.isLoading ? (
            <Skeleton height={70} />
          ) : (
            <>
              <Select
                label="Version"
                data={(detailQuery.data?.versions ?? []).map((v) => ({ value: v.version, label: v.version }))}
                value={version ?? null}
                onChange={(v) => setExplicitVersion(v ?? undefined)}
                allowDeselect={false}
              />
              {selectedVersion && selectedVersion.dependencies.length > 0 && (
                <Box>
                  <Text size="xs" c="dimmed" mb={2}>
                    Dependencies
                  </Text>
                  <List size="xs" spacing={2}>
                    {selectedVersion.dependencies.map((dep) => (
                      <List.Item key={dep}>{dep}</List.Item>
                    ))}
                  </List>
                </Box>
              )}
              <Group justify="flex-end" gap="xs">
                <Button size="xs" variant="default" onClick={() => setOpened(false)}>
                  Cancel
                </Button>
                <Tooltip label="The full dependency plan is printed to the job log">
                  <Button size="xs" loading={install.isPending} disabled={!version} onClick={confirmInstall}>
                    Install
                  </Button>
                </Tooltip>
              </Group>
            </>
          )}
        </Stack>
      </Popover.Dropdown>
    </Popover>
  )
}

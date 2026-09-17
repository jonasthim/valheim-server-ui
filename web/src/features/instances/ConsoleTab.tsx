import { useEffect, useRef, useState } from 'react'
import { Badge, Button, Group, ScrollArea, Select, Stack, Switch, Text, TextInput } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { IconChevronRight, IconDownload, IconTrash } from '@tabler/icons-react'
import { api } from '../../api/client'
import { onEvent } from '../../events/useEvents'
import type { LogFileInfo, LogMatch, PlayersResponse } from '../../api/types'
import type { PlayersEvent } from '../../events/useEvents'
import { SectionCard } from '../../ui'
import { notifyError } from '../../lib/notify'
import { highlightColor } from './instanceHelpers'
import { useLogFiles, useLogFileTail, useLogSearch } from './useLogs'
import classes from './ConsoleTab.module.css'

const MAX_LINES = 2000
const FOLLOW_THRESHOLD_PX = 40
// The live console log's file name (domain.InstancePaths.ConsoleLog()'s base
// name); selecting it in the log picker is equivalent to "live console".
const LIVE_LOG_NAME = 'console.log'

function logFileLabel(f: LogFileInfo): string {
  if (f.kind === 'console') return `${f.name} (live)`
  if (f.kind === 'bepinex') return `${f.name} (BepInEx)`
  return f.name
}

// One rendered log line, shared by the live console view and the static file
// viewer so both look identical.
function LogLine({ line }: { line: string }) {
  const color = highlightColor(line)
  return (
    <Text
      size="xs"
      ff="monospace"
      style={color ? { backgroundColor: `var(--mantine-color-${color}-light)`, borderRadius: 4, padding: '0 4px' } : undefined}
    >
      {line}
    </Text>
  )
}

// Owned by WP-11. Props: the instance id.
export function ConsoleTab({ id }: { id: string }) {
  // `lines` is derived at render time from the initial tail fetch plus a
  // ring buffer of lines that arrived live via SSE, rather than copied into
  // its own state (which would mean syncing external state via an effect).
  const [liveLines, setLiveLines] = useState<string[]>([])
  const [initialCleared, setInitialCleared] = useState(false)
  const [filter, setFilter] = useState('')
  const [follow, setFollow] = useState(true)
  const [selectedLog, setSelectedLog] = useState(LIVE_LOG_NAME)
  const [regexEnabled, setRegexEnabled] = useState(false)
  const [searchResults, setSearchResults] = useState<LogMatch[] | null>(null)
  // null until a live instance.players event arrives; the players query seeds the chips before that.
  const [liveOnline, setLiveOnline] = useState<PlayersEvent['online'] | null>(null)
  const viewportRef = useRef<HTMLDivElement | null>(null)

  const isLive = selectedLog === LIVE_LOG_NAME

  const initialQuery = useQuery({
    queryKey: ['instances', id, 'logs', 'initial'],
    queryFn: () => api.get<{ lines: string[] }>(`/instances/${id}/logs`, { lines: 500 }).then((r) => r.lines),
    enabled: !!id,
  })

  const playersQuery = useQuery({
    queryKey: ['instances', id, 'players'],
    queryFn: () => api.get<PlayersResponse>(`/instances/${id}/players`),
    enabled: !!id,
  })
  const online: PlayersEvent['online'] = liveOnline ?? playersQuery.data?.online ?? []

  const logFilesQuery = useLogFiles(id)
  const fileTailQuery = useLogFileTail(id, selectedLog, !isLive)
  const logSearch = useLogSearch(id)

  const logFiles = logFilesQuery.data ?? []
  const selectData = (
    logFiles.some((f) => f.name === LIVE_LOG_NAME) ? logFiles : [{ name: LIVE_LOG_NAME, kind: 'console' } as LogFileInfo, ...logFiles]
  ).map((f) => ({ value: f.name, label: logFileLabel(f) }))

  const initialLines = initialCleared ? [] : (initialQuery.data ?? [])
  const combined = [...initialLines, ...liveLines]
  const lines = combined.length > MAX_LINES ? combined.slice(combined.length - MAX_LINES) : combined

  useEffect(() => {
    return onEvent('instance.log', (e) => {
      if (e.instance_id !== id) return
      setLiveLines((prev) => (prev.length >= MAX_LINES ? [...prev.slice(1), e.line] : [...prev, e.line]))
    })
  }, [id])

  useEffect(() => {
    return onEvent('instance.players', (e) => {
      if (e.instance_id !== id) return
      setLiveOnline(e.online)
    })
  }, [id])

  useEffect(() => {
    // Only the live view auto-follows; a static file's own tail doesn't
    // change while it's open, and the live buffer keeps updating in the
    // background regardless of which view is on screen.
    if (isLive && follow && viewportRef.current) {
      viewportRef.current.scrollTo({ top: viewportRef.current.scrollHeight })
    }
  }, [lines, follow, isLive])

  function handleScroll({ y }: { x: number; y: number }) {
    const vp = viewportRef.current
    if (!vp) return
    const distanceFromBottom = vp.scrollHeight - y - vp.clientHeight
    setFollow(distanceFromBottom < FOLLOW_THRESHOLD_PX)
  }

  const filtered = filter ? lines.filter((l) => l.toLowerCase().includes(filter.toLowerCase())) : lines
  const displayLines = isLive ? filtered : (fileTailQuery.data ?? [])

  const downloadHref = isLive
    ? api.url(`/instances/${id}/logs/download`)
    : api.url(`/instances/${id}/logs/files/${encodeURIComponent(selectedLog)}/download`)

  function runSearch() {
    logSearch.mutate(
      { q: filter, regex: regexEnabled },
      {
        onSuccess: (matches) => setSearchResults(matches),
        onError: (err) => notifyError(err, 'Could not search logs'),
      },
    )
  }

  return (
    <Stack>
      <Group justify="space-between" wrap="wrap">
        <Group gap="xs">
          <Text size="sm" fw={600}>
            Online:
          </Text>
          {online.length === 0 && (
            <Text size="sm" c="dimmed">
              no players seen yet
            </Text>
          )}
          {online.map((p) => (
            <Badge key={p.platform_id ?? p.name} variant="light" color="green">
              {p.name}
            </Badge>
          ))}
        </Group>
        <Select
          size="xs"
          w={260}
          label="Log"
          data={selectData}
          value={selectedLog}
          onChange={(v) => setSelectedLog(v ?? LIVE_LOG_NAME)}
          allowDeselect={false}
        />
      </Group>

      <SectionCard flush>
        <div className={classes.terminal}>
          <div className={classes.toolbar}>
            <Text size="xs" c="dimmed">
              {isLive
                ? `${filtered.length} of ${lines.length} lines shown (buffer holds the last ${MAX_LINES})`
                : `${displayLines.length} lines shown from ${selectedLog}`}
            </Text>
            <Group gap="sm" wrap="wrap">
              <Switch size="xs" label="Auto-follow" checked={follow} onChange={(e) => setFollow(e.currentTarget.checked)} />
              <Button
                size="xs"
                variant="subtle"
                color="gray"
                leftSection={<IconTrash size={14} />}
                onClick={() => {
                  setInitialCleared(true)
                  setLiveLines([])
                }}
              >
                Clear view
              </Button>
              <Button
                size="xs"
                variant="subtle"
                color="gray"
                component="a"
                href={downloadHref}
                target="_blank"
                rel="noreferrer"
                leftSection={<IconDownload size={14} />}
              >
                Download
              </Button>
            </Group>
          </div>

          <ScrollArea h={440} viewportRef={viewportRef} onScrollPositionChange={handleScroll} className={classes.screen}>
            <Stack gap={2} p="sm">
              {displayLines.map((line, i) => (
                <LogLine key={i} line={line} />
              ))}
              {displayLines.length === 0 && (
                <Text size="sm" c="dimmed">
                  {isLive ? 'No log lines yet.' : fileTailQuery.isLoading ? 'Loading…' : 'This log file is empty.'}
                </Text>
              )}
            </Stack>
          </ScrollArea>

          <div className={classes.promptRow} style={{ flexWrap: 'wrap', rowGap: 6 }}>
            <IconChevronRight size={16} className={classes.promptGlyph} aria-hidden />
            <TextInput
              className={classes.promptInput}
              variant="unstyled"
              placeholder="Filter lines"
              value={filter}
              disabled={!isLive}
              onChange={(e) => setFilter(e.currentTarget.value)}
            />
            <Switch size="xs" label="Regex" checked={regexEnabled} onChange={(e) => setRegexEnabled(e.currentTarget.checked)} />
            <Button size="xs" variant="light" onClick={runSearch} loading={logSearch.isPending} disabled={!filter.trim()}>
              Search all logs
            </Button>
          </div>
        </div>
      </SectionCard>

      {searchResults !== null && (
        <SectionCard
          title={searchResults.length === 0 ? 'No matches in any log file.' : `${searchResults.length} match${searchResults.length === 1 ? '' : 'es'}`}
          actions={
            <Button size="xs" variant="subtle" color="gray" onClick={() => setSearchResults(null)}>
              Clear
            </Button>
          }
        >
          {searchResults.length > 0 && (
            <Stack gap={6}>
              {searchResults.map((m, i) => (
                <Group key={i} gap="xs" wrap="nowrap" align="flex-start">
                  <Badge size="xs" variant="light" color="gray" style={{ flexShrink: 0 }}>
                    {m.file}
                  </Badge>
                  <Text size="xs" ff="monospace" style={{ overflowWrap: 'anywhere' }}>
                    {m.line}
                  </Text>
                </Group>
              ))}
            </Stack>
          )}
        </SectionCard>
      )}
    </Stack>
  )
}

import { useEffect, useRef, useState } from 'react'
import { Badge, Button, Group, ScrollArea, Stack, Switch, Text, TextInput } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { IconChevronRight, IconDownload, IconTrash } from '@tabler/icons-react'
import { api } from '../../api/client'
import { onEvent } from '../../events/useEvents'
import type { PlayersResponse } from '../../api/types'
import type { PlayersEvent } from '../../events/useEvents'
import { SectionCard } from '../../ui'
import { highlightColor } from './instanceHelpers'
import classes from './ConsoleTab.module.css'

const MAX_LINES = 2000
const FOLLOW_THRESHOLD_PX = 40

// Owned by WP-11. Props: the instance id.
export function ConsoleTab({ id }: { id: string }) {
  // `lines` is derived at render time from the initial tail fetch plus a
  // ring buffer of lines that arrived live via SSE, rather than copied into
  // its own state (which would mean syncing external state via an effect).
  const [liveLines, setLiveLines] = useState<string[]>([])
  const [initialCleared, setInitialCleared] = useState(false)
  const [filter, setFilter] = useState('')
  const [follow, setFollow] = useState(true)
  // null until a live instance.players event arrives; the players query seeds the chips before that.
  const [liveOnline, setLiveOnline] = useState<PlayersEvent['online'] | null>(null)
  const viewportRef = useRef<HTMLDivElement | null>(null)

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
    if (follow && viewportRef.current) {
      viewportRef.current.scrollTo({ top: viewportRef.current.scrollHeight })
    }
  }, [lines, follow])

  function handleScroll({ y }: { x: number; y: number }) {
    const vp = viewportRef.current
    if (!vp) return
    const distanceFromBottom = vp.scrollHeight - y - vp.clientHeight
    setFollow(distanceFromBottom < FOLLOW_THRESHOLD_PX)
  }

  const filtered = filter ? lines.filter((l) => l.toLowerCase().includes(filter.toLowerCase())) : lines

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
      </Group>

      <SectionCard flush>
        <div className={classes.terminal}>
          <div className={classes.toolbar}>
            <Text size="xs" c="dimmed">
              {filtered.length} of {lines.length} lines shown (buffer holds the last {MAX_LINES})
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
                href={api.url(`/instances/${id}/logs/download`)}
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
              {filtered.map((line, i) => {
                const color = highlightColor(line)
                return (
                  <Text
                    key={i}
                    size="xs"
                    ff="monospace"
                    style={color ? { backgroundColor: `var(--mantine-color-${color}-light)`, borderRadius: 4, padding: '0 4px' } : undefined}
                  >
                    {line}
                  </Text>
                )
              })}
              {filtered.length === 0 && (
                <Text size="sm" c="dimmed">
                  No log lines yet.
                </Text>
              )}
            </Stack>
          </ScrollArea>

          <div className={classes.promptRow}>
            <IconChevronRight size={16} className={classes.promptGlyph} aria-hidden />
            <TextInput
              className={classes.promptInput}
              variant="unstyled"
              placeholder="Filter lines"
              value={filter}
              onChange={(e) => setFilter(e.currentTarget.value)}
            />
          </div>
        </div>
      </SectionCard>
    </Stack>
  )
}

import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Button, Drawer, Group, Paper, RangeSlider, Select, Stack, Switch, Text, Title } from '@mantine/core'
import { IconRefresh } from '@tabler/icons-react'
import { api } from '../../api/client'
import type { SurvivalHistory, SurvivalMoment } from '../../api/types'
import { useAgent } from '../agent/useAgent'
import { AgentSetupNotice } from '../agent/AgentSetupNotice'
import { EmptyState, LoadError } from '../../ui'
import classes from './SurvivalTab.module.css'

type Category = 'enemy' | 'enemy_level' | 'situation' | 'biome'

function categoryValue(moment: SurvivalMoment, category: Category): string {
  if (category === 'enemy') return moment.enemy || 'Unknown'
  if (category === 'enemy_level') return moment.enemy_level ? `Level ${moment.enemy_level}` : 'Unknown'
  if (category === 'situation') return moment.situation || 'Unknown'
  return moment.biome || 'Unknown'
}

function locationLabel(moment: SurvivalMoment): string {
  if (moment.biome) return moment.biome
  if (typeof moment.x === 'number' && typeof moment.z === 'number') return `Position ${Math.round(moment.x)}, ${Math.round(moment.z)}`
  return 'Location unknown'
}

function DeathChart({ deaths, moments, highlighted, range, focus, onSelect }: {
  deaths: SurvivalMoment[]; moments: SurvivalMoment[]; highlighted: Set<number>;
  range: [number, number]; focus: boolean; onSelect: (m: SurvivalMoment) => void
}) {
  const visible = deaths.filter((m) => m.day >= range[0] && m.day <= range[1])
  const plotted = focus ? visible.filter((m) => highlighted.has(m.id)) : visible
  const rankById = new Map(deaths.map((m, index) => [m.id, index + 1]))
  const first = focus && plotted.length ? Math.min(...plotted.map((m) => m.day)) : range[0]
  const last = Math.max(first + 1, focus && plotted.length ? Math.max(...plotted.map((m) => m.day)) : range[1])
  const ranks = plotted.map((m) => rankById.get(m.id) ?? 0)
  const minY = focus && ranks.length ? Math.max(0, Math.min(...ranks) - 1) : 0
  const maxY = Math.max(minY + 1, focus && ranks.length ? Math.max(...ranks) : deaths.length)
  const x = (day: number) => 48 + ((day - first) / (last - first)) * 910
  const y = (rank: number) => 300 - ((rank - minY) / (maxY - minY)) * 250
  const ticks = [...new Set([0, 0.25, 0.5, 0.75, 1].map((fraction) => Math.round(minY + (maxY - minY) * fraction)))]
  return <div className={classes.chartScroll} role="group" aria-label="Deaths by world day, with notable world events">
    <svg className={classes.chart} viewBox="0 0 1000 340" role="presentation">
      {ticks.map((count) => <g key={count}>
        <line x1="48" x2="958" y1={y(count)} y2={y(count)} className={classes.grid} />
        <text x="40" y={y(count) + 4} textAnchor="end" className={classes.axis}>{count}</text>
      </g>)}
      {moments.filter((m) => m.kind !== 'death' && m.day >= first && m.day <= last).map((m) => <g key={m.id} role="button" tabIndex={0}
        aria-label={`Day ${m.day}: ${m.label}`} onClick={() => onSelect(m)}
        onKeyDown={(event) => { if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); onSelect(m) } }} className={classes.marker}>
        <line x1={x(m.day)} x2={x(m.day)} y1="36" y2="300" className={classes.milestone} />
        <title>{`Day ${m.day}: ${m.label}`}</title>
      </g>)}
      {plotted.map((m) => <circle key={m.id} cx={x(m.day)} cy={y(rankById.get(m.id) ?? 0)} r="5" role="button" tabIndex={0}
        aria-label={`Death ${rankById.get(m.id)}, day ${m.day}${m.player_name ? `, ${m.player_name}` : ''}`}
        className={highlighted.has(m.id) ? classes.point : classes.pointMuted}
        onClick={() => onSelect(m)} onKeyDown={(event) => { if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); onSelect(m) } }}>
        <title>{`Death ${rankById.get(m.id)}, day ${m.day}${m.player_name ? `, ${m.player_name}` : ''}`}</title>
      </circle>)}
      <text x="48" y="330" className={classes.axis}>{`Day ${first}`}</text>
      <text x="958" y="330" textAnchor="end" className={classes.axis}>{`Day ${last}`}</text>
    </svg>
  </div>
}

export function SurvivalTab({ id }: { id: string }) {
  const agent = useAgent(id)
  const [world, setWorld] = useState('0')
  const [character, setCharacter] = useState('')
  const [category, setCategory] = useState<Category>('biome')
  const [selection, setSelection] = useState('')
  const [focus, setFocus] = useState(false)
  const [range, setRange] = useState<[number, number] | null>(null)
  const [showAll, setShowAll] = useState(false)
  const [detail, setDetail] = useState<SurvivalMoment | null>(null)
  const history = useQuery({
    queryKey: ['instances', id, 'survival', world],
    queryFn: () => api.get<SurvivalHistory>(`/instances/${id}/survival?world_uid=${world}`),
    refetchInterval: 10_000,
  })
  const allMoments = history.data?.moments ?? []
  const moments = allMoments.filter((m) => !character || m.character_id === character || !m.character_id)
  const deaths = useMemo(() => moments.filter((m) => m.kind === 'death'), [moments])
  const hasEnemy = deaths.some((m) => m.enemy)
  const hasLevel = deaths.some((m) => m.enemy_level)
  const hasSituation = deaths.some((m) => m.situation)
  const maxDay = Math.max(1, ...(world === '0' ? [agent.data?.status?.world?.day ?? 0] : []), ...moments.map((m) => m.day))
  const selectedRange: [number, number] = range ? [Math.min(range[0], maxDay), Math.min(range[1], maxDay)] : [0, maxDay]
  const available = deaths.filter((m) => m.day >= selectedRange[0] && m.day <= selectedRange[1])
  const highlighted = available.filter((m) => !selection || categoryValue(m, category) === selection)
  const highlightedIds = new Set(highlighted.map((m) => m.id))
  const categories = [...new Set(available.map((m) => categoryValue(m, category)))].map((label) => ({ label, count: available.filter((m) => categoryValue(m, category) === label).length })).sort((a, b) => b.count - a.count)
  const players = [...new Map(allMoments.filter((m) => m.kind === 'death' && m.character_id).map((m) => [m.character_id!, m.player_name || m.character_id!])).entries()]
  const recent = [...moments].sort((a, b) => b.id - a.id)
  const worldOptions = [{ value: '0', label: agent.data?.connected ? 'Current world' : 'Latest recorded world' }, ...(history.data?.worlds ?? []).map((uid) => ({ value: uid, label: `World ${uid}` }))]

  return <Stack gap="md">
    <AgentSetupNotice id={id} context="survival" />
    <Group justify="space-between" align="start" gap="sm">
      <div><Text size="xs" c="dimmed">The long road back</Text><Title order={2}>Survival log</Title><Text size="sm" c="dimmed">Confirmed deaths and milestones from this server, starting when tracking was installed.</Text></div>
      <Button variant="default" leftSection={<IconRefresh size={15} />} onClick={() => void history.refetch()}>Refresh</Button>
    </Group>
    {history.isError && <LoadError error={history.error} title="Survival history unavailable" onRetry={() => history.refetch()} />}
    <Group gap="sm"><Select label="World" data={worldOptions} value={world} onChange={(value) => { setWorld(value ?? '0'); setRange(null); setSelection('') }} /><Select label="Player" data={[{ value: '', label: 'All players' }, ...players.map(([value, label]) => ({ value, label }))]} value={character} onChange={(value) => { setCharacter(value ?? ''); setSelection('') }} /></Group>
    {!history.isLoading && !history.isError && moments.length === 0 ? <EmptyState title="No survival moments yet" description="Deaths and world milestones will appear after the updated server agent observes them." /> : !history.isError && <>
      <div className={classes.summary}>
        <Paper withBorder className={classes.stat}><Text size="xs" c="dimmed">Deaths recorded</Text><Text className={classes.statValue}>{deaths.length}</Text></Paper>
        <Paper withBorder className={classes.stat}><Text size="xs" c="dimmed">World milestones</Text><Text className={classes.statValue}>{moments.length - deaths.length}</Text></Paper>
        <Paper withBorder className={classes.stat}><Text size="xs" c="dimmed">Known death biomes</Text><Text className={classes.statValue}>{deaths.filter((m) => m.biome).length}</Text></Paper>
      </div>
      <Paper withBorder className={classes.panel}>
        <Group justify="space-between"><div><Text size="xs" c="dimmed">The latest moments</Text><Title order={3}>Recent events</Title></div><Button variant="subtle" size="xs" onClick={() => setShowAll(!showAll)}>{showAll ? 'Show less' : 'Show more'}</Button></Group>
        <div className={classes.events}>{recent.slice(0, showAll ? 20 : 3).map((m) => <button type="button" key={m.id} className={classes.event} onClick={() => setDetail(m)}><strong>{m.kind === 'death' ? m.player_name || 'Player death' : m.kind === 'progression' ? 'Progression' : 'Setting change'}</strong><span>Day {m.day}</span><span>{m.kind === 'death' ? `${locationLabel(m)} · Cause unknown` : m.label}</span></button>)}</div>
      </Paper>
      <Paper withBorder className={classes.panel}>
        <Text size="xs" c="dimmed">The long road back</Text><Title order={3}>Death progression</Title><Text size="sm" c="dimmed">Each point is a death. Vertical lines mark world milestones. Select a point or line for details.</Text>
        <Group justify="space-between" mt="lg"><Text fw={600}>Highlight deaths <Text span size="xs" c="dimmed">{highlighted.length} match</Text></Text><Group><Switch label="Focus on matching days" checked={focus} onChange={(e) => setFocus(e.currentTarget.checked)} /><Button variant="subtle" size="xs" onClick={() => { setSelection(''); setRange(null) }}>Clear filters</Button></Group></Group>
        <Group mt="sm">{hasEnemy && <Select label="Enemy" value={category === 'enemy' ? selection : ''} data={[{ value: '', label: 'All' }, ...new Set(deaths.map((m) => m.enemy || 'Unknown'))].map((v) => typeof v === 'string' ? { value: v, label: v } : v)} onChange={(v) => { setCategory('enemy'); setSelection(v ?? '') }} />}<Select label="Location" value={category === 'biome' ? selection : ''} data={[{ value: '', label: 'All' }, ...new Set(deaths.map((m) => m.biome || 'Unknown'))].map((v) => typeof v === 'string' ? { value: v, label: v } : v)} onChange={(v) => { setCategory('biome'); setSelection(v ?? '') }} /></Group>
        <DeathChart deaths={deaths} moments={moments} highlighted={highlightedIds} range={selectedRange} focus={focus} onSelect={setDetail} />
        <RangeSlider min={0} max={maxDay} value={selectedRange} onChange={(value) => setRange(value)} minRange={0} marks={[{ value: 0, label: 'Day 0' }, { value: maxDay, label: `Day ${maxDay}` }]} mt="md" mb="xl" />
        <div className={classes.categoriesHead}><div><Text size="xs" c="dimmed">The many ways to fall</Text><Title order={3}>Deaths by category</Title></div><Group gap="xs">{([...(hasEnemy ? ['enemy'] : []), ...(hasLevel ? ['enemy_level'] : []), ...(hasSituation ? ['situation'] : []), 'biome'] as Category[]).map((c) => <Button key={c} size="xs" variant={category === c ? 'filled' : 'default'} onClick={() => { setCategory(c); setSelection('') }}>{c === 'enemy' ? 'Enemy' : c === 'enemy_level' ? 'Enemy level' : c === 'situation' ? 'Situation' : 'Location'}</Button>)}</Group></div>
        <Text size="xs" c="dimmed">Counts use the selected day range. Select a bar to highlight its deaths.</Text>
        <div className={classes.bars}>{categories.map(({ label, count }) => <button type="button" key={label} className={`${classes.barRow} ${selection === label ? classes.barSelected : ''}`} onClick={() => setSelection(selection === label ? '' : label)}><span>{label}</span><span className={classes.barTrack}><span style={{ width: `${(count / Math.max(1, categories[0]?.count ?? 1)) * 100}%` }} /></span><strong>{count}</strong></button>)}</div>
      </Paper>
    </>}
    <Drawer opened={detail !== null} onClose={() => setDetail(null)} title={detail?.kind === 'death' ? 'Recorded death' : 'World milestone'} position="right" closeButtonProps={{ 'aria-label': 'Close details' }}>
      {detail && <Stack gap="xs"><Title order={3}>{detail.kind === 'death' ? detail.player_name || 'Player death' : detail.label}</Title><Text>Day {detail.day} · {new Date(detail.at).toLocaleString()}</Text>{detail.kind === 'death' && <><Text>Cause: {detail.enemy || 'Unknown'}</Text><Text>Location: {locationLabel(detail)}</Text></>}{(detail.kind !== 'death' || detail.biome) && detail.x !== undefined && detail.z !== undefined && <Text size="sm" c="dimmed">Position: {Math.round(detail.x)}, {Math.round(detail.z)}</Text>}</Stack>}
    </Drawer>
  </Stack>
}

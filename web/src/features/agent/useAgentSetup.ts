// Where an instance stands on the way to a working agent: BepInEx, the
// plugin itself, whether it's enabled, and whether the manager can reach it.
// Drives the proactive notice (AgentSetupNotice) so the agent's absence is
// surfaced wherever it matters, not just as a Mods-tab secret.
import type { AgentInfo } from '../../api/types'
import { useInstance } from '../instances/useInstance'
import { useModsOverview } from '../mods/useMods'
import { useAgent } from './useAgent'

export type AgentSetupStage = 'bepinex' | 'install' | 'update' | 'disabled' | 'offline' | 'ready'

export interface AgentSetup {
  stage: AgentSetupStage
  /** Instance is running or starting (install/update stops and restarts it). */
  isRunning: boolean
  /** An agent or BepInEx install/update job is already queued or running. */
  activeInstallJob: boolean
  info: AgentInfo | undefined
}

export function useAgentSetup(id: string): AgentSetup {
  const agent = useAgent(id)
  const mods = useModsOverview(id)
  const instance = useInstance(id)

  const info = agent.data
  const state = instance.data?.status.state
  const isRunning = state === 'running' || state === 'starting'
  const activeJobType = instance.data?.status.active_job?.type
  const activeInstallJob = activeJobType === 'agent_install' || activeJobType === 'bepinex_install'

  let stage: AgentSetupStage
  if (!mods.data?.bepinex.installed) stage = 'bepinex'
  else if (!info || !info.installed) stage = 'install'
  else if (info.update_available) stage = 'update'
  else if (info.enabled === false) stage = 'disabled'
  else if (!info.connected) stage = 'offline'
  else stage = 'ready'

  return { stage, isRunning, activeInstallJob, info }
}

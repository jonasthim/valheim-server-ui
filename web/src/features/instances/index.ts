// Barrel for the instances feature (WP-11) so other areas (dashboard, jobs)
// can reuse its hooks/helpers without reaching into individual files, e.g.:
//   import { useStartInstance, stateColor } from '../instances'
export { useInstance } from './useInstance'
export {
  useInstanceStatus,
  useStartInstance,
  useStopInstance,
  useRestartInstance,
  useInstallInstance,
  useUpdateInstance,
  useCheckForUpdate,
  useSetAutostart,
  useDeleteInstance,
} from './instanceActions'
export {
  stateColor,
  stateLabel,
  isTransitioning,
  canStart,
  canStop,
  canRestart,
  canInstall,
  slugify,
  highlightColor,
  mapConfigFieldErrors,
  INSTANCE_ID_PATTERN,
  WORLD_NAME_PATTERN,
} from './instanceHelpers'
export { InstanceConfigForm } from './InstanceConfigForm'
export type { InstanceConfigFormInitial, InstanceConfigFormSubmit, FormHelpers } from './InstanceConfigForm'
export { DeleteInstanceModal } from './DeleteInstanceModal'
export { CreateInstancePage } from './CreateInstancePage'
export { OverviewTab } from './OverviewTab'
export { ConsoleTab } from './ConsoleTab'
export { ConfigTab } from './ConfigTab'

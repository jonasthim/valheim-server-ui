// Barrel for the system feature (WP-32: manager self-upgrade) so the
// dashboard and settings pages can reuse its hooks/components, e.g.:
//   import { useSystemInfo, AppUpdateBanner } from '../system'
export { useSystemInfo, useCheckAppUpdate, useUpgradeApp, useManagerRestartWatch, UPGRADE_EXPLANATION } from './useAppUpdate'
export { AppUpdateBanner } from './AppUpdateBanner'
export { ReleaseNotesModal } from './ReleaseNotesModal'
export { ManagerRestartOverlay } from './ManagerRestartOverlay'

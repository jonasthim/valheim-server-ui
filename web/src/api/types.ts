// Convenience aliases over the generated OpenAPI schema. Never hand-write API
// shapes; regenerate schema.d.ts with `npm run gen:api` when docs/openapi.yaml changes.
import type { components } from './schema'

export type Schemas = components['schemas']
export type Role = Schemas['Role']
export type User = Schemas['User']
export type AuthStatus = Schemas['AuthStatus']
export type Settings = Schemas['Settings']
export type OIDCSettings = Schemas['OIDCSettings']
export type SystemInfo = Schemas['SystemInfo']
export type Instance = Schemas['Instance']
export type InstanceConfig = Schemas['InstanceConfig']
export type InstanceStatus = Schemas['InstanceStatus']
export type InstanceState = Schemas['InstanceState']
export type CreateInstanceRequest = Schemas['CreateInstanceRequest']
export type UpdateInstanceRequest = Schemas['UpdateInstanceRequest']
export type UpdateInfo = Schemas['UpdateInfo']
export type PlayersResponse = Schemas['PlayersResponse']
export type PlayerList = Schemas['PlayerList']
export type ListKind = Schemas['ListKind']
export type World = Schemas['World']
export type Backup = Schemas['Backup']
export type Schedule = Schemas['Schedule']
export type ScheduleInput = Schemas['ScheduleInput']
export type Mod = Schemas['Mod']
export type ModsOverview = Schemas['ModsOverview']
export type ConfigFile = Schemas['ConfigFile']
export type ConfigFileInfo = Schemas['ConfigFileInfo']
export type ConfigEntry = Schemas['ConfigEntry']
export type PackageSummary = Schemas['PackageSummary']
export type Package = Schemas['Package']
export type PackageSearchResult = Schemas['PackageSearchResult']
export type Job = Schemas['Job']
export type JobStatus = Schemas['JobStatus']
export type AuditEntry = Schemas['AuditEntry']
export type ErrorResponse = Schemas['ErrorResponse']

export const ROLE_LEVEL: Record<Role, number> = { viewer: 1, operator: 2, admin: 3 }
export function roleAtLeast(role: Role | undefined, min: Role): boolean {
  return !!role && ROLE_LEVEL[role] >= ROLE_LEVEL[min]
}

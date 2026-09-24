// Hook owning the @mantine/form instance shared by CreateInstancePage (POST
// /instances) and ConfigTab (PATCH /instances/{id}) so both callers get
// identical fields/validation; the caller only sees the resolved
// InstanceConfig from toSubmit, never the form's internal (string-based) shape.
import { useState } from 'react'
import { useForm, type UseFormReturnType } from '@mantine/form'
import type { BackupKind, InstanceConfig, Modifiers, RemoteBackupConfig } from '../../api/types'
import { INSTANCE_ID_PATTERN, WORLD_NAME_PATTERN } from './instanceHelpers'

const DEFAULT_REMOTE_BACKUP_KINDS: BackupKind[] = ['manual', 'scheduled']

type ModifierKey = keyof Modifiers

interface ModifiersFormValues {
  combat: string
  deathpenalty: string
  resources: string
  raids: string
  portals: string
}

export interface ConfigFormValues {
  name: string
  world: string
  password: string
  port: number
  public: boolean
  crossplay: boolean
  preset: string
  modifiers: ModifiersFormValues
  setkeys: string[]
  save_interval_sec: number
  game_backups: number
  game_backup_short_sec: number
  game_backup_long_sec: number
  extra_args: string[]
  bepinex_enabled: boolean
  backup_keep_last: number
  backup_keep_days: number
  backup_before_update: boolean
  remote_backup_target_id: string
  remote_backup_kinds: BackupKind[]
}

export interface InstanceConfigFormValues {
  id: string
  name: string
  config: ConfigFormValues
  autostart: boolean
  install: boolean
}

export interface InstanceConfigFormInitial {
  id?: string
  name: string
  config: InstanceConfig
  autostart: boolean
}

export interface InstanceConfigFormSubmit {
  id: string
  name: string
  config: InstanceConfig
  autostart: boolean
  install: boolean
}

export interface FormHelpers {
  setErrors: (errors: Record<string, string>) => void
}

export interface InstanceConfigFormApi {
  form: UseFormReturnType<InstanceConfigFormValues>
  mode: 'create' | 'edit'
  passwordMasked: boolean
  passwordUnlocked: boolean
  unlockPassword: () => void
  toSubmit: (values: InstanceConfigFormValues) => InstanceConfigFormSubmit
}

function configToForm(config: InstanceConfig): ConfigFormValues {
  const m = config.modifiers ?? {}
  return {
    name: config.name,
    world: config.world,
    password: config.password,
    port: config.port,
    public: config.public,
    crossplay: config.crossplay,
    preset: config.preset ?? '',
    modifiers: {
      combat: m.combat ?? '',
      deathpenalty: m.deathpenalty ?? '',
      resources: m.resources ?? '',
      raids: m.raids ?? '',
      portals: m.portals ?? '',
    },
    setkeys: config.setkeys ?? [],
    save_interval_sec: config.save_interval_sec,
    game_backups: config.game_backups,
    game_backup_short_sec: config.game_backup_short_sec,
    game_backup_long_sec: config.game_backup_long_sec,
    extra_args: config.extra_args ?? [],
    bepinex_enabled: config.bepinex_enabled,
    backup_keep_last: config.backup_keep_last,
    backup_keep_days: config.backup_keep_days,
    backup_before_update: config.backup_before_update,
    remote_backup_target_id: config.remote_backup?.target_id ?? '',
    remote_backup_kinds: config.remote_backup?.kinds ?? DEFAULT_REMOTE_BACKUP_KINDS,
  }
}

function formToConfig(v: ConfigFormValues): InstanceConfig {
  const modifiers: Modifiers = {}
  ;(Object.keys(v.modifiers) as ModifierKey[]).forEach((key) => {
    const value = v.modifiers[key]
    if (value) (modifiers as Record<ModifierKey, string>)[key] = value
  })
  return {
    name: v.name,
    world: v.world,
    password: v.password,
    port: v.port,
    public: v.public,
    crossplay: v.crossplay,
    preset: (v.preset || '') as InstanceConfig['preset'],
    modifiers,
    setkeys: v.setkeys as InstanceConfig['setkeys'],
    save_interval_sec: v.save_interval_sec,
    game_backups: v.game_backups,
    game_backup_short_sec: v.game_backup_short_sec,
    game_backup_long_sec: v.game_backup_long_sec,
    extra_args: v.extra_args,
    bepinex_enabled: v.bepinex_enabled,
    backup_keep_last: v.backup_keep_last,
    backup_keep_days: v.backup_keep_days,
    backup_before_update: v.backup_before_update,
    remote_backup: v.remote_backup_target_id
      ? ({ target_id: v.remote_backup_target_id, kinds: v.remote_backup_kinds } satisfies RemoteBackupConfig)
      : undefined,
  }
}

export function useInstanceConfigForm({
  mode,
  initial,
  passwordMasked = false,
}: {
  mode: 'create' | 'edit'
  initial: InstanceConfigFormInitial
  passwordMasked?: boolean
}): InstanceConfigFormApi {
  const [passwordUnlocked, setPasswordUnlocked] = useState(!passwordMasked)

  const form = useForm<InstanceConfigFormValues>({
    initialValues: {
      id: initial.id ?? '',
      name: initial.name,
      config: configToForm(initial.config),
      autostart: initial.autostart,
      install: true,
    },
    validate: {
      id: (v) => (mode === 'create' && !INSTANCE_ID_PATTERN.test(v) ? 'Lowercase letters, digits and "-", starting with a letter or digit, up to 32 characters' : null),
      name: (v) => (v.trim().length > 0 && v.length <= 64 ? null : 'Required, up to 64 characters'),
      config: {
        name: (v) => (v.trim().length > 0 && v.length <= 64 ? null : 'Required, up to 64 characters'),
        world: (v) => (WORLD_NAME_PATTERN.test(v) ? null : 'Letters, digits, spaces, "_" or "-", 1-32 characters'),
        password: (v, values) => {
          if (passwordMasked && !passwordUnlocked) return null
          if (v.length < 5 || v.length > 32) return '5-32 characters'
          if (values.config.name && v && values.config.name.toLowerCase().includes(v.toLowerCase())) {
            return 'Must not be contained in the server name'
          }
          return null
        },
        port: (v) => (Number.isInteger(v) && v >= 1024 && v <= 65000 ? null : 'Between 1024 and 65000'),
      },
    },
  })

  function toSubmit(values: InstanceConfigFormValues): InstanceConfigFormSubmit {
    const password = passwordMasked && !passwordUnlocked ? initial.config.password : values.config.password
    const config = formToConfig({ ...values.config, password })
    return { id: values.id, name: values.name, config, autostart: values.autostart, install: values.install }
  }

  return {
    form,
    mode,
    passwordMasked,
    passwordUnlocked,
    unlockPassword: () => setPasswordUnlocked(true),
    toSubmit,
  }
}

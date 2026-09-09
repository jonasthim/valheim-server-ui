// Shared InstanceConfig form used by CreateInstancePage (POST /instances) and
// ConfigTab (PATCH /instances/{id}). Owns the @mantine/form instance so both
// callers get identical fields/validation; the caller only sees the resolved
// InstanceConfig on submit, never the form's internal (string-based) shape.
import { useState } from 'react'
import {
  Accordion,
  Button,
  Checkbox,
  Group,
  NumberInput,
  PasswordInput,
  Select,
  SimpleGrid,
  Stack,
  Switch,
  Text,
  TagsInput,
  TextInput,
} from '@mantine/core'
import { useForm } from '@mantine/form'
import type { InstanceConfig, Modifiers } from '../../api/types'
import { SectionCard } from '../../ui'
import { INSTANCE_ID_PATTERN, WORLD_NAME_PATTERN, slugify } from './instanceHelpers'
import classes from './InstanceConfigForm.module.css'

type ModifierKey = keyof Modifiers

interface ModifiersFormValues {
  combat: string
  deathpenalty: string
  resources: string
  raids: string
  portals: string
}

interface ConfigFormValues {
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
}

interface FormValues {
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

export interface InstanceConfigFormProps {
  mode: 'create' | 'edit'
  initial: InstanceConfigFormInitial
  /** True when config.password came back masked ("********") — keep it read-only until unlocked. */
  passwordMasked?: boolean
  /** Disable every field and hide the submit button (viewer looking at ConfigTab). */
  readOnly?: boolean
  submitting?: boolean
  submitLabel: string
  onSubmit: (values: InstanceConfigFormSubmit, helpers: FormHelpers) => void
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
  }
}

const PRESET_OPTIONS = [
  { value: '', label: 'None (use modifiers below)' },
  { value: 'normal', label: 'Normal' },
  { value: 'casual', label: 'Casual' },
  { value: 'easy', label: 'Easy' },
  { value: 'hard', label: 'Hard' },
  { value: 'hardcore', label: 'Hardcore' },
  { value: 'immersive', label: 'Immersive' },
  { value: 'hammer', label: 'Hammer' },
]

// Options in the game's own slider order with a human label; "" is the
// middle "Normal" step, which is not passed to the server (see selectData).
type ModifierOption = { value: string; label: string }
const NORMAL: ModifierOption = { value: '', label: 'Normal' }
const MODIFIER_FIELDS: { key: ModifierKey; label: string; options: ModifierOption[] }[] = [
  {
    key: 'combat',
    label: 'Combat',
    options: [
      { value: 'veryeasy', label: 'Very easy' },
      { value: 'easy', label: 'Easy' },
      NORMAL,
      { value: 'hard', label: 'Hard' },
      { value: 'veryhard', label: 'Very hard' },
    ],
  },
  {
    key: 'deathpenalty',
    label: 'Death penalty',
    options: [
      { value: 'casual', label: 'Casual' },
      { value: 'veryeasy', label: 'Very easy' },
      { value: 'easy', label: 'Easy' },
      NORMAL,
      { value: 'hard', label: 'Hard' },
      { value: 'hardcore', label: 'Hardcore' },
    ],
  },
  {
    key: 'resources',
    label: 'Resources',
    options: [
      { value: 'muchless', label: 'Much less' },
      { value: 'less', label: 'Less' },
      NORMAL,
      { value: 'more', label: 'More' },
      { value: 'muchmore', label: 'Much more' },
      { value: 'most', label: 'Most' },
    ],
  },
  {
    key: 'raids',
    label: 'Raids',
    options: [
      { value: 'none', label: 'None' },
      { value: 'muchless', label: 'Much less' },
      { value: 'less', label: 'Less' },
      NORMAL,
      { value: 'more', label: 'More' },
      { value: 'muchmore', label: 'Much more' },
    ],
  },
  {
    key: 'portals',
    label: 'Portals',
    options: [
      { value: 'casual', label: 'Casual' },
      NORMAL,
      { value: 'hard', label: 'Hard' },
      { value: 'veryhard', label: 'Very hard' },
    ],
  },
]

const SETKEY_OPTIONS: { value: string; label: string; description: string }[] = [
  { value: 'nobuildcost', label: 'No build cost', description: 'Building requires no materials' },
  { value: 'playerevents', label: 'Player-based raids', description: 'Raids follow each player\'s own progress' },
  { value: 'passivemobs', label: 'Passive mobs', description: 'Enemies do not attack until provoked' },
  { value: 'nomap', label: 'No map', description: 'Map and minimap are disabled' },
  { value: 'fire', label: 'Fire hazards', description: 'Wood can catch fire and spread outside the Ashlands' },
]

// The empty value means "-modifier is not passed": without a preset the game
// runs the rule at Normal; with a preset it runs the preset's value for that
// rule, so the middle step is labelled "From preset" instead of "Normal".
function selectData(options: ModifierOption[], hasPreset: boolean) {
  return options.map((o) => (o.value === '' && hasPreset ? { value: '', label: 'From preset' } : o))
}

function presetLabel(value: string) {
  return PRESET_OPTIONS.find((o) => o.value === value)?.label ?? value
}

export function InstanceConfigForm({
  mode,
  initial,
  passwordMasked = false,
  readOnly = false,
  submitting = false,
  submitLabel,
  onSubmit,
}: InstanceConfigFormProps) {
  const [idTouched, setIdTouched] = useState(mode === 'edit')
  const [passwordUnlocked, setPasswordUnlocked] = useState(!passwordMasked)

  const form = useForm<FormValues>({
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

  function handleNameChange(value: string) {
    form.setFieldValue('name', value)
    if (mode === 'create' && !idTouched) form.setFieldValue('id', slugify(value))
  }

  function handleSubmit(values: FormValues) {
    const password = passwordMasked && !passwordUnlocked ? initial.config.password : values.config.password
    const config = formToConfig({ ...values.config, password })
    onSubmit(
      { id: values.id, name: values.name, config, autostart: values.autostart, install: values.install },
      { setErrors: (errors) => form.setErrors(errors) },
    )
  }

  return (
    <form onSubmit={form.onSubmit(handleSubmit)}>
      <fieldset disabled={readOnly} style={{ border: 0, padding: 0, margin: 0 }}>
        <Stack gap="lg">
          <SectionCard title="Server" description="Identity, in-game name and network port.">
            <Stack gap="sm">
              <SimpleGrid cols={{ base: 1, sm: 2 }}>
                <TextInput
                  label="Display name"
                  description="Shown in this UI"
                  required
                  {...form.getInputProps('name')}
                  onChange={(e) => handleNameChange(e.currentTarget.value)}
                />
                <TextInput
                  label="Instance ID"
                  description={mode === 'create' ? 'Used in URLs and file paths; cannot be changed later' : 'Cannot be changed'}
                  required
                  disabled={mode === 'edit'}
                  {...form.getInputProps('id')}
                  onChange={(e) => {
                    setIdTouched(true)
                    form.setFieldValue('id', e.currentTarget.value)
                  }}
                />
              </SimpleGrid>
              <TextInput
                label="Server name"
                description="Shown to players in the server browser"
                required
                {...form.getInputProps('config.name')}
              />
              <SimpleGrid cols={{ base: 1, sm: 2 }}>
                <TextInput label="World name" required {...form.getInputProps('config.world')} />
                <NumberInput
                  label="Port"
                  description="Uses port, port+1 and port+2 UDP"
                  required
                  min={1024}
                  max={65000}
                  {...form.getInputProps('config.port')}
                />
              </SimpleGrid>
            </Stack>
          </SectionCard>

          <SectionCard title="Access & visibility" description="Who can find and join this server.">
            <Stack gap="sm">
              {passwordMasked && !passwordUnlocked ? (
                <Group align="flex-end" gap="sm">
                  <PasswordInput
                    label="Password"
                    description="5-32 characters, not contained in the server name"
                    value="********"
                    readOnly
                    style={{ flex: 1 }}
                  />
                  <Button variant="light" onClick={() => setPasswordUnlocked(true)}>
                    Change password
                  </Button>
                </Group>
              ) : (
                <PasswordInput
                  label="Password"
                  description="5-32 characters, not contained in the server name"
                  required
                  {...form.getInputProps('config.password')}
                />
              )}
              <Group>
                <Switch label="Public" description="List on the community server list" {...form.getInputProps('config.public', { type: 'checkbox' })} />
                <Switch label="Crossplay" description="Allow non-Steam platforms" {...form.getInputProps('config.crossplay', { type: 'checkbox' })} />
              </Group>
            </Stack>
          </SectionCard>

          <SectionCard title="World rules" description="Difficulty preset and per-rule overrides.">
            <Stack gap="sm">
              <Select label="Preset" data={PRESET_OPTIONS} {...form.getInputProps('config.preset')} />
              <Text size="xs" c="dimmed">
                {form.values.config.preset
                  ? `Rules left on "From preset" use the ${presetLabel(form.values.config.preset)} preset's values; pick a value to override just that rule.`
                  : 'No preset selected: every rule runs at Normal unless you pick another value.'}
              </Text>
              <SimpleGrid cols={{ base: 1, sm: 3 }}>
                {MODIFIER_FIELDS.map((f) => (
                  <Select
                    key={f.key}
                    label={f.label}
                    data={selectData(f.options, Boolean(form.values.config.preset))}
                    {...form.getInputProps(`config.modifiers.${f.key}`)}
                  />
                ))}
              </SimpleGrid>
              <Checkbox.Group
                label="World keys"
                value={form.values.config.setkeys}
                onChange={(v) => form.setFieldValue('config.setkeys', v)}
              >
                <SimpleGrid cols={{ base: 1, sm: 2 }} mt="xs">
                  {SETKEY_OPTIONS.map((o) => (
                    <Checkbox key={o.value} value={o.value} label={o.label} description={o.description} />
                  ))}
                </SimpleGrid>
              </Checkbox.Group>
            </Stack>
          </SectionCard>

          <SectionCard title="Saves & backups" description="Save cadence and how many copies are kept.">
            <Stack gap="sm">
              <SimpleGrid cols={{ base: 1, sm: 2 }}>
                <NumberInput label="Save interval (sec)" min={60} {...form.getInputProps('config.save_interval_sec')} />
                <NumberInput label="Game backups to keep (Valheim's own)" min={0} {...form.getInputProps('config.game_backups')} />
                <NumberInput label="Short backup interval (sec)" min={60} {...form.getInputProps('config.game_backup_short_sec')} />
                <NumberInput label="Long backup interval (sec)" min={60} {...form.getInputProps('config.game_backup_long_sec')} />
                <NumberInput label="Manager backups: keep last" min={0} {...form.getInputProps('config.backup_keep_last')} />
                <NumberInput label="Manager backups: keep days" min={0} {...form.getInputProps('config.backup_keep_days')} />
              </SimpleGrid>
              <Switch
                label="Back up before updating"
                {...form.getInputProps('config.backup_before_update', { type: 'checkbox' })}
              />
            </Stack>
          </SectionCard>

          <SectionCard flush>
            <Accordion variant="filled">
              <Accordion.Item value="advanced">
                <Accordion.Control>Advanced</Accordion.Control>
                <Accordion.Panel>
                  <Stack gap="sm">
                    <TagsInput
                      label="Extra launch arguments"
                      description="Passed verbatim after the generated arguments"
                      placeholder="Type and press Enter"
                      {...form.getInputProps('config.extra_args')}
                    />
                    <Switch
                      label="BepInEx enabled"
                      description="Only takes effect once BepInEx is installed from the Mods tab"
                      {...form.getInputProps('config.bepinex_enabled', { type: 'checkbox' })}
                    />
                    <Switch
                      label="Autostart"
                      description="Start this instance automatically when the manager starts"
                      {...form.getInputProps('autostart', { type: 'checkbox' })}
                    />
                    {mode === 'create' && (
                      <Checkbox
                        label="Download game files now"
                        description="Uncheck to create the instance without installing it yet"
                        {...form.getInputProps('install', { type: 'checkbox' })}
                      />
                    )}
                  </Stack>
                </Accordion.Panel>
              </Accordion.Item>
            </Accordion>
          </SectionCard>

          {!readOnly && (
            <Group justify="flex-end" className={classes.submitBar}>
              <Button type="submit" loading={submitting}>
                {submitLabel}
              </Button>
            </Group>
          )}
          {readOnly && (
            <Text c="dimmed" size="sm">
              You have read-only access to this configuration.
            </Text>
          )}
        </Stack>
      </fieldset>
    </form>
  )
}

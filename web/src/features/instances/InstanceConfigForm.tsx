// Shared InstanceConfig form used by CreateInstancePage (POST /instances) and
// ConfigTab (PATCH /instances/{id}). Renders the fields for the
// @mantine/form instance the page obtains from useInstanceConfigForm, so both
// callers get identical fields/validation; the caller only sees the resolved
// InstanceConfig on submit, never the form's internal (string-based) shape.
import { useState } from 'react'
import {
  Accordion,
  Button,
  Checkbox,
  Divider,
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
import type { BackupKind, Modifiers } from '../../api/types'
import { SectionCard } from '../../ui'
import { BACKUP_KIND_LABELS } from './backups/constants'
import { useBackupTargets } from './backups/useBackups'
import { slugify } from './instanceHelpers'
import type { FormHelpers, InstanceConfigFormApi, InstanceConfigFormSubmit } from './useInstanceConfigForm'
import classes from './InstanceConfigForm.module.css'

const REMOTE_BACKUP_KIND_OPTIONS: { value: BackupKind; label: string }[] = (
  Object.keys(BACKUP_KIND_LABELS) as BackupKind[]
).map((k) => ({ value: k, label: BACKUP_KIND_LABELS[k] }))

type ModifierKey = keyof Modifiers

export interface InstanceConfigFormProps {
  api: InstanceConfigFormApi
  formId: string
  /** Disable every field (viewer looking at ConfigTab); the save control lives outside this form. */
  readOnly?: boolean
  onSubmit: (values: InstanceConfigFormSubmit, helpers: FormHelpers) => void
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

// The empty value means "-modifier is not passed": without a preset (or with
// the Normal preset, which is the game's defaults) the rule runs at Normal;
// with any other preset it runs that preset's value for the rule, so the
// middle step is labelled "From preset" instead of "Normal". Valheim's command
// line has no explicit "normal" modifier value to override a preset with.
function presetOverridesRules(preset: string) {
  return preset !== '' && preset !== 'normal'
}

function selectData(options: ModifierOption[], hasPreset: boolean) {
  return options.map((o) => (o.value === '' && hasPreset ? { value: '', label: 'From preset' } : o))
}

function presetLabel(value: string) {
  return PRESET_OPTIONS.find((o) => o.value === value)?.label ?? value
}

export function InstanceConfigForm({ api, formId, readOnly = false, onSubmit }: InstanceConfigFormProps) {
  const [idTouched, setIdTouched] = useState(api.mode === 'edit')
  const targetsQ = useBackupTargets()
  const targetOptions = [
    { value: '', label: 'None' },
    ...(targetsQ.data ?? []).map((t) => ({ value: t.id, label: t.name })),
  ]

  function handleNameChange(value: string) {
    api.form.setFieldValue('name', value)
    if (api.mode === 'create' && !idTouched) api.form.setFieldValue('id', slugify(value))
  }

  return (
    <form
      id={formId}
      onSubmit={api.form.onSubmit((v) => onSubmit(api.toSubmit(v), { setErrors: (e) => api.form.setErrors(e) }))}
      className={classes.form}
    >
      <fieldset disabled={readOnly} style={{ border: 0, padding: 0, margin: 0 }}>
        <Stack gap="md">
          <SectionCard title="Server" description="Identity, in-game name and network port.">
            <Stack gap="sm">
              <SimpleGrid cols={{ base: 1, sm: 2 }} className={classes.inputRow}>
                <TextInput
                  label="Display name"
                  description="Shown in this UI"
                  required
                  {...api.form.getInputProps('name')}
                  onChange={(e) => handleNameChange(e.currentTarget.value)}
                />
                <TextInput
                  label="Instance ID"
                  description={api.mode === 'create' ? 'Used in URLs and file paths; cannot be changed later' : 'Cannot be changed'}
                  required
                  disabled={api.mode === 'edit'}
                  {...api.form.getInputProps('id')}
                  onChange={(e) => {
                    setIdTouched(true)
                    api.form.setFieldValue('id', e.currentTarget.value)
                  }}
                />
              </SimpleGrid>
              <TextInput
                label="Server name"
                description="Shown to players in the server browser"
                required
                {...api.form.getInputProps('config.name')}
              />
              <SimpleGrid cols={{ base: 1, sm: 2 }} className={classes.inputRow}>
                <TextInput label="World name" required {...api.form.getInputProps('config.world')} />
                <NumberInput
                  label="Port"
                  description="Uses port, port+1 and port+2 UDP"
                  required
                  min={1024}
                  max={65000}
                  {...api.form.getInputProps('config.port')}
                />
              </SimpleGrid>
            </Stack>
          </SectionCard>

          <SectionCard title="Access & visibility" description="Who can find and join this server.">
            <Stack gap="sm">
              {api.passwordMasked && !api.passwordUnlocked ? (
                <Group align="flex-end" gap="sm">
                  <PasswordInput
                    label="Password"
                    description="5-32 characters, not contained in the server name"
                    value="********"
                    readOnly
                    style={{ flex: 1 }}
                  />
                  <Button variant="light" onClick={api.unlockPassword}>
                    Change password
                  </Button>
                </Group>
              ) : (
                <PasswordInput
                  label="Password"
                  description="5-32 characters, not contained in the server name"
                  required
                  {...api.form.getInputProps('config.password')}
                />
              )}
              <Group>
                <Switch label="Public" description="List on the community server list" {...api.form.getInputProps('config.public', { type: 'checkbox' })} />
                <Switch label="Crossplay" description="Allow non-Steam platforms" {...api.form.getInputProps('config.crossplay', { type: 'checkbox' })} />
              </Group>
            </Stack>
          </SectionCard>

          <SectionCard title="World rules" description="Difficulty preset and per-rule overrides.">
            <Stack gap="sm">
              <Select label="Preset" data={PRESET_OPTIONS} {...api.form.getInputProps('config.preset')} />
              <Text size="xs" c="dimmed">
                {presetOverridesRules(api.form.values.config.preset)
                  ? `Rules left on "From preset" use the ${presetLabel(api.form.values.config.preset)} preset's values; pick a value to override just that rule.`
                  : 'Every rule runs at Normal unless you pick another value.'}
              </Text>
              <SimpleGrid cols={{ base: 1, sm: 3 }} className={classes.inputRow}>
                {MODIFIER_FIELDS.map((f) => (
                  <Select
                    key={f.key}
                    label={f.label}
                    data={selectData(f.options, presetOverridesRules(api.form.values.config.preset))}
                    {...api.form.getInputProps(`config.modifiers.${f.key}`)}
                  />
                ))}
              </SimpleGrid>
              <Checkbox.Group
                label="World keys"
                value={api.form.values.config.setkeys}
                onChange={(v) => api.form.setFieldValue('config.setkeys', v)}
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
              <SimpleGrid cols={{ base: 1, sm: 2 }} className={classes.inputRow}>
                <NumberInput label="Save interval (sec)" min={60} {...api.form.getInputProps('config.save_interval_sec')} />
                <NumberInput label="Game backups to keep (Valheim's own)" min={0} {...api.form.getInputProps('config.game_backups')} />
                <NumberInput label="Short backup interval (sec)" min={60} {...api.form.getInputProps('config.game_backup_short_sec')} />
                <NumberInput label="Long backup interval (sec)" min={60} {...api.form.getInputProps('config.game_backup_long_sec')} />
                <NumberInput label="Manager backups: keep last" min={0} {...api.form.getInputProps('config.backup_keep_last')} />
                <NumberInput label="Manager backups: keep days" min={0} {...api.form.getInputProps('config.backup_keep_days')} />
              </SimpleGrid>
              <Switch
                label="Back up before updating"
                {...api.form.getInputProps('config.backup_before_update', { type: 'checkbox' })}
              />

              <Divider label="Off-site copy" labelPosition="left" />
              <Select
                label="Off-site target"
                description="Copy backups here after they are created; configured by an admin in Settings"
                data={targetOptions}
                allowDeselect={false}
                {...api.form.getInputProps('config.remote_backup_target_id')}
              />
              {api.form.values.config.remote_backup_target_id && (
                <Checkbox.Group
                  label="Copy these backups"
                  value={api.form.values.config.remote_backup_kinds}
                  onChange={(v) => api.form.setFieldValue('config.remote_backup_kinds', v as BackupKind[])}
                >
                  <SimpleGrid cols={{ base: 1, sm: 3 }} mt="xs">
                    {REMOTE_BACKUP_KIND_OPTIONS.map((o) => (
                      <Checkbox key={o.value} value={o.value} label={o.label} />
                    ))}
                  </SimpleGrid>
                </Checkbox.Group>
              )}
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
                      {...api.form.getInputProps('config.extra_args')}
                    />
                    <Switch
                      label="BepInEx enabled"
                      description="Only takes effect once BepInEx is installed from the Mods tab"
                      {...api.form.getInputProps('config.bepinex_enabled', { type: 'checkbox' })}
                    />
                    {/* Create-only: an existing instance toggles autostart
                        immediately from the Overview tab instead. */}
                    {api.mode === 'create' && (
                      <Switch
                        label="Autostart"
                        description="Start this instance automatically when the manager starts"
                        {...api.form.getInputProps('autostart', { type: 'checkbox' })}
                      />
                    )}
                    {api.mode === 'create' && (
                      <Checkbox
                        label="Download game files now"
                        description="Uncheck to create the instance without installing it yet"
                        {...api.form.getInputProps('install', { type: 'checkbox' })}
                      />
                    )}
                  </Stack>
                </Accordion.Panel>
              </Accordion.Item>
            </Accordion>
          </SectionCard>
        </Stack>
      </fieldset>
    </form>
  )
}

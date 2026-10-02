// Vanilla raid list shown in the Raids table when the agent isn't connected
// (so the table still has rows to toggle before the server has ever run).
// The live catalogue (GET /instances/{id}/agent/catalog -> events, via
// useAgentCatalog) wins whenever the agent is connected; see
// mergeRaidNames in gameplayConfig.ts.
export interface VanillaRaid {
  name: string
  label: string
}

export const VANILLA_RAIDS: VanillaRaid[] = [
  { name: 'army_eikthyr', label: "Eikthyr's wrath" },
  { name: 'foresttrolls', label: 'The ground is shaking (trolls)' },
  { name: 'army_theelder', label: 'The forest is moving (greydwarfs)' },
  { name: 'skeletons', label: 'Skeleton surprise' },
  { name: 'blobs', label: 'You stirred the cauldron (oozers)' },
  { name: 'army_bonemass', label: 'A foul smell from the swamp' },
  { name: 'wolves', label: 'You are being hunted' },
  { name: 'army_moder', label: 'A cold wind blows from the mountains (drakes)' },
  { name: 'army_goblin', label: 'The horde is attacking (fulings)' },
  { name: 'surtlings', label: "There's a smell of sulfur" },
  { name: 'bats', label: 'The hungry are coming (bats)' },
  { name: 'army_seekers', label: 'They sought you out (seekers)' },
  { name: 'army_gjall', label: "What's that smell? (gjall)" },
  { name: 'army_charred', label: 'The Ashlands reach out (charred)' },
  { name: 'army_fader', label: "Fader's wrath" },
]

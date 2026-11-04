import { readFileSync } from 'node:fs';

export const BUILT_IN_SKILL_IDS = ['dont-kill-an-ant-with-a-bazooka', 'council-implementation-review', 'council-independent-perspectives', 'council-advisor'] as const;
export type CouncilSkillId = typeof BUILT_IN_SKILL_IDS[number];
const catalog = [
  { id: BUILT_IN_SKILL_IDS[0], name: "Don't kill an ant with a bazooka", description: 'Right-size models, effort and collaboration to the task while preserving verification and explicit model choices.' },
  { id: BUILT_IN_SKILL_IDS[1], name: 'Implementation and review', description: 'Separate implementation, independent review and revision for work that benefits from a second perspective.' },
  { id: BUILT_IN_SKILL_IDS[2], name: 'Independent perspectives', description: 'Keep parallel analyses independent and make disagreements and evidence explicit.' },
  { id: BUILT_IN_SKILL_IDS[3], name: 'Focused advisor', description: 'Use a focused read-only consultation when a specific uncertainty warrants another model.' },
] as const;
const cache = new Map<CouncilSkillId, string>();

export function collaborationSkills() { return catalog.map(skill => ({ ...skill })); }
export function validateSkills(value: unknown): CouncilSkillId[] {
  if (!Array.isArray(value) || Array.from(value).some(id => !BUILT_IN_SKILL_IDS.includes(id as CouncilSkillId)) || new Set(value).size !== value.length) {
    throw new Error('Skills must be unique IDs from the Council collaboration skill catalog.');
  }
  return [...value] as CouncilSkillId[];
}

/** Only bundled worker guidance is injected; host orchestration instructions stay outside worker prompts. */
export function skillInstructions(value: unknown): string {
  return validateSkills(value).map(id => {
    let instructions = cache.get(id);
    if (instructions === undefined) {
      const source = readFileSync(new URL(`../skills/${id}/SKILL.md`, import.meta.url), 'utf8');
      const start = '<!-- council-agent-instructions -->', end = '<!-- /council-agent-instructions -->';
      if (source.split(start).length !== 2 || source.split(end).length !== 2) throw new Error(`Bundled skill ${id} is missing its worker guidance.`);
      instructions = source.split(start)[1]!.split(end)[0]!.trim();
      if (!instructions) throw new Error(`Bundled skill ${id} has empty worker guidance.`);
      cache.set(id, instructions);
    }
    return `${catalog.find(skill => skill.id === id)!.name}\n${instructions}`;
  }).join('\n\n');
}

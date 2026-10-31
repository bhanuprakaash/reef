import type { SkillInfo, SkillSource } from '@/types/api';

const sourceLabels: Record<SkillSource, string> = {
  reef: 'Reef',
  claude: 'Claude',
  codex: 'Codex',
};

export function sourceLabel(source: SkillSource) {
  return sourceLabels[source];
}

export function addableSkills(available: SkillInfo[] | undefined, picked: string[]) {
  return (available ?? []).filter((s) => !picked.includes(s.ref));
}

export function isMissing(available: SkillInfo[] | undefined, ref: string) {
  return available !== undefined && !available.some((s) => s.ref === ref);
}

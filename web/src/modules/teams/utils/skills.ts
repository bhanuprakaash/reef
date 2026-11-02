import type { SkillInfo, SkillSource } from '@/types/api';

const sourceLabels: Record<SkillSource, string> = {
  reef: 'Reef',
  claude: 'Claude',
  codex: 'Codex',
};

export function sourceLabel(source: SkillSource) {
  return sourceLabels[source];
}

export function isMissing(available: SkillInfo[] | undefined, ref: string) {
  return available !== undefined && !available.some((s) => s.ref === ref);
}

export function incompatibility(skill: SkillInfo | undefined, backend: string | undefined) {
  return (backend && skill?.compat?.[backend]) || '';
}

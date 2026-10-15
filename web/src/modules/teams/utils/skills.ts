import type { SkillInfo } from '@/types/api';

export function addableSkills(available: SkillInfo[] | undefined, picked: string[]) {
  return (available ?? []).map((s) => s.name).filter((name) => !picked.includes(name));
}

export function isMissing(available: SkillInfo[] | undefined, name: string) {
  return available !== undefined && !available.some((s) => s.name === name);
}

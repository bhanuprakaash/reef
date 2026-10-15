import type { Team } from '@/types/api';

export function roleFaces(teams: Team[] | undefined, team: string | undefined) {
  return Object.fromEntries(
    (teams?.find((t) => t.name === team)?.roles ?? []).map((r) => [r.name, r.face]),
  );
}

import type { Team } from '@/types/api';

export function teamRequestBody(team: Omit<Team, 'id'>) {
  return {
    ...team,
    roles: team.roles.map((role) => ({ ...role, mcp: undefined })),
  };
}

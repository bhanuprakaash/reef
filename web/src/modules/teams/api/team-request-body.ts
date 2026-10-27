import type { Team } from '@/types/api';

export function teamRequestBody(team: Team) {
  return {
    ...team,
    roles: team.roles.map((role) => ({ ...role, mcp: undefined })),
  };
}

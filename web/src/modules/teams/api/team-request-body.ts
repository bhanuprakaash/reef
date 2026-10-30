import type { Team } from '@/types/api';

export function teamRequestBody({ name, responsibility, roles }: Omit<Team, 'id'>) {
  return {
    name,
    responsibility,
    roles: roles.map(({ botId, lead }) => ({ botId, lead })),
  };
}

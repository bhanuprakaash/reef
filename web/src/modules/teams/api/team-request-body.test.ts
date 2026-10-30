import { expect, it } from 'vitest';
import { teamRequestBody } from './team-request-body';
import type { Role } from '@/types/api';

it('sends a member as its Bot id and lead flag only', () => {
  const role: Role = {
    botId: 7,
    name: 'sam',
    lead: true,
    write: false,
    backend: 'claude',
    permissionMode: '',
    description: 'Reads.',
    skills: [],
    tools: ['Read'],
    mcp: [],
  };

  expect(teamRequestBody({ name: 'crew', responsibility: 'Ship it.', roles: [role] })).toEqual({
    name: 'crew',
    responsibility: 'Ship it.',
    roles: [{ botId: 7, lead: true }],
  });
});

import { expect, it } from 'vitest';
import { teammateRequestBody } from './teammate-request-body';

it('sends only the fields the server knows for a Teammate', () => {
  const body = teammateRequestBody({
    teammateId: 4,
    name: 'sam',
    lead: true,
    write: false,
    backend: 'claude',
    permissionMode: '',
    persona: 'Reads.',
    skills: [],
    tools: ['Read'],
    mcp: [{ name: 'mcp1', label: 'true' }],
  });

  expect(Object.keys(body).sort()).toEqual([
    'backend',
    'face',
    'model',
    'name',
    'permissionMode',
    'persona',
    'provider',
    'skills',
    'tools',
    'write',
  ]);
});

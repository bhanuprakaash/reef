import { expect, it } from 'vitest';
import { botRequestBody } from './bot-request-body';

it('sends only the fields the server knows for a Bot', () => {
  const body = botRequestBody({
    name: 'sam',
    lead: true,
    write: false,
    backend: 'claude',
    permissionMode: '',
    description: 'Reads.',
    skills: [],
    tools: ['Read'],
    connections: [3],
  });

  expect(Object.keys(body).sort()).toEqual([
    'backend',
    'colour',
    'connections',
    'description',
    'face',
    'model',
    'name',
    'permissionMode',
    'provider',
    'skills',
    'tools',
    'write',
  ]);
});

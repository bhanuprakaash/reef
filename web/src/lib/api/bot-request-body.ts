import type { Role } from '@/types/api';

export function botRequestBody(role: Omit<Role, 'botId'>) {
  const { name, description, skills, backend, provider, model, face, colour } = role;
  const { tools, connections, write, permissionMode } = role;
  return {
    name,
    description,
    skills,
    backend,
    provider,
    model,
    face,
    colour,
    tools,
    connections,
    write,
    permissionMode,
  };
}

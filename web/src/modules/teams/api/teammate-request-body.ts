import type { Role } from '@/types/api';

export function teammateRequestBody(role: Role) {
  const { name, persona, skills, backend, provider, model, face, colour } = role;
  const { tools, write, permissionMode } = role;
  return {
    name,
    persona,
    skills,
    backend,
    provider,
    model,
    face,
    colour,
    tools,
    write,
    permissionMode,
  };
}

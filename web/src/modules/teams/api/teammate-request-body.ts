import type { Role } from '@/types/api';

export function teammateRequestBody(role: Role) {
  const { name, persona, skills, backend, provider, model, face, tools, write, permissionMode } =
    role;
  return { name, persona, skills, backend, provider, model, face, tools, write, permissionMode };
}

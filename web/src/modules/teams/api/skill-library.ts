import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import { skillLibraryKey, skillsRootKey } from '@/lib/query-keys';
import type { LibrarySkill, LibrarySkillFiles } from '@/types/api';

export type SkillImport = { path: string } | { repo: string; ref: string; dir: string };

export function useSkillLibrary() {
  return useQuery({
    queryKey: skillLibraryKey,
    queryFn: () => api<LibrarySkill[]>('/skills/library'),
  });
}

export function useSkillFiles(id: number, version: number, enabled: boolean) {
  return useQuery({
    queryKey: [...skillLibraryKey, id, version],
    queryFn: () => api<LibrarySkillFiles>(`/skills/${id}`),
    enabled,
  });
}

function useRefreshLibrary() {
  const queryClient = useQueryClient();
  return () => {
    void queryClient.invalidateQueries({ queryKey: skillLibraryKey });
    void queryClient.invalidateQueries({ queryKey: skillsRootKey });
  };
}

export function useImportSkill() {
  const refresh = useRefreshLibrary();
  return useMutation({
    mutationFn: (input: SkillImport) =>
      api<LibrarySkill>('/skills/import', { method: 'POST', body: JSON.stringify(input) }),
    onSuccess: refresh,
  });
}

export function useUpdateSkill() {
  const refresh = useRefreshLibrary();
  return useMutation({
    mutationFn: (id: number) => api<LibrarySkill>(`/skills/${id}/update`, { method: 'POST' }),
    onSuccess: refresh,
  });
}

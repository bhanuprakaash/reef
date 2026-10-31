import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import { skillDraftsKey, skillLibraryKey, skillsRootKey } from '@/lib/query-keys';
import type { LibrarySkill, LibrarySkillFiles, SkillDraft, SkillFile } from '@/types/api';

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

export function useCreateSkill() {
  const refresh = useRefreshLibrary();
  return useMutation({
    mutationFn: (input: { name: string; files: SkillFile[] }) =>
      api<LibrarySkill>('/skills', { method: 'POST', body: JSON.stringify(input) }),
    onSuccess: refresh,
  });
}

export function useEditSkill(id: number) {
  const refresh = useRefreshLibrary();
  return useMutation({
    mutationFn: (files: SkillFile[]) =>
      api<LibrarySkill>(`/skills/${id}`, { method: 'PUT', body: JSON.stringify({ files }) }),
    onSuccess: refresh,
  });
}

export function useSkillDrafts(spaceId?: number) {
  return useQuery({
    queryKey: [...skillDraftsKey, spaceId ?? null],
    queryFn: () => api<SkillDraft[]>(spaceId ? `/skill-drafts?space=${spaceId}` : '/skill-drafts'),
  });
}

export function useSaveSkillDraft() {
  const refresh = useRefreshLibrary();
  return useMutation({
    mutationFn: ({ id, update }: { id: number; update: boolean }) =>
      api<LibrarySkill>(`/skill-drafts/${id}/save`, {
        method: 'POST',
        body: JSON.stringify({ update }),
      }),
    onSuccess: refresh,
  });
}

export function useRejectSkillDraft() {
  const refresh = useRefreshLibrary();
  return useMutation({
    mutationFn: (id: number) =>
      api<void>(`/skill-drafts/${id}/reject`, { method: 'POST', body: JSON.stringify({}) }),
    onSuccess: refresh,
  });
}

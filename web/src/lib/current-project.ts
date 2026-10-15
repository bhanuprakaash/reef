import { useEffect, useState } from 'react';
import type { Project } from '@/types/api';

export const PROJECT_KEY = 'reef.sidebar.projectId';

const listeners = new Set<(id: string | null) => void>();

export function useCurrentProject(projects: Project[] | undefined) {
  const [stored, setStored] = useState(() => localStorage.getItem(PROJECT_KEY));
  useEffect(() => {
    listeners.add(setStored);
    return () => void listeners.delete(setStored);
  }, []);

  const current = projects?.find((p) => String(p.id) === stored) ?? projects?.[0];

  function pick(project: Project) {
    localStorage.setItem(PROJECT_KEY, String(project.id));
    listeners.forEach((set) => set(String(project.id)));
  }
  return [current, pick] as const;
}

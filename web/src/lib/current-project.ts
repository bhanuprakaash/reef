import { useEffect, useState } from 'react';
import type { Project } from '@/types/api';

export const PROJECT_KEY = 'reef.sidebar.projectId';
const ALL_PROJECTS = 'all';

const listeners = new Set<(id: string | null) => void>();

export function useCurrentProject(projects: Project[] | undefined) {
  const [stored, setStored] = useState(() => localStorage.getItem(PROJECT_KEY));
  useEffect(() => {
    listeners.add(setStored);
    return () => void listeners.delete(setStored);
  }, []);

  const current =
    stored === ALL_PROJECTS
      ? undefined
      : (projects?.find((p) => String(p.id) === stored) ?? projects?.[0]);

  function pick(project: Project | null) {
    const value = project ? String(project.id) : ALL_PROJECTS;
    localStorage.setItem(PROJECT_KEY, value);
    listeners.forEach((set) => set(value));
  }
  return [current, pick] as const;
}

import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { Team } from '@/types/api';

type Add = { team: string; role: string; pattern: string; stopChannel?: number };

export function useAddRoleTool() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ team, role, pattern, stopChannel }: Add) => {
      const path = `/teams/${encodeURIComponent(team)}`;
      const current = await api<Team>(path);
      await api<Team>(path, {
        method: 'PUT',
        body: JSON.stringify({
          ...current,
          roles: current.roles.map((r) => ({
            ...r,
            mcp: undefined,
            tools: r.name === role && !r.tools.includes(pattern) ? [...r.tools, pattern] : r.tools,
          })),
        }),
      });
      if (stopChannel !== undefined) {
        await api<void>(`/channels/${stopChannel}/stop`, { method: 'POST', body: '{}' });
      }
    },
    onSettled: () => queryClient.invalidateQueries({ queryKey: ['teams'] }),
  });
}

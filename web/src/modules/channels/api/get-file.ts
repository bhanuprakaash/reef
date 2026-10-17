import { useQuery } from '@tanstack/react-query';
import { api, ApiError } from '@/lib/api-client';
import { fileKey } from '@/lib/query-keys';
import type { ChannelFile } from '@/types/api';

export function useFile(channelId: number, path: string) {
  return useQuery({
    queryKey: fileKey(channelId, path),
    queryFn: () => api<ChannelFile>(`/channels/${channelId}/file?path=${encodeURIComponent(path)}`),
    retry: (count, error) =>
      !(error instanceof ApiError && (error.status === 404 || error.status === 415)) && count < 3,
  });
}

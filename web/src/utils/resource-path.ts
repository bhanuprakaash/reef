import type { FindResult } from '@/types/api';

export const kindLabel: Record<FindResult['kind'], string> = {
  bot: 'Bot',
  chat: 'Chat',
  space: 'Space',
  workItem: 'Work item',
};

export const resourcePath = ({ kind, id }: Pick<FindResult, 'kind' | 'id'>) =>
  kind === 'bot' ? `/bots/${id}/chat` : kind === 'workItem' ? `/work-items/${id}` : `/spaces/${id}`;

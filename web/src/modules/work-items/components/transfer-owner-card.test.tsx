import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it, vi } from 'vitest';
import type { WorkItem } from '@/types/api';
import { TransferOwnerCard } from './transfer-owner-card';

vi.mock('@/lib/api/get-bots', () => ({
  useBots: () => ({
    data: [
      { id: 1, name: 'ann' },
      { id: 2, name: 'bea' },
      { id: 3, name: 'cal' },
    ],
  }),
}));

function render(item: Partial<WorkItem>) {
  return renderToStaticMarkup(
    <QueryClientProvider client={new QueryClient()}>
      <TransferOwnerCard
        item={
          {
            id: 1,
            ownerBotId: 1,
            pendingOwnerBotId: null,
            participantBotIds: [1, 2],
            ...item,
          } as WorkItem
        }
      />
    </QueryClientProvider>,
  );
}

describe('TransferOwnerCard', () => {
  it('names the owner and starts with no transfer to apply', () => {
    const html = render({});

    expect(html).toContain('ann owns this Work item.');
    expect(html).toContain('>Transfer ownership<');
    expect(html).not.toContain('applies after');
  });

  it('says a transfer waits for the turn and how to apply it now', () => {
    expect(render({ pendingOwnerBotId: 3 })).toContain(
      'Transfer to cal applies after the current turn. Stop the turn to apply it now.',
    );
  });
});

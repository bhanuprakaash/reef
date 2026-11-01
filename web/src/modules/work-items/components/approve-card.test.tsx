import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it, vi } from 'vitest';
import type { WorkItem } from '@/types/api';
import { ApproveCard } from './approve-card';

vi.mock('@/lib/api/get-bots', () => ({
  useBots: () => ({ data: [{ id: 1, name: 'ann' }] }),
}));

function render(item: Partial<WorkItem>) {
  return renderToStaticMarkup(
    <QueryClientProvider client={new QueryClient()}>
      <ApproveCard
        item={
          {
            id: 1,
            projectId: 1,
            ownerBotId: 1,
            participantBotIds: [1],
            proposedBy: 'ann',
            targets: [
              { projectId: 1, projectName: 'app', proposedBy: 'ann', authorizedBy: '' },
              { projectId: 2, projectName: 'infra', proposedBy: 'ann', authorizedBy: '' },
            ],
            ...item,
          } as WorkItem
        }
      />
    </QueryClientProvider>,
  );
}

describe('ApproveCard', () => {
  it('lists each proposed repository with a box, the home one fixed', () => {
    const html = render({});

    expect(html).toContain('aria-label="Repository app"');
    expect(html).toContain('aria-label="Repository infra"');
    expect(html.match(/proposed by ann<\/span>/g)).toHaveLength(2);
    expect(html.match(/aria-label="Repository [a-z]+"[^>]*disabled/g)).toHaveLength(1);
  });
});

import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it, vi } from 'vitest';
import type { Inspector } from '@/types/api';
import { InspectorPanel } from './inspector-panel';

const { useInspector } = vi.hoisted(() => ({ useInspector: vi.fn() }));

vi.mock('@/modules/channels/api/get-inspector', () => ({ useInspector }));

const usage = { inputTokens: 1, outputTokens: 1, cacheReadTokens: 0, cacheWriteTokens: 0 };

const data: Inspector = {
  roles: [{ role: 'lead', turns: 1, usage, costUsd: 0.0034 }],
  turns: [],
};

describe('InspectorPanel', () => {
  it('shows cost to 4 decimals instead of rounding to $0.00', () => {
    useInspector.mockReturnValue({ isPending: false, isError: false, data });

    const markup = renderToStaticMarkup(<InspectorPanel channelId={1} onClose={() => {}} />);

    expect(markup).toContain('$0.0034');
  });

  it('shows — when cost is null', () => {
    useInspector.mockReturnValue({
      isPending: false,
      isError: false,
      data: { roles: [{ role: 'lead', turns: 2, usage, costUsd: null }], turns: [] },
    });

    const markup = renderToStaticMarkup(<InspectorPanel channelId={1} onClose={() => {}} />);

    expect(markup).toContain('—');
  });

  it('shows "1 turn" for a single turn, not "1 turns"', () => {
    useInspector.mockReturnValue({ isPending: false, isError: false, data });

    const markup = renderToStaticMarkup(<InspectorPanel channelId={1} onClose={() => {}} />);

    expect(markup).toContain('1 turn ');
    expect(markup).not.toContain('1 turns');
  });

  it('shows "2 turns" for more than one turn', () => {
    useInspector.mockReturnValue({
      isPending: false,
      isError: false,
      data: { roles: [{ role: 'lead', turns: 2, usage, costUsd: 0.0034 }], turns: [] },
    });

    const markup = renderToStaticMarkup(<InspectorPanel channelId={1} onClose={() => {}} />);

    expect(markup).toContain('2 turns');
  });
});

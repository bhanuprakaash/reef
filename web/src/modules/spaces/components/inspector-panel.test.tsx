import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it, vi } from 'vitest';
import type { Inspector } from '@/types/api';
import { InspectorPanel } from './inspector-panel';

const { useInspector } = vi.hoisted(() => ({ useInspector: vi.fn() }));

vi.mock('@/modules/spaces/api/get-inspector', () => ({ useInspector }));

const usage = { inputTokens: 1, outputTokens: 1, cacheReadTokens: 0, cacheWriteTokens: 0 };

const data: Inspector = {
  roles: [{ role: 'lead', botId: null, turns: 1, usage, costUsd: 0.0034 }],
  turns: [],
};

describe('InspectorPanel', () => {
  it('shows cost to 4 decimals instead of rounding to $0.00', () => {
    useInspector.mockReturnValue({ isPending: false, isError: false, data });

    const markup = renderToStaticMarkup(
      <InspectorPanel spaceId={1} faces={{}} onClose={() => {}} />,
    );

    expect(markup).toContain('$0.0034');
  });

  it('names a conversation in the header', () => {
    useInspector.mockReturnValue({ isPending: false, isError: false, data });

    const markup = renderToStaticMarkup(
      <InspectorPanel spaceId={1} conversation faces={{}} onClose={() => {}} />,
    );

    expect(markup).toContain('this conversation');
    expect(markup).not.toContain('this Space');
  });

  it('shows n/a when cost is null', () => {
    useInspector.mockReturnValue({
      isPending: false,
      isError: false,
      data: { roles: [{ role: 'lead', botId: null, turns: 2, usage, costUsd: null }], turns: [] },
    });

    const markup = renderToStaticMarkup(
      <InspectorPanel spaceId={1} faces={{}} onClose={() => {}} />,
    );

    expect(markup).toContain('n/a');
    expect(markup).toContain('Codex does not report cost');
  });

  it('sums known costs and marks the total when a role has none', () => {
    useInspector.mockReturnValue({
      isPending: false,
      isError: false,
      data: {
        roles: [
          {
            role: 'lead',
            botId: null,
            turns: 4,
            usage: { ...usage, inputTokens: 1500, outputTokens: 500 },
            costUsd: 0.25,
          },
          {
            role: 'developer',
            botId: null,
            turns: 8,
            usage: { ...usage, inputTokens: 2000, outputTokens: 0 },
            costUsd: 0.5,
          },
          { role: 'tester', botId: null, turns: 2, usage, costUsd: null },
        ],
        turns: [],
      },
    });

    const markup = renderToStaticMarkup(
      <InspectorPanel spaceId={1} faces={{}} onClose={() => {}} />,
    );

    expect(markup).toContain('>$0.7500 + n/a<');
    expect(markup).toContain('4.0k');
    expect(markup).toContain('>14<');
  });

  it('shows "1 turn" for a single turn, not "1 turns"', () => {
    useInspector.mockReturnValue({ isPending: false, isError: false, data });

    const markup = renderToStaticMarkup(
      <InspectorPanel spaceId={1} faces={{}} onClose={() => {}} />,
    );

    expect(markup).toContain('1 turn<');
    expect(markup).not.toContain('1 turns');
  });

  it('shows "2 turns" for more than one turn', () => {
    useInspector.mockReturnValue({
      isPending: false,
      isError: false,
      data: { roles: [{ role: 'lead', botId: null, turns: 2, usage, costUsd: 0.0034 }], turns: [] },
    });

    const markup = renderToStaticMarkup(
      <InspectorPanel spaceId={1} faces={{}} onClose={() => {}} />,
    );

    expect(markup).toContain('2 turns');
  });

  it('shows cache tokens in the totals and the turn row, and the session instructions above the prompt', () => {
    const cached = {
      inputTokens: 1500,
      outputTokens: 600,
      cacheReadTokens: 322000,
      cacheWriteTokens: 49000,
    };
    useInspector.mockReturnValue({
      isPending: false,
      isError: false,
      data: {
        roles: [{ role: 'architect', botId: null, turns: 1, usage: cached, costUsd: 0.2493 }],
        turns: [
          {
            id: 3,
            role: 'architect',
            botId: null,
            startedAt: '2026-10-20T10:00:00Z',
            status: 'done',
            model: 'claude-opus-5-5',
            usage: cached,
            costUsd: 0.2493,
            instructions: 'You are a Bot working in Reef.',
            prompt: '## You\nYou are architect.',
            toolCalls: [],
          },
        ],
      },
    });

    const markup = renderToStaticMarkup(
      <InspectorPanel spaceId={1} faces={{}} onClose={() => {}} />,
    );

    expect(markup).toContain('>322.0k / 49.0k<');
    expect(markup).toContain('2.1k · cache 322.0k / 49.0k · $0.2493');
    expect(markup).toContain('Instructions</div>');
    expect(markup.indexOf('You are a Bot working in Reef.')).toBeLessThan(markup.indexOf('## You'));
  });

  it('shows a subagent call by its type with its nested calls in a closed group', () => {
    useInspector.mockReturnValue({
      isPending: false,
      isError: false,
      data: {
        roles: [{ role: 'developer', botId: null, turns: 1, usage, costUsd: 0.1 }],
        turns: [
          {
            id: 7,
            role: 'developer',
            botId: null,
            startedAt: '2026-10-20T10:00:00Z',
            status: 'done',
            model: 'claude-opus-5-5',
            usage,
            costUsd: 0.1,
            prompt: 'do it',
            toolCalls: [
              { name: 'Bash', input: '{"command":"ls"}', result: 'a.txt' },
              {
                name: 'Agent',
                input: '{"description":"Comment pass","subagent_type":"comment-sicko"}',
                result: 'nothing removed',
                children: [{ name: 'Read', input: '{"file_path":"a.go"}', result: 'package a' }],
              },
            ],
          },
        ],
      },
    });

    const markup = renderToStaticMarkup(
      <InspectorPanel spaceId={1} faces={{}} onClose={() => {}} />,
    );

    expect(markup).toContain('Agent comment-sicko · Comment pass');
    expect(markup).toContain('sub-agent · 1 tool<');
    expect(markup).toContain('<details class="mt-1.5"><summary class="cursor-pointer">sub-agent');
    expect(markup).toContain('<div class="text-soft">Bash</div>');
  });
});

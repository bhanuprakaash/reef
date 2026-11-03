import { isValidElement, type ReactNode } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { PromptCards } from './prompt-cards';

const { usePrompts, useAnswerPrompt } = vi.hoisted(() => ({
  usePrompts: vi.fn(),
  useAnswerPrompt: vi.fn(),
}));

const prompt = {
  id: 7,
  spaceId: 4,
  botId: 2,
  turnId: 31,
  tool: 'Bash',
  input: { command: 'gzip -l x.zip' },
  rule: 'Bash(gzip -l x.zip)',
  path: '/Users/a/x.zip',
  status: 'waiting',
  reason: '',
  createdAt: '2026-11-02T13:52:31Z',
};

const roles = [{ botId: 2, name: 'developer' }] as Parameters<typeof PromptCards>[0]['roles'];

vi.mock('@/modules/spaces/api/get-prompts', () => ({ usePrompts }));
vi.mock('@/modules/spaces/api/answer-prompt', () => ({ useAnswerPrompt }));

const render = () => renderToStaticMarkup(<PromptCards spaceId={4} roles={roles} />);

describe('PromptCards', () => {
  const mutate = vi.fn();

  beforeEach(() => {
    mutate.mockReset();
    useAnswerPrompt.mockReturnValue({ isPending: false, isError: false, mutate });
  });

  it('shows a waiting prompt with its command and three answers', () => {
    usePrompts.mockReturnValue({ data: [prompt] });

    const markup = render();

    expect(markup).toContain('developer');
    expect(markup).toContain(' wants to <span class="font-medium">Bash</span>');
    expect(markup).toContain('gzip -l x.zip</code>');
    expect(markup).toContain('Allow once');
    expect(markup).toContain('Allow always');
    expect(markup).toContain('Deny');
    expect(markup).not.toContain('Denied:');
  });

  it('posts the answer each button stands for', () => {
    usePrompts.mockReturnValue({ data: [prompt] });

    const clicks: Record<string, () => void> = {};
    const collect = (node: ReactNode) => {
      if (Array.isArray(node)) node.forEach(collect);
      if (!isValidElement<{ children?: ReactNode; onClick?: () => void }>(node)) return;
      if (node.props.onClick) clicks[String(node.props.children)] = node.props.onClick;
      collect(node.props.children);
    };
    collect(PromptCards({ spaceId: 4, roles }));
    for (const label of ['Allow once', 'Allow always', 'Deny']) clicks[label]();

    expect(mutate.mock.calls).toEqual([
      [{ id: 7, answer: 'once' }],
      [{ id: 7, answer: 'always' }],
      [{ id: 7, answer: 'deny' }],
    ]);
  });

  it('keeps Allow once and Allow always, but no Deny, on an expired prompt', () => {
    usePrompts.mockReturnValue({
      data: [{ ...prompt, status: 'expired', reason: 'no answer from the user' }],
    });

    const markup = render();

    expect(markup).toContain('Denied: no answer from the user');
    expect(markup).toContain('Allow once');
    expect(markup).toContain('Allow always');
    expect(markup).not.toContain('Deny');
  });

  it('shows nothing without prompts', () => {
    usePrompts.mockReturnValue({ data: [] });

    expect(render()).toBe('');
  });
});

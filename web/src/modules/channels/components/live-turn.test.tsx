import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { LiveTurn } from './live-turn';

const bash = {
  id: 't1',
  name: 'Bash',
  input: { command: 'npm test' },
  done: false,
  output: '',
  isError: false,
  children: [],
};

describe('LiveTurn', () => {
  it('shows one compact row with the latest tool, not the stream', () => {
    const turn = {
      status: 'running' as const,
      items: [
        { type: 'text' as const, text: 'Once upon' },
        { type: 'tool' as const, tool: bash },
      ],
    };

    const markup = renderToStaticMarkup(<LiveTurn turn={turn} role="lead" />);

    expect(markup).toContain('is working');
    expect(markup).toContain('Bash npm test');
    expect(markup).toContain('1 tool');
    expect(markup).not.toContain('Once upon');
  });

  it('says writing while text streams', () => {
    const turn = { status: 'running' as const, items: [{ type: 'text' as const, text: 'Hi' }] };

    expect(renderToStaticMarkup(<LiveTurn turn={turn} role="lead" />)).toContain('writing…');
  });

  it('shows a failed turn by its error only: the stored message has the text', () => {
    const turn = {
      status: 'failed' as const,
      error: 'interrupted',
      items: [{ type: 'text' as const, text: 'Once upon' }],
    };

    const markup = renderToStaticMarkup(<LiveTurn turn={turn} role="lead" />);

    expect(markup).toContain('Failed: interrupted');
    expect(markup).not.toContain('Once upon');
  });
});

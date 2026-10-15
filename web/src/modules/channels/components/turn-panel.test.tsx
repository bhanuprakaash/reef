import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { TurnPanel } from './turn-panel';

const bash = {
  id: 't1',
  name: 'Bash',
  input: { command: 'npm test' },
  done: true,
  output: 'ok',
  isError: false,
  children: [],
};

describe('TurnPanel', () => {
  it('shows the text and every tool row of the running turn', () => {
    const turn = {
      status: 'running' as const,
      items: [
        { type: 'text' as const, text: 'Once upon' },
        { type: 'tool' as const, tool: bash },
      ],
    };

    const markup = renderToStaticMarkup(<TurnPanel turn={turn} role="lead" onClose={() => {}} />);

    expect(markup).toContain('Once upon');
    expect(markup).toContain('npm test');
  });

  it('hides the text of a failed turn: the stored message has it', () => {
    const turn = {
      status: 'failed' as const,
      error: 'interrupted',
      items: [{ type: 'text' as const, text: 'Once upon' }],
    };

    const markup = renderToStaticMarkup(<TurnPanel turn={turn} role="lead" onClose={() => {}} />);

    expect(markup).toContain('Failed: interrupted');
    expect(markup).not.toContain('Once upon');
  });
});

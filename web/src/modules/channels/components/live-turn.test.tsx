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
  it('shows the last three tools and the streaming reply inline', () => {
    const tools = ['Read', 'Grep', 'Bash', 'Edit'].map((name, i) => ({
      ...bash,
      id: `t${i}`,
      name,
      input: { command: `cmd-${name}` },
    }));
    const turn = {
      status: 'running' as const,
      items: [
        ...tools.map((tool) => ({ type: 'tool' as const, tool })),
        { type: 'text' as const, text: 'Once upon' },
      ],
    };

    const markup = renderToStaticMarkup(<LiveTurn turn={turn} role="lead" />);

    expect(markup).toContain('is working');
    expect(markup).toContain('4 tools');
    expect(markup).not.toContain('Read');
    expect(markup).toContain('Grep');
    expect(markup).toContain('Bash');
    expect(markup).toContain('Edit');
    expect(markup).toContain('Once upon');
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

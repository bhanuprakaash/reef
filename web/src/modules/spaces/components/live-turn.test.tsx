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
  it('names the running tool and streams the reply inline', () => {
    const tools = ['Read', 'Grep', 'Bash', 'Edit'].map((name, i) => ({
      ...bash,
      id: `t${i}`,
      name,
      input: { command: `cmd-${name}` },
      done: name !== 'Edit',
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
    expect(markup).toContain('Editing');
    expect(markup).toContain('4 tools');
    expect(markup).not.toContain('Read');
    expect(markup).toContain('Grep');
    expect(markup).toContain('Bash');
    expect(markup).toContain('Edit');
    expect(markup).toContain('Once upon');
  });

  it('names no tool while the agent only thinks', () => {
    const turn = {
      status: 'running' as const,
      items: [{ type: 'text' as const, text: 'Hey. What do you want to work on?' }],
    };

    const markup = renderToStaticMarkup(<LiveTurn turn={turn} role="lead" />);

    expect(markup).toContain('is working');
    expect(markup).toContain('0 tools');
  });

  it('floats one bubble per tool call', () => {
    const tools = ['a', 'b'].map((id) => ({ ...bash, id }));
    const turn = {
      status: 'running' as const,
      items: tools.map((tool) => ({ type: 'tool' as const, tool })),
    };

    const markup = renderToStaticMarkup(<LiveTurn turn={turn} role="lead" />);

    expect(markup.match(/class="bubble/g)).toHaveLength(2);
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

  it('shows only that a direct turn is working, with no reply text or tool rows', () => {
    const turn = {
      status: 'running' as const,
      directMessageId: 1,
      items: [
        { type: 'tool' as const, tool: { ...bash, input: { command: 'send secret' } } },
        { type: 'text' as const, text: 'Secret reply' },
      ],
    };

    const markup = renderToStaticMarkup(<LiveTurn turn={turn} role="tester" />);

    expect(markup).toContain('is working');
    expect(markup).not.toContain('Secret reply');
    expect(markup).not.toContain('send secret');
  });
});

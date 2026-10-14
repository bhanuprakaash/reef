import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { LiveTurn } from './live-turn';

describe('LiveTurn', () => {
  it('shows the text of a running turn', () => {
    const turn = {
      status: 'running' as const,
      items: [{ type: 'text' as const, text: 'Once upon' }],
    };

    expect(renderToStaticMarkup(<LiveTurn turn={turn} role="lead" />)).toContain('Once upon');
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

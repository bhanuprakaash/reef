import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { HistoryInput } from './history-input';

describe('history input', () => {
  it('replaces the composer with the reason and paused state in deleted history', () => {
    const html = renderToStaticMarkup(
      <HistoryInput readOnly reason="assignment paused">
        <textarea aria-label="Message" />
      </HistoryInput>,
    );
    expect(html).toContain('read-only');
    expect(html).toContain('assignment paused');
    expect(html).not.toContain('textarea');
  });
  it('keeps the composer for active history', () => {
    expect(
      renderToStaticMarkup(
        <HistoryInput readOnly={false} reason="">
          <textarea aria-label="Message" />
        </HistoryInput>,
      ),
    ).toContain('textarea');
  });
});

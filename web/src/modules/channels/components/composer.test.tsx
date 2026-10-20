import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { Composer } from './composer';

function render(props: { running: boolean; working?: string }) {
  return renderToStaticMarkup(
    <QueryClientProvider client={new QueryClient()}>
      <Composer channelId={1} paused={false} {...props} />
    </QueryClientProvider>,
  );
}

describe('Composer', () => {
  it('keeps Send next to Stop and says the message runs next while a teammate works', () => {
    const markup = render({ running: true, working: 'developer' });
    expect(markup).toContain('aria-label="Send"');
    expect(markup).toContain('aria-label="Stop"');
    expect(markup).toContain('placeholder="Developer is working… your message runs next"');
  });

  it('shows only Send when idle', () => {
    const markup = render({ running: false });
    expect(markup).toContain('aria-label="Send"');
    expect(markup).not.toContain('aria-label="Stop"');
  });
});

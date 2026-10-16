import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it, vi } from 'vitest';
import { Composer } from './composer';

vi.mock('@/modules/channels/api/send-message', () => ({
  useSendMessage: () => ({ isPending: false, isError: false, mutate: vi.fn() }),
}));
vi.mock('@/modules/channels/api/stop-channel', () => ({
  useStopChannel: () => ({ isPending: false, mutate: vi.fn() }),
}));

describe('Composer', () => {
  it('shows Send beside Stop while a teammate works, and says the message runs next', () => {
    const markup = renderToStaticMarkup(
      <Composer channelId={1} running paused={false} working="developer" />,
    );

    expect(markup).toContain('aria-label="Stop"');
    expect(markup).toContain('aria-label="Send"');
    expect(markup).toContain('placeholder="Developer is working… your message runs next"');
  });

  it('shows only Send when nobody works', () => {
    const markup = renderToStaticMarkup(<Composer channelId={1} running={false} paused={false} />);

    expect(markup).not.toContain('aria-label="Stop"');
    expect(markup).toContain('aria-label="Send"');
  });
});

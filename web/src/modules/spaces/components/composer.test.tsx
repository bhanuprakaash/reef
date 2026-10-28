import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { renderToStaticMarkup } from 'react-dom/server';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { Composer, filesFromDrop } from './composer';

vi.mock('react', async (importOriginal) => ({
  ...(await importOriginal<typeof import('react')>()),
  useEffect: (effect: () => void) => void effect(),
}));

beforeEach(() => {
  vi.stubGlobal('window', {
    reef: undefined,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
  });
});

afterEach(() => vi.unstubAllGlobals());

function render(props: { running: boolean; working?: string; parent?: number }) {
  return renderToStaticMarkup(
    <QueryClientProvider client={new QueryClient()}>
      <Composer spaceId={1} paused={false} {...props} />
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

  it('checks the clipboard for a file in the main composer only, not in a thread', () => {
    const clipboardFile = vi.fn(() => Promise.resolve(null));
    vi.stubGlobal('window', {
      reef: { clipboardFile },
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    });

    render({ running: false });
    expect(clipboardFile).toHaveBeenCalledTimes(1);
    render({ running: false, parent: 5 });
    expect(clipboardFile).toHaveBeenCalledTimes(1);
  });

  it('shows no chip when the clipboard check rejects', async () => {
    const clipboardFile = vi.fn(() => Promise.reject(new Error('invalid file URL')));
    const unhandled = vi.fn();
    process.on('unhandledRejection', unhandled);
    vi.stubGlobal('window', {
      reef: { clipboardFile },
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    });

    render({ running: false });
    await new Promise((resolve) => setTimeout(resolve, 0));
    process.off('unhandledRejection', unhandled);

    expect(clipboardFile).toHaveBeenCalledTimes(1);
    expect(unhandled).not.toHaveBeenCalled();
  });

  it('does not cancel a plain-text drop', () => {
    const preventDefault = vi.fn();
    const files = filesFromDrop({ preventDefault, dataTransfer: { files: [] } }, false);

    expect(files).toEqual([]);
    expect(preventDefault).not.toHaveBeenCalled();
  });
});

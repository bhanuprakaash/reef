import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it, vi } from 'vitest';
import { ApiError } from '@/lib/api-client';
import { FilePanel } from '@/modules/channels/components/file-panel';

const { useFile } = vi.hoisted(() => ({ useFile: vi.fn() }));

vi.mock('@/modules/channels/api/get-file', () => ({ useFile }));

function render(line?: number) {
  return renderToStaticMarkup(
    <FilePanel channelId={1} path="./web/src/app.tsx" line={line} onClose={() => {}} />,
  );
}

describe('FilePanel', () => {
  it('shows a skeleton and the requested path while loading', () => {
    useFile.mockReturnValue({ isPending: true, isError: false });

    const html = render();

    expect(html).toContain('animate-pulse');
    expect(html).toContain('<bdi dir="ltr">./web/src/app.tsx</bdi>');
  });

  it.each([
    [404, 'Not found in this channel&#x27;s working folder'],
    [415, 'Binary file'],
    [400, 'path is outside the working folder'],
  ])('shows a %i error', (status, text) => {
    useFile.mockReturnValue({
      isPending: false,
      isError: true,
      error: new ApiError('path is outside the working folder', status),
    });

    expect(render()).toContain(text);
  });

  it('numbers every line, marks the requested one and notes a truncated file', () => {
    useFile.mockReturnValue({
      isPending: false,
      isError: false,
      data: {
        path: 'web/src/app.tsx',
        content: 'one\ntwo\nthree\n',
        size: 2 * 1024 * 1024,
        truncated: true,
      },
    });

    const html = render(2);

    expect(html).toContain('<bdi dir="ltr">web/src/app.tsx</bdi>');
    expect(html).toContain('Showing the first 512 KiB of 2.0 MiB');
    expect(html.match(/select-none[^>]*>\d+</g)?.map((m) => m.split('>')[1])).toEqual([
      '1<',
      '2<',
      '3<',
    ]);
    expect(html).toMatch(/<div class="[^"]*bg-current\/\[0.1\][^"]*">.*?>2<.*?>two</);
  });
});

import { renderToStaticMarkup } from 'react-dom/server';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { AddProviderDialog } from './add-provider-dialog';

const { useAddProvider } = vi.hoisted(() => ({ useAddProvider: vi.fn() }));

vi.mock('@/modules/providers/api/add-provider', () => ({ useAddProvider }));

afterEach(() => vi.unstubAllGlobals());

const render = () => renderToStaticMarkup(<AddProviderDialog open onClose={() => {}} />);

describe('AddProviderDialog', () => {
  it('offers Claude Code and Codex with Next disabled until one is picked', () => {
    useAddProvider.mockReturnValue({ isPending: false, isError: false, mutate: vi.fn() });
    vi.stubGlobal('window', {});

    const markup = render();

    expect(markup).toContain('Which CLI is in the folder?');
    expect(markup).toContain('Claude Code');
    expect(markup).toContain('codex binary');
    expect(markup).toMatch(/<button[^>]*disabled=""[^>]*>Next<\/button>/);
  });

  it('shows Choose only when the desktop bridge exists', () => {
    useAddProvider.mockReturnValue({ isPending: false, isError: false, mutate: vi.fn() });

    vi.stubGlobal('window', {});
    expect(render()).not.toContain('Choose…');

    vi.stubGlobal('window', { reef: { chooseFolder: vi.fn() } });
    expect(render()).toContain('Choose…');
  });

  it('shows the server error inside the dialog', () => {
    vi.stubGlobal('window', {});
    useAddProvider.mockReturnValue({
      isPending: false,
      isError: true,
      error: new Error('invalid: provider folder "/x": not a directory'),
      mutate: vi.fn(),
    });

    expect(render()).toContain('not a directory');
  });
});

import { renderToStaticMarkup } from 'react-dom/server';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { AddProviderForm } from './add-provider-form';

const { useAddProvider } = vi.hoisted(() => ({ useAddProvider: vi.fn() }));

vi.mock('@/modules/providers/api/add-provider', () => ({ useAddProvider }));

afterEach(() => vi.unstubAllGlobals());

describe('AddProviderForm', () => {
  it('shows Choose only when the desktop bridge exists', () => {
    useAddProvider.mockReturnValue({ isPending: false, isError: false, mutate: vi.fn() });

    vi.stubGlobal('window', {});
    expect(renderToStaticMarkup(<AddProviderForm />)).not.toContain('Choose…');

    vi.stubGlobal('window', { reef: { chooseFolder: vi.fn() } });
    expect(renderToStaticMarkup(<AddProviderForm />)).toContain('Choose…');
  });

  it('shows the server error under the form', () => {
    vi.stubGlobal('window', {});
    useAddProvider.mockReturnValue({
      isPending: false,
      isError: true,
      error: new Error('invalid: provider folder "/x": not a directory'),
      mutate: vi.fn(),
    });

    expect(renderToStaticMarkup(<AddProviderForm />)).toContain('not a directory');
  });
});

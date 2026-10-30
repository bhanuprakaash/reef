import { renderToStaticMarkup } from 'react-dom/server';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { CapturePage } from './capture';

const { useCreateWorkItem, useProjects, useBots, useStartConversation } = vi.hoisted(() => ({
  useCreateWorkItem: vi.fn(),
  useProjects: vi.fn(),
  useBots: vi.fn(() => ({ data: [] })),
  useStartConversation: vi.fn(() => ({
    isPending: false,
    isError: false,
    isSuccess: false,
    error: null,
    mutate: vi.fn(),
  })),
}));

vi.mock('@/modules/work-items/api/create-work-item', () => ({ useCreateWorkItem }));
vi.mock('@/modules/projects/api/get-projects', () => ({ useProjects }));
vi.mock('@/lib/api/get-bots', () => ({ useBots }));
vi.mock('@/modules/spaces/api/start-conversation', () => ({ useStartConversation }));

function stubWindow(reef?: object) {
  vi.stubGlobal('localStorage', { getItem: () => null, setItem: () => {} });
  vi.stubGlobal('window', { reef, localStorage: { getItem: () => null, setItem: () => {} } });
}

afterEach(() => vi.unstubAllGlobals());

describe('CapturePage', () => {
  it('shows the error when the capture request fails', () => {
    stubWindow();
    useProjects.mockReturnValue({ data: [] });
    useCreateWorkItem.mockReturnValue({
      isPending: false,
      isError: true,
      isSuccess: false,
      error: new Error('project not found'),
      mutate: vi.fn(),
    });

    const markup = renderToStaticMarkup(<CapturePage />);

    expect(markup).toContain('project not found');
  });

  it('shows Captured after a successful capture in a plain browser', () => {
    stubWindow();
    useProjects.mockReturnValue({ data: [] });
    useCreateWorkItem.mockReturnValue({
      isPending: false,
      isError: false,
      isSuccess: true,
      error: null,
      mutate: vi.fn(),
    });

    const markup = renderToStaticMarkup(<CapturePage />);

    expect(markup).toContain('Captured');
  });

  it('does not show Captured in the desktop window', () => {
    stubWindow({ capture: vi.fn() });
    useProjects.mockReturnValue({ data: [] });
    useCreateWorkItem.mockReturnValue({
      isPending: false,
      isError: false,
      isSuccess: true,
      error: null,
      mutate: vi.fn(),
    });

    const markup = renderToStaticMarkup(<CapturePage />);

    expect(markup).not.toContain('Captured');
  });
});

import { renderToStaticMarkup } from 'react-dom/server';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { CapturePage } from './capture';

const { useCreateTask, useProjects } = vi.hoisted(() => ({
  useCreateTask: vi.fn(),
  useProjects: vi.fn(),
}));

vi.mock('@/modules/tasks/api/create-task', () => ({ useCreateTask }));
vi.mock('@/modules/projects/api/get-projects', () => ({ useProjects }));

function stubWindow(reef?: object) {
  vi.stubGlobal('localStorage', { getItem: () => null, setItem: () => {} });
  vi.stubGlobal('window', { reef, localStorage: { getItem: () => null, setItem: () => {} } });
}

afterEach(() => vi.unstubAllGlobals());

describe('CapturePage', () => {
  it('shows the error when the capture request fails', () => {
    stubWindow();
    useProjects.mockReturnValue({ data: [] });
    useCreateTask.mockReturnValue({
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
    useCreateTask.mockReturnValue({
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
    useCreateTask.mockReturnValue({
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

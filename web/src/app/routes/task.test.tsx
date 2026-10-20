import { renderToStaticMarkup } from 'react-dom/server';
import { MemoryRouter, Route, Routes } from 'react-router';
import { describe, expect, it, vi } from 'vitest';
import type { Task } from '@/types/api';
import { TaskPage } from './task';

const { useTask, useFeatures, useProjects, mutate, selects } = vi.hoisted(() => ({
  useTask: vi.fn(),
  useFeatures: vi.fn(),
  useProjects: vi.fn(),
  mutate: vi.fn(),
  selects: [] as {
    'aria-label': string;
    value: string;
    options: { value: string; label: string }[];
    onChange: (value: string) => void;
  }[],
}));

vi.mock('@/modules/tasks/api/get-task', () => ({ useTask }));
vi.mock('@/modules/features/api/get-features', () => ({ useFeatures }));
vi.mock('@/modules/projects/api/get-projects', () => ({ useProjects }));
vi.mock('@/modules/features/api/get-worktree', () => ({
  useWorktree: () => ({ data: { path: '/data/worktrees/login' } }),
}));
vi.mock('@/modules/features/api/get-diff', () => ({ useDiff: () => ({ data: [] }) }));
vi.mock('@/modules/tasks/api/update-task', () => ({
  useUpdateTask: () => ({ isError: false, mutate }),
}));
vi.mock('@/components/select', () => ({
  Select: (props: (typeof selects)[number]) => {
    selects.push(props);
    return null;
  },
}));

const task: Task = {
  id: 7,
  projectId: 1,
  featureId: 3,
  title: 'Fix the login',
  body: 'First line\nSecond line',
  status: 'doing',
  createdAt: '2026-10-01T09:00:00Z',
  updatedAt: '2026-10-02T10:30:00Z',
};

function render() {
  return renderToStaticMarkup(
    <MemoryRouter initialEntries={['/tasks/7']}>
      <Routes>
        <Route path="/tasks/:id" element={<TaskPage />} />
      </Routes>
    </MemoryRouter>,
  );
}

describe('TaskPage', () => {
  it('shows the task with its full body, feature and dates', () => {
    useTask.mockReturnValue({ isPending: false, isError: false, data: task });
    useFeatures.mockReturnValue({
      data: [{ id: 3, name: 'Login', slug: 'login', kind: 'fixes', stage: 'build' }],
    });
    useProjects.mockReturnValue({ data: [{ id: 1, name: 'Reef', baseBranch: 'dev' }] });

    const markup = render();

    expect(useTask).toHaveBeenCalledWith(7);
    expect(markup).toContain('#7');
    expect(markup).toContain('Fix the login');
    expect(markup).toMatch(/aria-pressed="true"[^>]*>Doing</);
    expect(markup).toMatch(/aria-label="Edit task details">First line\nSecond line<\/button>/);
    expect(markup).toMatch(/<a [^>]*href="\/features\/3"[^>]*>Login<\/a>/);
    expect(markup).toContain('feature/login');
    expect(markup).toContain('>dev<');
    expect(markup).toContain('/data/worktrees/login');
    expect(markup).toContain('No changes yet');
    expect(markup).toContain('aria-label="Edit task title"');
    expect(markup).not.toMatch(/>Edit<\/button>/);
    expect(markup).toContain(new Date(task.createdAt).toLocaleString());
    expect(markup).toContain(new Date(task.updatedAt).toLocaleString());
  });

  it('shows the error when the task is not found', () => {
    useTask.mockReturnValue({
      isPending: false,
      isError: true,
      error: new Error('task not found'),
    });
    useFeatures.mockReturnValue({ data: [] });
    useProjects.mockReturnValue({ data: [] });

    expect(render()).toContain('task not found');
  });

  it('moves the task to another feature of its project, or to none', () => {
    useTask.mockReturnValue({ isPending: false, isError: false, data: task });
    useFeatures.mockReturnValue({
      data: [
        { id: 3, projectId: 1, name: 'Login', slug: 'login', kind: 'fixes', stage: 'build' },
        { id: 4, projectId: 1, name: 'Billing', slug: 'billing', kind: 'fixes', stage: 'build' },
        { id: 5, projectId: 2, name: 'Other', slug: 'other', kind: 'fixes', stage: 'build' },
      ],
    });
    useProjects.mockReturnValue({ data: [{ id: 1, name: 'Reef', baseBranch: 'dev' }] });
    selects.length = 0;
    mutate.mockClear();

    render();
    const select = selects.find((s) => s['aria-label'] === 'Feature');

    expect(select?.value).toBe('3');
    expect(select?.options.map((o) => o.label)).toEqual(['No feature', 'Login', 'Billing']);
    select?.onChange('4');
    select?.onChange('');
    expect(mutate).toHaveBeenNthCalledWith(1, { task, featureId: 4 });
    expect(mutate).toHaveBeenNthCalledWith(2, { task, featureId: null });
  });
});

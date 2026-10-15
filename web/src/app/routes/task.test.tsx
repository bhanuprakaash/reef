import { renderToStaticMarkup } from 'react-dom/server';
import { MemoryRouter, Route, Routes } from 'react-router';
import { describe, expect, it, vi } from 'vitest';
import type { Task } from '@/types/api';
import { TaskPage } from './task';

const { useTask, useFeatures } = vi.hoisted(() => ({
  useTask: vi.fn(),
  useFeatures: vi.fn(),
}));

vi.mock('@/modules/tasks/api/get-task', () => ({ useTask }));
vi.mock('@/modules/features/api/get-features', () => ({ useFeatures }));

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
    useFeatures.mockReturnValue({ data: [{ id: 3, name: 'Login' }] });

    const markup = render();

    expect(useTask).toHaveBeenCalledWith(7);
    expect(markup).toContain('#7');
    expect(markup).toContain('Fix the login');
    expect(markup).toContain('doing');
    expect(markup).toMatch(/class="[^"]*whitespace-pre-wrap[^"]*">First line\nSecond line</);
    expect(markup).toMatch(/<a [^>]*href="\/features\/3"[^>]*>Login<\/a>/);
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

    expect(render()).toContain('task not found');
  });
});

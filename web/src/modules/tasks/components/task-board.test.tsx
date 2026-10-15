import { renderToStaticMarkup } from 'react-dom/server';
import { MemoryRouter } from 'react-router';
import { describe, expect, it, vi } from 'vitest';
import type { Task } from '@/types/api';
import { TaskBoard } from './task-board';

const { useUpdateTask } = vi.hoisted(() => ({ useUpdateTask: vi.fn() }));

vi.mock('@/modules/tasks/api/update-task', () => ({ useUpdateTask }));

const task: Task = {
  id: 12,
  projectId: 1,
  featureId: 3,
  title: 'Fix the login',
  body: 'The body of the task',
  status: 'todo',
  createdAt: '2026-10-01T09:00:00Z',
  updatedAt: '2026-10-01T09:00:00Z',
};

describe('TaskBoard', () => {
  it('links #N and the title to the task page and clamps the body to two lines', () => {
    useUpdateTask.mockReturnValue({ isPending: false, isError: false });

    const markup = renderToStaticMarkup(
      <MemoryRouter>
        <TaskBoard tasks={[task]} features={[]} />
      </MemoryRouter>,
    );

    expect(markup).toMatch(/<a [^>]*href="\/tasks\/12"[^>]*>.*#12.*Fix the login.*<\/a>/s);
    expect(markup).toMatch(/class="[^"]*line-clamp-2[^"]*">The body of the task</);
  });
});

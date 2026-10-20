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
  it('links #N to the task page and makes the title and body editable in place', () => {
    useUpdateTask.mockReturnValue({ isPending: false, isError: false });

    const markup = renderToStaticMarkup(
      <MemoryRouter>
        <TaskBoard tasks={[task]} features={[]} />
      </MemoryRouter>,
    );

    expect(markup).toMatch(/<a [^>]*href="\/tasks\/12"[^>]*>.*#12.*<\/a>/s);
    expect(markup).toContain('aria-label="Edit task title"');
    expect(markup).toContain('aria-label="Edit task details"');
    expect(markup).toMatch(/class="[^"]*line-clamp-2[^"]*"[^>]*>The body of the task</);
  });
});

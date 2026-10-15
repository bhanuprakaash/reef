import { renderToStaticMarkup } from 'react-dom/server';
import { MemoryRouter } from 'react-router';
import { describe, expect, it, vi } from 'vitest';
import type { Task } from '@/types/api';
import { TaskList } from './task-list';

const { useUpdateTask } = vi.hoisted(() => ({ useUpdateTask: vi.fn() }));

vi.mock('@/modules/tasks/api/update-task', () => ({ useUpdateTask }));

const task: Task = {
  id: 12,
  projectId: 1,
  featureId: null,
  title: 'Fix the login',
  body: 'The body of the task',
  status: 'todo',
  createdAt: '2026-10-01T09:00:00Z',
  updatedAt: '2026-10-01T09:00:00Z',
};

function render() {
  useUpdateTask.mockReturnValue({ isPending: false, isError: false });
  return renderToStaticMarkup(
    <MemoryRouter>
      <TaskList tasks={[task]} />
    </MemoryRouter>,
  );
}

describe('TaskList', () => {
  it('links #N and the title to the task page and clamps the body to two lines', () => {
    const markup = render();

    expect(markup).toMatch(/<a [^>]*href="\/tasks\/12"[^>]*>.*#12.*Fix the login.*<\/a>/s);
    expect(markup).toMatch(/class="[^"]*line-clamp-2[^"]*">The body of the task</);
  });

  it('keeps the link outside the checkbox label so clicking it does not tick the task', () => {
    const markup = render();

    expect(markup).not.toMatch(/<label[^>]*>(?:(?!<\/label>).)*<a /s);
  });
});

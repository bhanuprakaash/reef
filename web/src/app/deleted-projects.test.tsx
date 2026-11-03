import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it, vi } from 'vitest';
import { DeletedProjects } from './deleted-projects';

vi.mock('@/modules/projects/api/get-projects', () => ({
  useDeletedProjects: () => ({ data: [{ id: 1, name: 'Old' }] }),
}));

describe('DeletedProjects', () => {
  it('starts collapsed', () => {
    const html = renderToStaticMarkup(<DeletedProjects />);
    expect(html).toContain('Deleted projects');
    expect(html).toContain('aria-expanded="false"');
    expect(html).not.toContain('Old');
  });
});

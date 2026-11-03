import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it, vi } from 'vitest';
import { SpaceProjects } from './space-projects';

vi.mock('@/modules/spaces/api/space-projects', () => ({
  useSpaceProjects: () => ({ isSuccess: true, data: [{ id: 1, name: 'Old', deletingAt: 'now' }] }),
  useSetSpaceProject: () => ({}),
}));

describe('SpaceProjects', () => {
  it('shows a deleted association without a remove action in read-only history', () => {
    const html = renderToStaticMarkup(<SpaceProjects spaceId={1} projects={[]} readOnly />);
    expect(html).toContain('Old (deleted)');
    expect(html).not.toContain('Remove');
    expect(html).not.toContain('Add Project');
  });
});

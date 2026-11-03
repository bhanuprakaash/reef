import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it, vi } from 'vitest';
import { RequestList } from './request-list';

const { useRepoRequests } = vi.hoisted(() => ({ useRepoRequests: vi.fn() }));

vi.mock('@/modules/spaces/api/get-repo-requests', () => ({ useRepoRequests }));

const request = {
  host: 'gitlab.com',
  path: 'group/app',
  review: '',
  checks: [],
  checkedAt: null,
  checkError: '',
  dependsOn: [],
};

function render(data: unknown[], empty?: string) {
  useRepoRequests.mockReturnValue({ data });
  return renderToStaticMarkup(
    <RequestList path="/spaces/3/requests" workspaceId={1} empty={empty} />,
  );
}

describe('RequestList', () => {
  it('shows state, failing checks, review, merge order and a read error of each request', () => {
    const markup = render([
      {
        ...request,
        id: 1,
        provider: 'gitlab',
        number: 7,
        url: 'https://gitlab.com/group/app/-/merge_requests/7',
        state: 'closed',
        workspaceId: 1,
        review: 'review_required',
        checks: [{ name: 'pipeline', state: 'fail', key: '9' }],
        dependsOn: [2],
        checkError: 'provider unreachable',
      },
      {
        ...request,
        id: 2,
        provider: 'github',
        path: 'org/lib',
        number: 4,
        url: 'https://github.com/org/lib/pull/4',
        state: 'merged',
        workspaceId: 2,
      },
    ]);

    expect(markup).toContain('Closed, not merged');
    expect(markup).toContain('group/app!7');
    expect(markup).toContain('review required');
    expect(markup).toContain('failing: pipeline');
    expect(markup).toContain('merge step 2, after org/lib#4');
    expect(markup).toContain('Status not read: provider unreachable');
    expect(markup).not.toContain('Merged');
  });

  it('numbers the merge steps of the requests of one fix across repositories', () => {
    useRepoRequests.mockReturnValue({
      data: [
        { ...request, id: 1, provider: 'gitlab', number: 12, state: 'opened', dependsOn: [2] },
        { ...request, id: 2, provider: 'gitlab', path: 'group/lib', number: 11, state: 'opened' },
        { ...request, id: 3, provider: 'gitlab', number: 13, state: 'opened' },
      ],
    });
    const markup = renderToStaticMarkup(<RequestList path="/work-items/1/requests" />);

    expect(markup).toMatch(/app!12.*merge step 2, after group\/lib!11.*lib!11.*merge step 1/);
    expect(markup.match(/merge step/g)).toHaveLength(2);
  });

  it('shows the empty text, or nothing, when the workspace has no request', () => {
    expect(render([], 'Not opened yet')).toContain('Not opened yet');
    expect(render([])).toBe('');
  });
});

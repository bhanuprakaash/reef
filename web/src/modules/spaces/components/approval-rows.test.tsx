import { renderToStaticMarkup } from 'react-dom/server';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ApprovalRows } from './approval-rows';

const { useApprovals, useDecideApproval } = vi.hoisted(() => ({
  useApprovals: vi.fn(),
  useDecideApproval: vi.fn(),
}));

const sourceMessage = { id: 3, body: 'Source message body' };
const approval = {
  id: 1,
  spaceId: 2,
  from: 'architect',
  to: 'developer',
  messageId: sourceMessage.id,
  text: 'Approval text',
  status: 'pending',
  createdAt: '',
};

vi.mock('@/modules/spaces/api/get-approvals', () => ({ useApprovals }));
vi.mock('@/lib/api/decide-approval', () => ({ useDecideApproval }));

describe('ApprovalRows', () => {
  beforeEach(() => {
    useApprovals.mockReturnValue({
      data: [approval],
    });
    useDecideApproval.mockReturnValue({ isPending: false, isError: false });
  });

  it('shows the handed-off message body without the wake header', () => {
    useApprovals.mockReturnValue({
      data: [{ ...approval, text: '[Space 2, message 3]\narchitect wrote: Build the plan' }],
    });

    const markup = renderToStaticMarkup(<ApprovalRows spaceId={2} faces={{}} />);

    expect(markup).toContain('Build the plan');
    expect(markup).not.toContain('[Space 2');
    expect(markup).not.toContain('architect wrote');
  });

  it('shows the approval text', () => {
    const markup = renderToStaticMarkup(<ApprovalRows spaceId={2} faces={{}} />);

    expect(approval.text).not.toBe(sourceMessage.body);
    expect(markup).toContain(approval.text);
    expect(markup).not.toContain(sourceMessage.body);
  });

  it('shows who hands to whom with Approve and Reject', () => {
    const markup = renderToStaticMarkup(<ApprovalRows spaceId={2} faces={{}} />);

    expect(markup).toContain('architect → developer');
    expect(markup).toContain('Approve');
    expect(markup).toContain('Reject');
  });

  it('leaves the approval of a direct wake to the coordination item', () => {
    useApprovals.mockReturnValue({
      data: [{ ...approval, text: 'Private request', directMessageId: 4 }],
    });

    const markup = renderToStaticMarkup(<ApprovalRows spaceId={2} faces={{}} />);

    expect(markup).toBe('');
  });
});

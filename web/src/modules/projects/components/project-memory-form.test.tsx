import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it, vi } from 'vitest';
import { ProjectMemoryForm } from './project-memory-form';

const { useMemory, useAddMemory, useRetireMemory } = vi.hoisted(() => ({
  useMemory: vi.fn(),
  useAddMemory: vi.fn(),
  useRetireMemory: vi.fn(),
}));
vi.mock('@/lib/api/get-memory', () => ({ useMemory }));
vi.mock('@/lib/api/add-memory', () => ({ useAddMemory }));
vi.mock('@/lib/api/retire-memory', () => ({ useRetireMemory }));

function render(added?: object) {
  useMemory.mockReturnValue({ data: [], isPending: false, isError: false });
  useAddMemory.mockReturnValue({ data: added, isPending: false, isError: false });
  useRetireMemory.mockReturnValue({ isPending: false, isError: false });
  return renderToStaticMarkup(<ProjectMemoryForm projectId={1} />);
}

describe('ProjectMemoryForm', () => {
  it('shows the stored item an added item looks like', () => {
    const markup = render({ id: 5, similar: [{ id: 2, body: 'run make test before a push' }] });

    expect(markup).toContain('Looks like an existing item (2): run make test before a push');
  });

  it('says why an added item has no vector', () => {
    const markup = render({ id: 5, embedError: 'connection refused' });

    expect(markup).toContain('Saved without a vector: connection refused');
  });

  it('shows no note for an item that is new and embedded', () => {
    const markup = render({ id: 5 });

    expect(markup).not.toContain('Looks like');
    expect(markup).not.toContain('without a vector');
  });
});

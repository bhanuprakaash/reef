import { describe, expect, it, vi } from 'vitest';
import { highlight } from '@/lib/highlight';

const { createHighlighterCore } = vi.hoisted(() => ({ createHighlighterCore: vi.fn() }));

vi.mock('shiki/core', () => ({ createHighlighterCore }));

describe('highlight', () => {
  it('retries loading the highlighter after a failed load', async () => {
    createHighlighterCore.mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce({
      getLoadedLanguages: () => ['go'],
      codeToTokens: () => ({ tokens: [[{ content: 'x', htmlStyle: undefined }]] }),
    });

    await expect(highlight('x', 'go')).rejects.toThrow('offline');
    await expect(highlight('x', 'go')).resolves.toEqual([[{ content: 'x', style: undefined }]]);
  });

  it('leaves a block over 2000 lines unhighlighted', async () => {
    await expect(highlight('x\n'.repeat(2000), 'go')).resolves.toBeNull();
  });
});

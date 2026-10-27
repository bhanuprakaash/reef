import { describe, expect, it } from 'vitest';
import { subagentLabel } from './subagent-label';

describe('subagentLabel', () => {
  it('joins the subagent type and the description of an Agent call', () => {
    expect(
      subagentLabel('Agent', {
        description: 'Comment pass over diff',
        subagent_type: 'comment-sicko',
        prompt: 'Review it',
      }),
    ).toBe('comment-sicko · Comment pass over diff');
  });

  it('falls back to the description alone', () => {
    expect(subagentLabel('Agent', { description: 'Find the bug', prompt: 'x' })).toBe(
      'Find the bug',
    );
  });

  it('has no label for another tool or an Agent call without either field', () => {
    expect(subagentLabel('Bash', { description: 'ls', subagent_type: 'x' })).toBeUndefined();
    expect(subagentLabel('Agent', { prompt: 'x' })).toBeUndefined();
    expect(subagentLabel('Agent', undefined)).toBeUndefined();
  });
});

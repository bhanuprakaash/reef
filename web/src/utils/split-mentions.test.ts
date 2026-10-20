import { describe, expect, it } from 'vitest';
import { splitMentions } from './split-mentions';

describe('splitMentions', () => {
  it('marks @words at the start or after whitespace', () => {
    expect(splitMentions('@dev check @arc-1\n@qa')).toEqual([
      { value: '@dev', mention: true },
      { value: ' check ', mention: false },
      { value: '@arc-1', mention: true },
      { value: '\n', mention: false },
      { value: '@qa', mention: true },
    ]);
  });

  it('leaves emails and plain text alone', () => {
    expect(splitMentions('me@x.io')).toEqual([{ value: 'me@x.io', mention: false }]);
    expect(splitMentions('')).toEqual([]);
  });
});

import { describe, expect, it } from 'vitest';
import { splitMentions } from './split-mentions';

describe('splitMentions', () => {
  it('marks an @name at the start of the text or of a line, after spaces or tabs', () => {
    expect(splitMentions('@dev check\n \t@qa now')).toEqual([
      { value: '@dev', mention: true },
      { value: ' check\n \t', mention: false },
      { value: '@qa', mention: true },
      { value: ' now', mention: false },
    ]);
  });

  it('marks every @name of an unbroken run at the start', () => {
    expect(splitMentions('@architect, @tester do X')).toEqual([
      { value: '@architect', mention: true },
      { value: ', ', mention: false },
      { value: '@tester', mention: true },
      { value: ' do X', mention: false },
    ]);
  });

  it('leaves an @name inside a sentence, after a word break in the run, emails and empty text alone', () => {
    expect(splitMentions('looks good, @developer please fix')).toEqual([
      { value: 'looks good, @developer please fix', mention: false },
    ]);
    expect(splitMentions('@architect and @tester do X')).toEqual([
      { value: '@architect', mention: true },
      { value: ' and @tester do X', mention: false },
    ]);
    expect(splitMentions('me@x.io')).toEqual([{ value: 'me@x.io', mention: false }]);
    expect(splitMentions('')).toEqual([]);
  });
});

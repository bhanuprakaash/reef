import { describe, expect, it } from 'vitest';
import { handoffBody } from './handoff-body';

describe('handoffBody', () => {
  it('strips the message header and the author prefix', () => {
    expect(handoffBody('[channel 1, message 5]\ntester wrote: Now build web.\n\nDone.')).toBe(
      'Now build web.\n\nDone.',
    );
  });

  it('shows the last message of a wake that holds several', () => {
    const wake =
      '[channel 1, message 5]\ntester wrote: first\n\n[channel 1, message 6]\nlead wrote: second';
    expect(handoffBody(wake)).toBe('second');
  });

  it('drops the thread so far before a reply', () => {
    const wake =
      '[channel 1, message 9, thread 4]\nThread so far:\nuser: hi\n\nlead: ok\n\ntester wrote: result';
    expect(handoffBody(wake)).toBe('result');
  });

  it('keeps text that has no header', () => {
    expect(handoffBody('Approval text')).toBe('Approval text');
  });
});

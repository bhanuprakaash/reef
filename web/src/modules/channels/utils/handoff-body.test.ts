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

  it('strips the header and prefix of a direct wake', () => {
    expect(
      handoffBody(
        '[channel 1, direct message 2, chain 1, source: direct request]\nlead wrote privately: Review the plan.',
      ),
    ).toBe('Review the plan.');
  });

  it('strips the line naming everyone asked together', () => {
    expect(
      handoffBody('[channel 1, message 5]\nAsked together: @developer, @tester\nBuild it'),
    ).toBe('Build it');
  });

  it('keeps text that has no header', () => {
    expect(handoffBody('Approval text')).toBe('Approval text');
  });
});

import { describe, expect, it } from 'vitest';
import { dockLayout } from './dock-layout';

const running = { status: 'running' as const };
const failed = { status: 'failed' as const };

describe('dockLayout', () => {
  it('shows pills for one or two working Bots', () => {
    expect(dockLayout([running])).toBe('pills');
    expect(dockLayout([running, running])).toBe('pills');
  });

  it('switches to the tide row at three working Bots', () => {
    expect(dockLayout([running, running, running])).toBe('tide');
  });

  it('counts only the Bots still working', () => {
    expect(dockLayout([running, running, failed])).toBe('pills');
  });
});

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createHoverIntent } from './hover-intent';

describe('createHoverIntent', () => {
  const onChange = vi.fn();
  beforeEach(() => {
    vi.useFakeTimers();
    onChange.mockClear();
  });
  afterEach(() => vi.useRealTimers());

  it('opens after the delay, not before', () => {
    const intent = createHoverIntent(onChange, 400);
    intent.enter();
    vi.advanceTimersByTime(399);
    expect(onChange).not.toHaveBeenCalled();
    vi.advanceTimersByTime(1);
    expect(onChange).toHaveBeenCalledExactlyOnceWith(true);
  });

  it('never opens when the pointer leaves before the delay', () => {
    const intent = createHoverIntent(onChange, 400);
    intent.enter();
    vi.advanceTimersByTime(200);
    intent.leave();
    vi.advanceTimersByTime(1000);
    expect(onChange).not.toHaveBeenCalled();
  });

  it('closes after the grace, and stays open when the pointer comes back in time', () => {
    const intent = createHoverIntent(onChange, 400, 100);
    intent.focus();
    onChange.mockClear();
    intent.leave();
    vi.advanceTimersByTime(99);
    intent.enter();
    vi.advanceTimersByTime(1000);
    expect(onChange).not.toHaveBeenCalled();
    intent.leave();
    vi.advanceTimersByTime(100);
    expect(onChange).toHaveBeenCalledExactlyOnceWith(false);
  });

  it('opens on focus and closes on blur and on dismiss at once', () => {
    const intent = createHoverIntent(onChange, 400);
    intent.focus();
    expect(onChange).toHaveBeenLastCalledWith(true);
    intent.blur();
    expect(onChange).toHaveBeenLastCalledWith(false);
    intent.focus();
    intent.dismiss();
    expect(onChange.mock.calls).toEqual([[true], [false], [true], [false]]);
  });

  it('dismiss cancels a pending open', () => {
    const intent = createHoverIntent(onChange, 400);
    intent.enter();
    intent.dismiss();
    vi.advanceTimersByTime(1000);
    expect(onChange).not.toHaveBeenCalled();
  });
});

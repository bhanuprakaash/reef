import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { CleanupStatus } from './cleanup-status';

const render = (state: string, snapshotRef: string, cleanupError: string) =>
  renderToStaticMarkup(<CleanupStatus w={{ state, snapshotRef, cleanupError }} />);

describe('CleanupStatus', () => {
  it('shows a failed cleanup as retrying', () => {
    expect(render('ready', '', 'save changes: permission denied')).toContain(
      'Cleanup failed: save changes: permission denied. Retrying.',
    );
  });

  it('names the ref that keeps the unfinished changes of a removed workspace', () => {
    expect(render('removed', 'refs/reef/snapshots/ab12/7', '')).toContain(
      'Unfinished changes saved at <code class="font-mono">refs/reef/snapshots/ab12/7</code>.',
    );
  });

  it('shows nothing for a workspace in use', () => {
    expect(render('ready', '', '')).toBe('');
  });
});

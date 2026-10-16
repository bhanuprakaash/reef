import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { StatusDot } from './status-dot';

describe('StatusDot', () => {
  it('pulses with a tooltip while a teammate is working', () => {
    const markup = renderToStaticMarkup(
      <StatusDot status="working" label="developer is working" />,
    );

    expect(markup).toContain('title="developer is working"');
    expect(markup).toContain('aria-label="developer is working"');
    expect(markup).toContain('animate-ping');
    expect(markup).toContain('bg-working');
  });

  it('stays still for other statuses', () => {
    const markup = renderToStaticMarkup(<StatusDot status="asleep" />);

    expect(markup).not.toContain('animate-ping');
    expect(markup).toContain('aria-hidden="true"');
  });
});

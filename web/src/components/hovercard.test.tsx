import { renderToStaticMarkup } from 'react-dom/server';
import { MemoryRouter } from 'react-router';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { Hovercard, HovercardPanel } from './hovercard';

describe('Hovercard', () => {
  it('renders only the trigger until it opens, with no description pointing at a missing card', () => {
    const markup = renderToStaticMarkup(
      <MemoryRouter>
        <Hovercard content={<p>details</p>}>
          {(trigger) => (
            <a href="/x" {...trigger}>
              row
            </a>
          )}
        </Hovercard>
      </MemoryRouter>,
    );
    expect(markup).toBe('<a href="/x">row</a>');
  });
});

describe('HovercardPanel', () => {
  const anchor = { top: 100, right: 300, bottom: 140 } as DOMRect;
  afterEach(() => vi.unstubAllGlobals());

  it('is a tooltip with the id the trigger describes itself by, beside the trigger', () => {
    vi.stubGlobal('window', { innerHeight: 800 });
    const markup = renderToStaticMarkup(
      <HovercardPanel id=":r1:" anchor={anchor}>
        details
      </HovercardPanel>,
    );
    expect(markup).toContain('id=":r1:"');
    expect(markup).toContain('role="tooltip"');
    expect(markup).toContain('left:308px;top:100px');
    expect(markup).toContain('details');
  });

  it('hangs from the trigger bottom when the trigger is in the lower half', () => {
    vi.stubGlobal('window', { innerHeight: 150 });
    const markup = renderToStaticMarkup(
      <HovercardPanel id="a" anchor={anchor}>
        x
      </HovercardPanel>,
    );
    expect(markup).toContain('left:308px;bottom:10px');
  });
});

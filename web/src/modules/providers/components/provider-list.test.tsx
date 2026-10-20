import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { ProviderRow } from './provider-list';

describe('ProviderRow', () => {
  it('shows a detected install with its path, version and models, and no Remove', () => {
    const markup = renderToStaticMarkup(
      <ProviderRow
        provider={{
          name: 'claude',
          kind: 'claude',
          detected: true,
          path: '/opt/bin/claude',
          version: '2.1.0 (Claude Code)',
        }}
        onRemove={() => {}}
      />,
    );

    expect(markup).toContain('/opt/bin/claude');
    expect(markup).toContain('2.1.0 (Claude Code)');
    expect(markup).toContain('Default, Opus, Sonnet, Haiku');
    expect(markup).toContain('Found on PATH');
    expect(markup).not.toContain('Remove');
  });

  it('shows an added install with its folder and a Remove button', () => {
    const markup = renderToStaticMarkup(
      <ProviderRow
        provider={{ name: 'lab', kind: 'codex', detected: false, folder: '/work/codex' }}
        onRemove={() => {}}
      />,
    );

    expect(markup).toContain('/work/codex');
    expect(markup).toContain('Binary not found');
    expect(markup).toContain('aria-label="Remove lab"');
  });
});

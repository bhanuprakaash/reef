import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { ProviderRow } from './provider-list';

describe('ProviderRow', () => {
  it('shows a detected install as Ready with its path, version and models, and no Remove', () => {
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

    expect(markup).toContain('2.1.0 (Claude Code) · PATH');
    expect(markup).toContain('Ready');
    expect(markup).toContain('/opt/bin/claude');
    expect(markup).toContain('Found on PATH');
    expect(markup).toContain('Opus');
    expect(markup).not.toContain('Remove');
  });

  it('shows an added install without a binary as not found, with a Remove button', () => {
    const markup = renderToStaticMarkup(
      <ProviderRow
        provider={{ name: 'lab', kind: 'codex', detected: false, folder: '/work/codex' }}
        onRemove={() => {}}
      />,
    );

    expect(markup).toContain('/work/codex');
    expect(markup).toContain('Binary not found');
    expect(markup).toContain('Any model, set on each Bot');
    expect(markup).toContain('aria-label="Remove lab"');
  });
});

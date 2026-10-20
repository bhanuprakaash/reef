import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { ClipboardChip } from './clipboard-chip';

describe('ClipboardChip', () => {
  it('names the file on the clipboard and offers to paste it', () => {
    const markup = renderToStaticMarkup(
      <ClipboardChip path="/Users/ada/Desktop/shot.png" onPaste={() => {}} />,
    );
    expect(markup).toContain('aria-label="Paste file from clipboard: shot.png"');
    expect(markup).toContain('File in clipboard');
    expect(markup).toContain('>Paste<');
  });
});

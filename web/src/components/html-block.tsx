import { useState } from 'react';
import { CodeBlock } from '@/components/code-block';
import { CopyButton } from '@/components/copy-button';
import { SegmentedControl } from '@/components/segmented-control';

// The policy must come first in the page. It stops remote images, styles, fonts and frames, so a
// preview sends no request.
const policy = `<meta http-equiv="Content-Security-Policy" content="default-src 'none'; img-src data:; style-src 'unsafe-inline'">`;

// HtmlBlock previews a ```html fence in an iframe with an empty sandbox: no scripts, no forms,
// no access to the window. Never add allow-scripts or allow-same-origin. The page stays white in
// both themes, as HTML assumes.
export function HtmlBlock({ code }: { code: string }) {
  const [view, setView] = useState<'preview' | 'source'>('preview');

  return (
    <div className="flex flex-col gap-2">
      <div className="flex items-center justify-between">
        <SegmentedControl
          label="HTML view"
          options={[
            { value: 'preview', label: 'Preview' },
            { value: 'source', label: 'Source' },
          ]}
          value={view}
          onChange={setView}
        />
        {view === 'preview' && <CopyButton text={code} />}
      </div>
      {view === 'preview' ? (
        <iframe
          sandbox=""
          srcDoc={policy + code}
          title="HTML preview"
          style={{ background: '#fff' }}
          className="h-80 w-full rounded-lg border border-current/10 [color-scheme:light]"
        />
      ) : (
        <CodeBlock code={code} lang="html" />
      )}
    </div>
  );
}

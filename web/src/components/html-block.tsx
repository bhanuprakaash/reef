import { useState } from 'react';
import { CodeBlock } from '@/components/code-block';
import { CopyButton } from '@/components/copy-button';
import { SegmentedControl } from '@/components/segmented-control';

// HtmlBlock previews a ```html fence in an iframe with an empty sandbox: no scripts, no forms,
// no access to the window. Never add allow-scripts or allow-same-origin.
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
          srcDoc={code}
          title="HTML preview"
          className="h-80 w-full rounded-lg border border-current/10 bg-white [color-scheme:light]"
        />
      ) : (
        <CodeBlock code={code} lang="html" />
      )}
    </div>
  );
}

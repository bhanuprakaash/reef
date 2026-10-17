import { useEffect, useState } from 'react';
import { CopyButton } from '@/components/copy-button';
import { highlight, type Line } from '@/lib/highlight';

export function CodeBlock({ code, lang }: { code: string; lang: string }) {
  const [lines, setLines] = useState<{ code: string; lines: Line[] } | null>(null);

  useEffect(() => {
    let live = true;
    highlight(code, lang)
      .then((result) => live && result && setLines({ code, lines: result }))
      .catch(() => undefined);
    return () => {
      live = false;
    };
  }, [code, lang]);

  const done = lines?.code === code ? lines.lines : null;

  return (
    <div className="overflow-hidden rounded-lg border border-current/10 bg-current/[0.05]">
      <div className="flex items-center justify-between border-b border-current/10 px-3 py-1">
        <span className="font-mono text-[11.5px] text-muted">{lang || 'text'}</span>
        <CopyButton text={code} />
      </div>
      <pre className="overflow-x-auto px-3.5 py-3 font-mono text-[12.5px] leading-[1.55]">
        <code>
          {done
            ? done.map((line, i) => (
                <span key={i} className="block min-h-[1.55em]">
                  {line.map((token, j) => (
                    <span key={j} className="shiki-token" style={token.style}>
                      {token.content}
                    </span>
                  ))}
                </span>
              ))
            : code}
        </code>
      </pre>
    </div>
  );
}

import { Fragment, useEffect, useRef, useState } from 'react';
import { CopyButton } from '@/components/copy-button';
import { highlight, type Line } from '@/lib/highlight';

export function CodeBlock({ code, lang }: { code: string; lang: string }) {
  const [shown, setShown] = useState<{ code: string; lines: Line[] } | null>(null);
  const drawn = useRef(false);

  // A streaming reply changes the code on every chunk: highlight once it pauses, and until then
  // keep the last highlighted lines with the new text after them.
  useEffect(() => {
    let live = true;
    const timer = setTimeout(
      () => {
        highlight(code, lang)
          .then((lines) => {
            if (!live) return;
            drawn.current = lines !== null;
            setShown(lines && { code, lines });
          })
          .catch(() => undefined);
      },
      drawn.current ? 150 : 0,
    );
    return () => {
      live = false;
      clearTimeout(timer);
    };
  }, [code, lang]);

  const rest = shown && code.startsWith(shown.code) ? code.slice(shown.code.length) : null;

  return (
    <div className="overflow-hidden rounded-lg border border-current/10 bg-current/[0.05]">
      <div className="flex items-center justify-between border-b border-current/10 px-3 py-1">
        <span className="font-mono text-[11.5px] text-muted">{lang || 'text'}</span>
        <CopyButton text={code} />
      </div>
      <pre className="overflow-x-auto px-3.5 py-3 font-mono text-[12.5px] leading-[1.55]">
        <code>
          {shown && rest !== null ? (
            <>
              {shown.lines.map((line, i) => (
                <Fragment key={i}>
                  {i > 0 && '\n'}
                  {line.map((token, j) => (
                    <span key={j} className="shiki-token" style={token.style}>
                      {token.content}
                    </span>
                  ))}
                </Fragment>
              ))}
              {rest}
            </>
          ) : (
            code
          )}
        </code>
      </pre>
    </div>
  );
}

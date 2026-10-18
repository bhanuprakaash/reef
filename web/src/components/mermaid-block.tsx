import { useEffect, useState } from 'react';
import { CodeBlock } from '@/components/code-block';
import { Skeleton } from '@/components/skeleton';
import { useResolvedTheme } from '@/lib/use-resolved-theme';

let next = 0;

// MermaidBlock draws a ```mermaid fence, loading mermaid on the first diagram. The SVG is shown
// as an image, so nothing in it can run or restyle the page.
export function MermaidBlock({ code }: { code: string }) {
  const theme = useResolvedTheme();
  const [svg, setSvg] = useState<string | null>(null);
  const [error, setError] = useState<{ key: string; message: string } | null>(null);
  const key = `${theme}\n${code}`;

  // A streaming reply changes the source on every chunk: draw once it pauses, keep the last
  // drawing meanwhile, and report an error only for source that has stopped changing.
  useEffect(() => {
    let live = true;
    let failed: ReturnType<typeof setTimeout> | undefined;
    const timer = setTimeout(() => {
      import('mermaid')
        .then(async ({ default: mermaid }) => {
          mermaid.initialize({
            startOnLoad: false,
            securityLevel: 'strict',
            theme: theme === 'light' ? 'default' : 'dark',
          });
          // parse first: a failed render leaves its error drawing in the page body.
          await mermaid.parse(code);
          const { svg } = await mermaid.render(`mermaid-${++next}`, code);
          if (live) setSvg(svg);
        })
        .catch((e: unknown) => {
          const message = e instanceof Error ? e.message : String(e);
          if (live) failed = setTimeout(() => setError({ key, message }), 350);
        });
    }, 250);
    return () => {
      live = false;
      clearTimeout(timer);
      clearTimeout(failed);
    };
  }, [code, theme, key]);

  if (error?.key === key) {
    return (
      <div className="flex flex-col gap-1">
        <CodeBlock code={code} lang="mermaid" />
        <p className="whitespace-pre-wrap font-mono text-[11.5px] text-muted">{error.message}</p>
      </div>
    );
  }

  if (svg === null) return <Skeleton className="h-32" />;

  // The SVG fills its box; its own max-width keeps it at the size mermaid drew it.
  const width = /max-width:\s*([\d.]+)px/.exec(svg)?.[1];

  return (
    <div className="flex justify-center rounded-lg border border-current/10 p-3">
      <img
        alt="Diagram"
        className="max-w-full"
        style={{ width: width && `${width}px` }}
        src={`data:image/svg+xml;charset=utf-8,${encodeURIComponent(svg)}`}
      />
    </div>
  );
}

import { useEffect, useState } from 'react';
import { CodeBlock } from '@/components/code-block';
import { Skeleton } from '@/components/skeleton';
import { useResolvedTheme } from '@/lib/use-resolved-theme';

let next = 0;

// MermaidBlock draws a ```mermaid fence, loading mermaid on the first diagram. The SVG is shown
// as an image, so nothing in it can run or restyle the page.
export function MermaidBlock({ code }: { code: string }) {
  const theme = useResolvedTheme();
  const [out, setOut] = useState<{ key: string; svg?: string; error?: string } | null>(null);
  const key = `${theme}\n${code}`;

  useEffect(() => {
    let live = true;
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
        if (live) setOut({ key, svg });
      })
      .catch((e: unknown) => {
        if (live) setOut({ key, error: e instanceof Error ? e.message : String(e) });
      });
    return () => {
      live = false;
    };
  }, [code, theme, key]);

  if (out?.key !== key) return <Skeleton className="h-32" />;

  if (out.error !== undefined) {
    return (
      <div className="flex flex-col gap-1">
        <CodeBlock code={code} lang="mermaid" />
        <p className="whitespace-pre-wrap font-mono text-[11.5px] text-muted">{out.error}</p>
      </div>
    );
  }

  // The SVG fills its box; its own max-width keeps it at the size mermaid drew it.
  const width = /max-width:\s*([\d.]+)px/.exec(out.svg ?? '')?.[1];

  return (
    <div className="flex justify-center rounded-lg border border-current/10 p-3">
      <img
        alt="Diagram"
        className="max-w-full"
        style={{ width: width && `${width}px` }}
        src={`data:image/svg+xml;charset=utf-8,${encodeURIComponent(out.svg ?? '')}`}
      />
    </div>
  );
}

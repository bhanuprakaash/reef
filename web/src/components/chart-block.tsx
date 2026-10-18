import { useEffect, useMemo, useRef, useState } from 'react';
import type { Chart } from 'chart.js';
import { CodeBlock } from '@/components/code-block';
import { Skeleton } from '@/components/skeleton';
import { useResolvedTheme } from '@/lib/use-resolved-theme';
import { parseChart } from '@/utils/chart-spec';

const palette = ['working', 'done', 'lead', 'architect', 'paused', 'failed', 'developer'];

// ChartBlock draws a ```chart fence with Chart.js, loaded on the first chart. Canvas cannot read
// CSS variables, so colours are read from the theme when drawn and again when the theme changes.
export function ChartBlock({ code }: { code: string }) {
  const parsed = useMemo(() => parseChart(code), [code]);
  const [last, setLast] = useState(parsed);
  if (parsed && parsed !== last) setLast(parsed);
  const spec = parsed ?? last;
  // A streaming reply is not valid JSON until it ends: call it invalid only once it stops changing.
  const [settled, setSettled] = useState<string | null>(null);
  const theme = useResolvedTheme();
  const canvas = useRef<HTMLCanvasElement>(null);

  useEffect(() => {
    if (parsed) return;
    const timer = setTimeout(() => setSettled(code), 600);
    return () => clearTimeout(timer);
  }, [code, parsed]);

  useEffect(() => {
    if (!spec) return;
    let live = true;
    let chart: Chart | undefined;
    void import('@/lib/chart').then(({ Chart: ChartJS }) => {
      if (!live || !canvas.current) return;
      const css = getComputedStyle(document.documentElement);
      const token = (name: string) => css.getPropertyValue(`--color-${name}`).trim();
      const colours = palette.map(token);
      const round = spec.type === 'pie' || spec.type === 'doughnut';
      ChartJS.defaults.font.family = css.getPropertyValue('--font-sans');
      ChartJS.defaults.color = token('muted');
      ChartJS.defaults.borderColor = token('line');
      chart = new ChartJS(canvas.current, {
        type: spec.type,
        data: {
          labels: spec.labels,
          datasets: spec.datasets.map((set, i) => ({
            label: set.label,
            data: set.data,
            backgroundColor: round
              ? set.data.map((_, j) => colours[j % colours.length])
              : colours[i % colours.length],
            borderColor: round ? token('main') : colours[i % colours.length],
          })),
        },
        options: {
          responsive: true,
          maintainAspectRatio: false,
          plugins: {
            title: { display: !!spec.title, text: spec.title, color: token('fg') },
            legend: { display: round || spec.datasets.some((set) => set.label) },
          },
        },
      });
    });
    return () => {
      live = false;
      chart?.destroy();
    };
  }, [spec, theme]);

  if (!parsed && settled === code) {
    return (
      <div className="flex flex-col gap-1">
        <CodeBlock code={code} lang="chart" />
        <p className="text-[12px] text-muted">Not a valid chart</p>
      </div>
    );
  }

  if (!spec) return <Skeleton className="h-64" />;

  return (
    <figure className="rounded-lg border border-current/10 p-3">
      <div className="relative h-64">
        <canvas ref={canvas} role="img" aria-label={spec.title ?? `${spec.type} chart`} />
      </div>
      <table className="sr-only">
        <thead>
          <tr>
            <th />
            {spec.datasets.map((set, i) => (
              <th key={i}>{set.label ?? `Series ${i + 1}`}</th>
            ))}
          </tr>
        </thead>
        <tbody>
          {spec.labels.map((label, row) => (
            <tr key={row}>
              <th>{label}</th>
              {spec.datasets.map((set, i) => (
                <td key={i}>{set.data[row]}</td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </figure>
  );
}

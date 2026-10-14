import type { DiffLine, FileDiff } from '@/types/api';
import { mutedClass } from '@/utils/classes';

const lineClass: Record<DiffLine['kind'], string> = {
  add: 'bg-green-50 text-green-900 dark:bg-green-950 dark:text-green-200',
  del: 'bg-red-50 text-red-900 dark:bg-red-950 dark:text-red-200',
  ctx: '',
};

export function DiffPanel({ files }: { files: FileDiff[] }) {
  if (files.length === 0) return <p className={mutedClass}>No changes.</p>;

  return (
    <div className="space-y-4">
      {files.map((f) => (
        <div
          key={f.path}
          className="overflow-x-auto rounded border border-neutral-300 dark:border-neutral-700"
        >
          <p className="border-b border-neutral-300 px-2 py-1 text-sm font-medium dark:border-neutral-700">
            {f.path} <span className={mutedClass}>{f.status}</span>
          </p>
          <pre className="font-mono text-xs">
            {f.hunks.map((h) => (
              <div key={h.header}>
                <div className={mutedClass + ' px-2'}>{h.header}</div>
                {h.lines.map((l, i) => (
                  <div
                    key={i}
                    data-path={f.path}
                    data-line={l.new}
                    className={`flex gap-2 px-2 ${lineClass[l.kind]}`}
                  >
                    <span className="w-4 select-none">
                      {l.kind === 'add' ? '+' : l.kind === 'del' ? '-' : ' '}
                    </span>
                    <span className="whitespace-pre-wrap">{l.text}</span>
                  </div>
                ))}
              </div>
            ))}
          </pre>
        </div>
      ))}
    </div>
  );
}

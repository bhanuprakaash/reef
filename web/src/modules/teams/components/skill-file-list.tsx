import type { SkillFile } from '@/types/api';

export function SkillFileList({ files }: { files: SkillFile[] }) {
  return (
    <ul className="mt-2 flex flex-col gap-1">
      {files.map((f) => (
        <li key={f.path}>
          <details>
            <summary className="cursor-pointer font-mono text-xs text-soft">{f.path}</summary>
            <pre className="mt-1 max-h-60 overflow-auto rounded-field bg-field p-2 text-xs whitespace-pre-wrap">
              {f.body}
            </pre>
          </details>
        </li>
      ))}
    </ul>
  );
}

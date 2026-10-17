import { useEffect, useRef, useState } from 'react';
import { Button } from '@/components/button';
import { CopyButton } from '@/components/copy-button';
import { Skeleton } from '@/components/skeleton';
import { ApiError } from '@/lib/api-client';
import { highlight, type Line } from '@/lib/highlight';
import { useFile } from '@/modules/channels/api/get-file';
import { fileLanguage } from '@/utils/file-ref';

function bytes(n: number) {
  return n >= 1024 * 1024 ? `${(n / 1024 / 1024).toFixed(1)} MiB` : `${Math.ceil(n / 1024)} KiB`;
}

function errorText(error: Error) {
  if (error instanceof ApiError && error.status === 404) {
    return "Not found in this channel's working folder";
  }
  if (error instanceof ApiError && error.status === 415) return 'Binary file';
  return error.message;
}

function Code({ code, lang, line }: { code: string; lang: string; line?: number }) {
  const [lines, setLines] = useState<{ code: string; lines: Line[] } | null>(null);
  const target = useRef<HTMLDivElement>(null);

  useEffect(() => {
    let live = true;
    highlight(code, lang)
      .then((result) => live && result && setLines({ code, lines: result }))
      .catch(() => undefined);
    return () => {
      live = false;
    };
  }, [code, lang]);

  useEffect(() => {
    target.current?.scrollIntoView({ block: 'center' });
  }, [code, line]);

  const done = lines?.code === code ? lines.lines : null;
  const rows: Line[] =
    done ?? code.split('\n').map((text) => [{ content: text, style: undefined }]);

  return (
    <pre className="min-w-max py-2 font-mono text-[12.5px] leading-[1.55]">
      <code>
        {rows.map((row, i) => (
          <div
            key={i}
            ref={i + 1 === line ? target : undefined}
            className={`flex min-h-[1.55em] ${i + 1 === line ? 'bg-current/[0.1]' : ''}`}
          >
            <span className="w-12 shrink-0 select-none pr-3 text-right text-faint">{i + 1}</span>
            <span className="pr-4">
              {row.map((token, j) => (
                <span key={j} className="shiki-token" style={token.style}>
                  {token.content}
                </span>
              ))}
            </span>
          </div>
        ))}
      </code>
    </pre>
  );
}

export function FilePanel({
  channelId,
  path,
  line,
  onClose,
}: {
  channelId: number;
  path: string;
  line?: number;
  onClose: () => void;
}) {
  const file = useFile(channelId, path);
  const shown = file.data?.path ?? path;

  return (
    <aside aria-label="File" className="flex min-h-0 flex-1 flex-col bg-sidebar max-md:bg-main">
      <div className="flex min-h-14 items-center gap-2 border-b border-hairline py-2 pl-[18px] pr-3">
        <h2 dir="rtl" title={shown} className="min-w-0 truncate text-left font-mono text-[13px]">
          <bdi dir="ltr">{shown}</bdi>
        </h2>
        <span className="ml-auto shrink-0">
          <CopyButton text={shown} label="Copy path" />
        </span>
        <Button variant="ghost" size="icon" type="button" aria-label="Close file" onClick={onClose}>
          <svg
            width="16"
            height="16"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="2"
            strokeLinecap="round"
            aria-hidden="true"
          >
            <path d="M6 6l12 12M18 6L6 18" />
          </svg>
        </Button>
      </div>
      {file.isPending && (
        <div className="space-y-2 p-[18px]">
          <Skeleton className="h-4 w-3/4" />
          <Skeleton className="h-4 w-1/2" />
          <Skeleton className="h-4 w-2/3" />
        </div>
      )}
      {file.isError && <p className="p-[18px] text-sm text-muted">{errorText(file.error)}</p>}
      {file.data?.truncated && (
        <p className="border-b border-hairline px-[18px] py-2 text-xs text-muted">
          Showing the first 512 KiB of {bytes(file.data.size)}
        </p>
      )}
      {file.data && (
        <div className="min-h-0 flex-1 overflow-auto">
          <Code
            code={file.data.content.replace(/\n$/, '')}
            lang={fileLanguage(file.data.path)}
            line={line}
          />
        </div>
      )}
    </aside>
  );
}

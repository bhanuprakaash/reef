import { Button } from '@/components/button';

export function ClipboardChip({ path, onPaste }: { path: string; onPaste: () => void }) {
  const name = path.split('/').pop() ?? path;
  return (
    <Button
      type="button"
      size="sm"
      className="max-w-full"
      aria-label={`Paste file from clipboard: ${name}`}
      onClick={onPaste}
    >
      <span className="text-muted">File in clipboard</span>
      <span className="min-w-0 truncate">{name}</span>
      <span className="text-mention">Paste</span>
    </Button>
  );
}

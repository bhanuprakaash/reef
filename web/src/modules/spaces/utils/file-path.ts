import type { ReefBridge } from '@/types/desktop';

export function insertFiles(text: string, caret: number, files: { path: string; name: string }[]) {
  const piece = files
    .map((f) => f.path || f.name)
    .map((p) => (/\s/.test(p) ? `"${p}"` : p))
    .join(' ');
  const before = text.slice(0, caret);
  const gap = before === '' || /\s$/.test(before) ? '' : ' ';
  const head = `${before}${gap}${piece} `;
  return {
    text: head + text.slice(caret),
    caret: head.length,
    nameOnly: files.some((f) => !f.path),
  };
}

export function withPaths(files: File[], bridge: Pick<ReefBridge, 'pathForFile'> | undefined) {
  return files.map((file) => ({ name: file.name, path: bridge?.pathForFile(file) ?? '' }));
}

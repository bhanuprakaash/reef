import { createContext, useContext, type ComponentProps } from 'react';
import type { Link, Parent, PhrasingContent, Root, RootContent } from 'mdast';
import type { ExtraProps } from 'react-markdown';
import { findFileRefs, parseCodeRef, parseFileRef, type FileRef } from '@/utils/file-ref';

export const FileLinkContext = createContext<((path: string, line?: number) => void) | null>(null);

function fileLink(ref: FileRef, children: PhrasingContent[], kind: string): Link {
  return {
    type: 'link',
    url: '',
    children,
    data: {
      hName: 'button',
      hProperties: { dataPath: ref.path, dataLine: ref.line, dataKind: kind },
    },
  };
}

function link(node: RootContent): RootContent[] {
  if (node.type === 'inlineCode') {
    const ref = parseCodeRef(node.value);
    return ref ? [fileLink(ref, [node], 'code')] : [node];
  }
  if (node.type === 'link') {
    const ref = parseFileRef(node.url);
    return ref ? [fileLink(ref, node.children, 'link')] : [node];
  }
  if (node.type === 'text' && !node.data) {
    const pieces: PhrasingContent[] = [];
    let last = 0;
    for (const ref of findFileRefs(node.value)) {
      if (ref.start > last) pieces.push({ type: 'text', value: node.value.slice(last, ref.start) });
      pieces.push(
        fileLink(ref, [{ type: 'text', value: node.value.slice(ref.start, ref.end) }], 'text'),
      );
      last = ref.end;
    }
    if (pieces.length === 0) return [node];
    if (last < node.value.length) pieces.push({ type: 'text', value: node.value.slice(last) });
    return pieces;
  }
  if ('children' in node && node.type !== 'linkReference') {
    (node as Parent).children = (node as Parent).children.flatMap(link);
  }
  return [node];
}

export function remarkFileLinks() {
  return (tree: Root) => {
    tree.children = tree.children.flatMap(link);
  };
}

const kinds: Record<string, string> = {
  code: '',
  link: 'underline hover:decoration-current',
  text: 'font-mono text-[0.9em]',
};

type FileLinkProps = ComponentProps<'button'> &
  ExtraProps & { 'data-path'?: string; 'data-line'?: string; 'data-kind'?: string };

export function FileLink({ children, ...props }: FileLinkProps) {
  const open = useContext(FileLinkContext);
  const path = props['data-path'] ?? '';
  const line = props['data-line'];
  return (
    <button
      type="button"
      title={`Open ${path}`}
      className={`cursor-pointer text-left wrap-anywhere decoration-current/40 underline-offset-2 hover:underline ${kinds[props['data-kind'] ?? 'text']}`}
      onClick={() => open?.(path, line ? Number(line) : undefined)}
    >
      {children}
    </button>
  );
}

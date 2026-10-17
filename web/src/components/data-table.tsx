import {
  Children,
  cloneElement,
  isValidElement,
  useState,
  type ComponentProps,
  type MouseEvent,
  type ReactElement,
  type ReactNode,
} from 'react';
import type { Element, ElementContent } from 'hast';
import { CopyButton } from '@/components/copy-button';

type Sort = { col: number; dir: 'asc' | 'desc' } | null;

function text(node: ElementContent): string {
  if (node.type === 'text') return node.value;
  if (node.type === 'element') return node.children.map(text).join('');
  return '';
}

function rowsOf(table: Element, section: 'thead' | 'tbody'): string[][] {
  const part = table.children.find(
    (c): c is Element => c.type === 'element' && c.tagName === section,
  );
  return (part?.children ?? [])
    .filter((c): c is Element => c.type === 'element' && c.tagName === 'tr')
    .map((tr) =>
      tr.children
        .filter((c): c is Element => c.type === 'element')
        .map((cell) => text(cell).trim()),
    );
}

const figure = /^[\s$€£+-]*(\d[\d,]*(?:\.\d+)?)\s*(?:%|ms|s|x|k|m|kb|mb|gb)?$/i;

function compare(a: string, b: string) {
  const x = figure.exec(a);
  const y = figure.exec(b);
  if (x && y) return Number(x[1].replaceAll(',', '')) - Number(y[1].replaceAll(',', ''));
  return a.localeCompare(b, undefined, { numeric: true, sensitivity: 'base' });
}

function csv(rows: string[][]) {
  return rows
    .map((row) => row.map((cell) => `"${cell.replaceAll('"', '""')}"`).join(','))
    .join('\n');
}

function mapChildren(node: ReactNode, fn: (child: ReactElement, index: number) => ReactNode) {
  return Children.toArray(node).map((child, i) => (isValidElement(child) ? fn(child, i) : child));
}

// DataTable is the markdown `table`: click a header to sort its rows, copy the whole table as
// CSV. Rows are reordered from the parsed tree, so cells keep their links, code and mentions.
export function DataTable({ node, children }: ComponentProps<'table'> & { node?: Element }) {
  const [sort, setSort] = useState<Sort>(null);
  const head = node ? rowsOf(node, 'thead') : [];
  const body = node ? rowsOf(node, 'tbody') : [];

  const order = body.map((_, i) => i);
  if (sort) {
    order.sort((a, b) => {
      const diff = compare(body[a][sort.col] ?? '', body[b][sort.col] ?? '');
      return sort.dir === 'asc' ? diff : -diff;
    });
  }

  const onHeaderClick = (e: MouseEvent) => {
    const th = (e.target as HTMLElement).closest('th');
    if (!th) return;
    const col = th.cellIndex;
    setSort((prev) =>
      prev?.col !== col ? { col, dir: 'asc' } : prev.dir === 'asc' ? { col, dir: 'desc' } : null,
    );
  };

  const sections = mapChildren(children, (section) => {
    if (section.type === 'thead') {
      return cloneElement(
        section,
        {},
        mapChildren((section.props as { children?: ReactNode }).children, (tr) =>
          cloneElement(
            tr,
            {},
            mapChildren((tr.props as { children?: ReactNode }).children, (th, col) =>
              cloneElement(th as ReactElement<Record<string, unknown>>, {
                'data-numeric': body.length > 0 && body.every((row) => figure.test(row[col] ?? '')),
                'aria-sort':
                  sort?.col === col ? (sort.dir === 'asc' ? 'ascending' : 'descending') : undefined,
              }),
            ),
          ),
        ),
      );
    }
    const rows = Children.toArray((section.props as { children?: ReactNode }).children);
    return cloneElement(
      section,
      {},
      order.map((i) => rows[i]),
    );
  });

  return (
    <div className="my-0.5 overflow-hidden rounded-lg border border-current/15">
      <div className="flex justify-end border-b border-current/10 px-1.5 py-0.5">
        <CopyButton text={csv([...head, ...body])} label="Copy CSV" />
      </div>
      <div className="overflow-x-auto">
        <table onClick={onHeaderClick}>{sections}</table>
      </div>
    </div>
  );
}

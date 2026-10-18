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
import { csv, numericColumn, sortOrder } from '@/utils/table-data';

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

function mapChildren(node: ReactNode, fn: (child: ReactElement, index: number) => ReactNode) {
  return Children.toArray(node).map((child, i) => (isValidElement(child) ? fn(child, i) : child));
}

export function DataTable({ node, children }: ComponentProps<'table'> & { node?: Element }) {
  const [sort, setSort] = useState<Sort>(null);
  const head = node ? rowsOf(node, 'thead') : [];
  const body = node ? rowsOf(node, 'tbody') : [];

  const order = sort ? sortOrder(body, sort.col, sort.dir) : body.map((_, i) => i);
  const numeric = (head[0] ?? []).map((_, col) => numericColumn(body, col));

  const onHeaderClick = (e: MouseEvent) => {
    const target = e.target as HTMLElement;
    const th = target.closest('th');
    if (!th || target.closest('button, a')) return;
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
                'data-numeric': numeric[col],
                'aria-sort':
                  sort?.col === col ? (sort.dir === 'asc' ? 'ascending' : 'descending') : undefined,
              }),
            ),
          ),
        ),
      );
    }
    const rows = mapChildren((section.props as { children?: ReactNode }).children, (tr) =>
      cloneElement(
        tr,
        {},
        mapChildren((tr.props as { children?: ReactNode }).children, (td, col) =>
          cloneElement(td as ReactElement<Record<string, unknown>>, {
            'data-numeric': numeric[col],
          }),
        ),
      ),
    );
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

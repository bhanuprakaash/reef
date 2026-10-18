const plainNumber = /^(?:([-−+]?)[$€£]?|[$€£]([-−+]))(\d{1,3}(?:,\d{3})+|\d+)(\.\d+)?%?$/;

function numberOf(cell: string): number | null {
  const found = plainNumber.exec(cell);
  if (!found) return null;
  const [, before, after, whole, fraction = ''] = found;
  const sign = before || after;
  const value = Number(whole.replaceAll(',', '') + fraction);
  return sign === '-' || sign === '−' ? -value : value;
}

export function numericColumn(body: string[][], col: number): boolean {
  return body.length > 0 && body.every((row) => numberOf(row[col] ?? '') !== null);
}

export function sortOrder(body: string[][], col: number, dir: 'asc' | 'desc'): number[] {
  const cells = body.map((row) => row[col] ?? '');
  const values = numericColumn(body, col) ? cells.map((cell) => numberOf(cell) ?? 0) : null;
  const order = body.map((_, i) => i);
  order.sort((a, b) => {
    const diff = values
      ? values[a] - values[b]
      : cells[a].localeCompare(cells[b], undefined, { numeric: true, sensitivity: 'base' });
    return dir === 'asc' ? diff : -diff;
  });
  return order;
}

// A spreadsheet runs a cell that starts with one of these as a formula.
const formula = /^[=+\-@\t\r]/;

export function csv(rows: string[][]): string {
  return rows
    .map((row) =>
      row
        .map((cell) => (formula.test(cell) && numberOf(cell) === null ? `'${cell}` : cell))
        .map((cell) => `"${cell.replaceAll('"', '""')}"`)
        .join(','),
    )
    .join('\n');
}

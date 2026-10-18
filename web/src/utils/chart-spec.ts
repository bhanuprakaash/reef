export type ChartSpec = {
  type: 'bar' | 'line' | 'pie' | 'doughnut';
  title?: string;
  labels: string[];
  datasets: { label?: string; data: number[] }[];
};

const types = ['bar', 'line', 'pie', 'doughnut'];

function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function optionalString(value: unknown): value is string | undefined {
  return value === undefined || typeof value === 'string';
}

// parseChart reads a ```chart fence. It returns null for anything that is not a complete, valid
// spec, including half-written JSON from a reply still streaming.
export function parseChart(text: string): ChartSpec | null {
  let value: unknown;
  try {
    value = JSON.parse(text);
  } catch {
    return null;
  }
  if (!isObject(value)) return null;
  const { type, title, labels, datasets } = value;
  if (typeof type !== 'string' || !types.includes(type) || !optionalString(title)) return null;
  if (!Array.isArray(labels) || labels.length === 0) return null;
  if (!labels.every((label) => typeof label === 'string')) return null;
  if (!Array.isArray(datasets) || datasets.length === 0) return null;
  const valid = datasets.every(
    (set) =>
      isObject(set) &&
      optionalString(set.label) &&
      Array.isArray(set.data) &&
      set.data.length === labels.length &&
      set.data.every((n) => typeof n === 'number' && Number.isFinite(n)),
  );
  if (!valid) return null;
  return {
    type: type as ChartSpec['type'],
    title,
    labels,
    datasets: datasets.map((set) => ({ label: set.label, data: set.data })),
  };
}

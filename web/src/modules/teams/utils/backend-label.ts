const labels: Record<string, string> = { claude: 'Claude Code', codex: 'Codex' };

export function backendLabel(backend: string) {
  return labels[backend] ?? backend;
}

export const backendOptions = Object.keys(labels).map((value) => ({
  value,
  label: backendLabel(value),
}));

const labels: Record<string, string> = { claude: 'Claude Code', codex: 'Codex' };

export function backendLabel(backend: string) {
  return labels[backend] ?? backend;
}

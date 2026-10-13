export const BACKENDS = ['claude', 'codex'] as const;

export const PERMISSION_MODES: Record<string, string[]> = {
  claude: ['default', 'acceptEdits'],
  codex: ['read-only', 'workspace-write'],
};

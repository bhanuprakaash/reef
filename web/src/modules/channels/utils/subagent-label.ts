export function subagentLabel(name: string, input: unknown): string | undefined {
  if (name !== 'Agent' || typeof input !== 'object' || input === null) return undefined;
  const { subagent_type: type, description } = input as Record<string, unknown>;
  const parts = [type, description].filter((v): v is string => typeof v === 'string' && v !== '');
  return parts.length > 0 ? parts.join(' · ') : undefined;
}

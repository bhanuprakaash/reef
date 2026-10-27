export function blocksUser(reason: string): boolean {
  return reason !== '' && !reason.startsWith('hop cap:');
}

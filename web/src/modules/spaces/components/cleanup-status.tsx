export type CleanupState = { state: string; snapshotRef: string; cleanupError: string };

// What happened when Reef removed a merged or abandoned workspace: why it could not yet (it
// tries again on the next read of the requests), or where the unfinished changes were saved.
export function CleanupStatus({ w }: { w: CleanupState | undefined }) {
  if (!w) return null;
  if (w.cleanupError !== '') {
    return <p className="text-[13px] text-danger">Cleanup failed: {w.cleanupError}. Retrying.</p>;
  }
  if (w.state !== 'removed') return null;
  return (
    <p className="text-[13px] text-muted">
      Workspace removed.
      {w.snapshotRef !== '' && (
        <>
          {' '}
          Unfinished changes saved at <code className="font-mono">{w.snapshotRef}</code>.
        </>
      )}
    </p>
  );
}

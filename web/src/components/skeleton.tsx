export function Skeleton({ className }: { className?: string }) {
  return (
    <div
      aria-hidden="true"
      className={`animate-pulse rounded-menu bg-card motion-reduce:animate-none ${className ?? ''}`}
    />
  );
}

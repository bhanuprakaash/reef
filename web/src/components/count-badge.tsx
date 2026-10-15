export function CountBadge({ count, label }: { count: number; label: string }) {
  return (
    <span
      aria-label={label}
      className="inline-flex h-5 min-w-5 items-center justify-center rounded-pill bg-needs-you px-1.5 text-xs text-white"
    >
      {count}
    </span>
  );
}

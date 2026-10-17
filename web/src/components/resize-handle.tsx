export function ResizeHandle({
  label,
  onPointerDown,
  className = '',
}: {
  label: string;
  onPointerDown: (e: React.PointerEvent<HTMLDivElement>) => void;
  className?: string;
}) {
  return (
    <div
      role="separator"
      aria-orientation="vertical"
      aria-label={label}
      onPointerDown={onPointerDown}
      className={`group absolute inset-y-0 -left-1 z-30 w-2 cursor-col-resize touch-none ${className}`}
    >
      <div className="mx-auto h-full w-0.5 bg-transparent group-hover:bg-line group-active:bg-line" />
    </div>
  );
}

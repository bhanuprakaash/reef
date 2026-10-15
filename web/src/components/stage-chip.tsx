export function StageChip({ label, color }: { label: string; color: string }) {
  return (
    <span
      style={{
        color,
        borderColor: `color-mix(in srgb, ${color} 35%, transparent)`,
        background: `color-mix(in srgb, ${color} 10%, transparent)`,
      }}
      className="flex-none rounded-pill border px-2 text-[11px] leading-[18px]"
    >
      {label}
    </span>
  );
}

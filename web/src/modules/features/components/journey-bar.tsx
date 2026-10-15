import { STAGES } from '@/modules/features/utils/stages';

export function JourneyBar({ stage }: { stage: string }) {
  const current = STAGES.findIndex((s) => s.value === stage);
  return (
    <ol aria-label="Journey" className="grid grid-cols-8 gap-1.5">
      {STAGES.map((s, i) => (
        <li key={s.value} aria-current={i === current ? 'step' : undefined}>
          <div
            className={`h-1 rounded-pill ${i === current ? 'bg-lead shadow-[0_0_12px_color-mix(in_srgb,var(--color-lead)_60%,transparent)]' : i < current ? 'bg-pill' : 'bg-track'}`}
          />
          <div className={`mt-2 text-xs ${i === current ? 'text-fg' : 'text-muted'}`}>
            {s.label}
          </div>
        </li>
      ))}
    </ol>
  );
}

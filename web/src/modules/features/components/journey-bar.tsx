import { STAGES } from '@/modules/features/utils/stages';

export function JourneyBar({ stage }: { stage: string }) {
  const current = STAGES.findIndex((s) => s.value === stage);
  return (
    <ol aria-label="Journey" className="grid grid-cols-8 gap-1.5">
      {STAGES.map((s, i) => (
        <li key={s.value} aria-current={i === current ? 'step' : undefined}>
          <div
            className={`h-1 rounded-pill ${i === current ? 'bg-lead shadow-[0_0_12px_rgb(242_106_46/0.6)]' : i < current ? 'bg-pill' : 'bg-track'}`}
          />
          <div className={`mt-2 text-xs ${i === current ? 'font-semibold text-fg' : 'text-muted'}`}>
            {s.label}
          </div>
        </li>
      ))}
    </ol>
  );
}

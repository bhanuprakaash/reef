type Props<T extends string> = {
  label: string;
  options: { value: T; label: string }[];
  value: T;
  onChange: (value: T) => void;
};

export function SegmentedControl<T extends string>({ label, options, value, onChange }: Props<T>) {
  return (
    <div role="group" aria-label={label} className="inline-flex rounded-field bg-card p-[3px]">
      {options.map((o) => (
        <button
          key={o.value}
          type="button"
          aria-pressed={o.value === value}
          onClick={() => onChange(o.value)}
          className={`h-11 md:h-[30px] rounded-lg px-3.5 text-[13px] ${o.value === value ? 'bg-raised text-fg' : 'text-muted'}`}
        >
          {o.label}
        </button>
      ))}
    </div>
  );
}

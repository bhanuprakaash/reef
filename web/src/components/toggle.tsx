type Props = { checked: boolean; onChange: (checked: boolean) => void; label: string };

export function Toggle({ checked, onChange, label }: Props) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={label}
      onClick={() => onChange(!checked)}
      className="-my-2.5 flex h-11 shrink-0 items-center"
    >
      <span className={`relative h-6 w-10 rounded-pill ${checked ? 'bg-done' : 'bg-line'}`}>
        <span
          className={`absolute top-[3px] size-[18px] rounded-full transition-[left] ${checked ? 'left-[19px] bg-fg' : 'left-[3px] bg-soft'}`}
        />
      </span>
    </button>
  );
}

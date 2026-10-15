const roles = ['Developer', 'Designer', 'Product', 'Founder', 'Student', 'Other'];
const styles = [
  'Short answers',
  'Detailed explanations',
  'Ask before big changes',
  'Just go ahead',
  'Explain decisions',
  'Tests first',
];

type ChipGroupProps = {
  label: string;
  options: string[];
  selected: string[];
  onToggle: (option: string) => void;
};

function ChipGroup({ label, options, selected, onToggle }: ChipGroupProps) {
  return (
    <div className="space-y-1.5">
      <span className="block text-[12.5px] text-muted">{label}</span>
      <div role="group" aria-label={label} className="flex flex-wrap gap-2">
        {options.map((option) => {
          const on = selected.includes(option);
          return (
            <button
              key={option}
              type="button"
              aria-pressed={on}
              onClick={() => onToggle(option)}
              className={`h-8 rounded-pill border px-3 text-[13px] ${on ? 'border-fg bg-raised text-fg' : 'border-line text-soft hover:bg-card'}`}
            >
              {option}
            </button>
          );
        })}
      </div>
    </div>
  );
}

type ProfileFieldsProps = {
  role: string;
  style: string[];
  onRole: (role: string) => void;
  onStyle: (style: string[]) => void;
};

export function ProfileFields({ role, style, onRole, onStyle }: ProfileFieldsProps) {
  return (
    <>
      <ChipGroup
        label="You are"
        options={roles}
        selected={[role]}
        onToggle={(option) => onRole(option === role ? '' : option)}
      />
      <ChipGroup
        label="You like"
        options={styles}
        selected={style}
        onToggle={(option) =>
          onStyle(style.includes(option) ? style.filter((s) => s !== option) : [...style, option])
        }
      />
    </>
  );
}

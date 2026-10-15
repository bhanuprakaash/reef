import type { SkillInfo } from '@/types/api';
import { ChipList } from '@/modules/teams/components/chip-list';
import { addableSkills, isMissing } from '@/modules/teams/utils/skills';
import { Select } from '@/components/select';

type Props = {
  label: string;
  picked: string[];
  available: SkillInfo[] | undefined;
  noProject?: boolean;
  onChange: (skills: string[]) => void;
};

export function SkillPicker({ label, picked, available, noProject, onChange }: Props) {
  const addable = addableSkills(available, picked);
  return (
    <div className="min-w-0">
      <ChipList
        label="Skills"
        mono
        empty="None picked: every skill is available."
        values={picked}
        missing={(name) => isMissing(available, name)}
        onRemove={(i) => onChange(picked.filter((_, j) => j !== i))}
      />
      <Select
        className="mt-2"
        variant="pill"
        aria-label={`${label} add skill`}
        value=""
        disabled={addable.length === 0}
        options={[
          { value: '', label: 'Add a skill' },
          ...addable.map((name) => ({
            value: name,
            label: name,
            hint: available?.find((s) => s.name === name)?.description,
          })),
        ]}
        onChange={(name) => name && onChange([...picked, name])}
      />
      {picked.length > 0 && (
        <p className="mt-2 text-xs text-muted">Only these skills are available.</p>
      )}
      {noProject && <p className="mt-2 text-xs text-muted">Add a project to see its skills.</p>}
    </div>
  );
}

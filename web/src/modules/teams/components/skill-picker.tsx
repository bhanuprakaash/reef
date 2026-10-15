import type { SkillInfo } from '@/types/api';
import { ChipList } from '@/modules/teams/components/chip-list';
import { addableSkills, isMissing } from '@/modules/teams/utils/skills';
import { Select } from '@/components/select';

type Props = {
  label: string;
  picked: string[];
  available: SkillInfo[] | undefined;
  onChange: (skills: string[]) => void;
};

export function SkillPicker({ label, picked, available, onChange }: Props) {
  const addable = addableSkills(available, picked);
  return (
    <div className="min-w-0">
      <ChipList
        label="Skills"
        mono
        empty="None picked."
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
    </div>
  );
}

import type { SkillInfo } from '@/types/api';
import { ChipList } from '@/modules/teams/components/chip-list';
import {
  addableSkills,
  incompatibility,
  isMissing,
  sourceLabel,
} from '@/modules/teams/utils/skills';
import { Select } from '@/components/select';

type Props = {
  label: string;
  picked: string[];
  available: SkillInfo[] | undefined;
  backend?: string;
  noProject?: boolean;
  limits?: boolean;
  onChange: (skills: string[]) => void;
};

export function SkillPicker({
  label,
  picked,
  available,
  backend,
  noProject,
  limits,
  onChange,
}: Props) {
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
          ...addable.map((s) => ({
            value: s.ref,
            label: `${s.name} · ${sourceLabel(s.source)}${incompatibility(s, backend) ? ' · not usable here' : ''}`,
            hint: incompatibility(s, backend) || s.description,
          })),
        ]}
        onChange={(name) => name && onChange([...picked, name])}
      />
      {picked.map((ref) => {
        const reason = incompatibility(
          available?.find((s) => s.ref === ref),
          backend,
        );
        return (
          reason && (
            <p key={ref} className="mt-2 text-xs text-danger">
              {ref} is not usable on {backend}: {reason}
            </p>
          )
        );
      })}
      {limits && picked.length > 0 && (
        <p className="mt-2 text-xs text-muted">Only these skills are available.</p>
      )}
      {noProject && <p className="mt-2 text-xs text-muted">Pick a project to see its skills.</p>}
    </div>
  );
}

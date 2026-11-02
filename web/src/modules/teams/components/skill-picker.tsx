import type { SkillInfo } from '@/types/api';
import { ChipList } from '@/modules/teams/components/chip-list';
import { incompatibility, isMissing, sourceLabel } from '@/modules/teams/utils/skills';
import { SearchPicker } from '@/components/search-picker';

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
      <SearchPicker
        label="Add skill"
        aria-label={`${label} add skill`}
        options={(available ?? []).map((s) => ({
          value: s.ref,
          label: `${s.name} · ${sourceLabel(s.source)}${incompatibility(s, backend) ? ' · not usable here' : ''}`,
          hint: incompatibility(s, backend) || s.description,
        }))}
        selected={picked}
        disabled={!available?.length}
        onToggle={(ref) =>
          onChange(picked.includes(ref) ? picked.filter((r) => r !== ref) : [...picked, ref])
        }
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
      {noProject && (
        <p className="mt-2 text-xs text-muted">Pick a project to also see its skills.</p>
      )}
    </div>
  );
}

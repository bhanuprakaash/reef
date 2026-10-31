import {
  useRejectSkillDraft,
  useSaveSkillDraft,
  useSkillLibrary,
} from '@/modules/teams/api/skill-library';
import { SkillFileList } from '@/modules/teams/components/skill-file-list';
import type { SkillDraft } from '@/types/api';
import { Button } from '@/components/button';

export function SkillDraftCard({ draft }: { draft: SkillDraft }) {
  const library = useSkillLibrary();
  const save = useSaveSkillDraft();
  const reject = useRejectSkillDraft();
  const taken = library.data?.some((s) => s.name === draft.name) ?? false;
  const error = save.error ?? reject.error;
  return (
    <section
      aria-label={`Skill draft ${draft.name}`}
      className="flex flex-col gap-2 rounded-card border border-line bg-card p-4"
    >
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-[13px] text-muted">Skill draft</span>
        <span className="font-mono text-[13px]">{draft.name}</span>
        <Button
          className="ml-auto"
          size="sm"
          variant="primary"
          aria-label={`${taken ? 'Save as new version' : 'Save'} ${draft.name}`}
          loading={save.isPending}
          onClick={() => save.mutate({ id: draft.id, update: taken })}
        >
          {taken ? 'Save as new version' : 'Save to library'}
        </Button>
        <Button
          size="sm"
          aria-label={`Reject ${draft.name}`}
          loading={reject.isPending}
          onClick={() => reject.mutate(draft.id)}
        >
          Reject
        </Button>
      </div>
      <SkillFileList files={draft.files} />
      {error && <p className="text-sm text-danger">{error.message}</p>}
    </section>
  );
}

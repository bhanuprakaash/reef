import { useSkillDrafts } from '@/modules/teams/api/skill-library';
import { SkillDraftCard } from '@/modules/teams/components/skill-draft-card';

export function SkillDraftCards({ spaceId }: { spaceId: number }) {
  const drafts = useSkillDrafts(spaceId);
  return drafts.data?.map((d) => <SkillDraftCard key={d.id} draft={d} />);
}

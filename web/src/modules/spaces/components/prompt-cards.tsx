import { useAnswerPrompt, type PromptAnswer } from '@/modules/spaces/api/answer-prompt';
import { usePrompts } from '@/modules/spaces/api/get-prompts';
import type { Role } from '@/types/api';
import { Button } from '@/components/button';
import { Creature } from '@/components/creature';

const allow: [PromptAnswer, string][] = [
  ['once', 'Allow once'],
  ['always', 'Allow always'],
];

export function PromptCards({ spaceId, roles }: { spaceId: number; roles: Role[] }) {
  const prompts = usePrompts(spaceId);
  const answer = useAnswerPrompt(spaceId);
  const rows = prompts.data ?? [];

  if (rows.length === 0) return null;

  return (
    <div className="space-y-2">
      <ul className="space-y-2">
        {rows.map((p) => {
          const role = roles.find((r) => r.botId === p.botId);
          const target = p.input.command ?? p.input.file_path ?? p.input.path ?? p.path;
          return (
            <li
              key={p.id}
              className="flex flex-wrap items-center gap-3 rounded-card border border-line bg-card px-4 py-3.5 [--creature-ring:var(--color-card)]"
            >
              <Creature look={role} size={24} state="needs-you" />
              <div className="min-w-0 flex-1 basis-48">
                <p>
                  <span className="capitalize">{role?.name ?? 'A Bot'}</span>
                  {' wants to '}
                  <span className="font-medium">{p.tool}</span>
                </p>
                {target && (
                  <code className="block break-all font-mono text-[13px] text-soft">{target}</code>
                )}
                {p.status === 'expired' && (
                  <p className="text-[13px] text-muted">No answer in time — denied</p>
                )}
              </div>
              {allow.map(([value, label]) => (
                <Button
                  key={value}
                  variant="primary"
                  type="button"
                  disabled={answer.isPending}
                  onClick={() => answer.mutate({ id: p.id, answer: value })}
                >
                  {label}
                </Button>
              ))}
              {p.status === 'waiting' && (
                <Button
                  variant="secondary"
                  type="button"
                  disabled={answer.isPending}
                  onClick={() => answer.mutate({ id: p.id, answer: 'deny' })}
                >
                  Deny
                </Button>
              )}
            </li>
          );
        })}
      </ul>
      {answer.isError && <p className="text-sm text-danger">{answer.error.message}</p>}
    </div>
  );
}

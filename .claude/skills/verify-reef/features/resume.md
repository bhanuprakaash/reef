# Resume a session

A user continues an earlier session with `reef-agent --resume ID`, where ID is the
`sessionId` from that session's `session` line. The new process keeps the session's history.
A bad id fails with exit 1 and a message, not a hang.

## Sub-features

- `resume-continue` the resumed session remembers what the earlier one said.
- `resume-same-id` the resumed session reports the same `sessionId`.
- `resume-bad-id` an unknown id ends with exit 1 and an error message.
- `resume-background` a claude session resumed after a background tool task answers its own prompt in its first turn.

## How to get to it (user POV)

- `reef-agent --backend claude --resume <sessionId> "prompt"` in the same cwd as the first run.
- `reef-agent --backend codex --resume <sessionId> "prompt"`.

## Driving it with verify-reef

Preconditions:

- Baseline from the README. Both drives below run in the same run id, so they share the scratch cwd.

- **First session, claude.** Run `ANTHROPIC_MODEL=sonnet REEF_VERIFY_RUN=<id> $H run --name resume1-claude -- --backend claude "Remember the word heliotrope. Reply ok."`. `exit: 0`. Take the id: `S=$(grep -m1 '"session"' ~/.reef-verify/<id>/resume1-claude.jsonl | python3 -I -c 'import json,sys; print(json.load(sys.stdin)["sessionId"])')`.
- **Resume, claude.** Run `ANTHROPIC_MODEL=sonnet REEF_VERIFY_RUN=<id> $H run --name resume2-claude -- --backend claude --resume "$S" "What word did I ask you to remember? Answer with the word only."`. `exit: 0`. The `session` line has `sessionId` equal to `$S`. The joined `text` lines contain `heliotrope`.
- **Bad id, claude.** Run `ANTHROPIC_MODEL=sonnet REEF_VERIFY_RUN=<id> $H run --name resumebad-claude -- --backend claude --resume 00000000-0000-0000-0000-000000000000 "hi"`. `exit: 1`. The last line is `{"kind":"error","text":"No conversation found with session ID: 00000000-..."}`, with no `usage` line (the result has zero usage); there is exactly one `error` line.
- **After a background task, claude.** Run `ANTHROPIC_MODEL=sonnet REEF_VERIFY_RUN=<id> $H run --name resumebg1-claude -- --backend claude "Run this with the Bash tool with run_in_background true: sleep 25; echo bgdone. Then reply with just: started."`, take the id as above, then `ANTHROPIC_MODEL=sonnet REEF_VERIFY_RUN=<id> $H run --name resumebg2-claude -- --backend claude --resume "$S" "What is 7 times 6? Reply with just the number."`. The joined `text` lines are `42` and the one `turn_end` comes after them. An empty turn (`turn_end` with no `text` before it) is a failure.
- **Codex.** Repeat the first two steps with `--backend codex` and names `resume1-codex`, `resume2-codex`. The resumed `session` line has the same id and the reply contains `heliotrope`.
- **Bad id, codex.** Run `REEF_VERIFY_RUN=<id> $H run --name resumebad-codex -- --backend codex --resume 00000000-0000-0000-0000-000000000000 "hi"`. `exit: 1`, empty jsonl, and stderr starts with `reef-agent: thread/resume:`.

## Gotchas

- Claude stores sessions per cwd. Resuming from another cwd gives "No conversation found". Keep both drives in one run id.
- Claude starts the process before it knows the id is bad, so the failure arrives as events after the prompt is sent. Codex fails during start, so it prints nothing on stdout.
- Codex replays the last turn's usage on resume; the codex adapter drops it, so no `usage` line comes before the resumed run's first `text`.
- `cleanup` deletes the claude session folder of the scratch cwd, so a session cannot be resumed after cleanup.

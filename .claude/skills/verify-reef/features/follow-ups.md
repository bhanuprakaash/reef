# Follow-up prompts

While `reef-agent` runs, each line the user types on stdin is a follow-up prompt. It is sent
after the current turn ends, as the next turn of the same session. When stdin closes,
`reef-agent` exits 0 after the last turn ends.

## Sub-features

- `followup-turns` each non-empty stdin line gives one more turn, ending in its own `turn_end`.
- `followup-memory` a later turn sees what an earlier turn said (same session).
- `followup-wait` a line typed mid-turn waits for the `turn_end`; it does not merge into the running turn.
- `followup-eof` closing stdin ends the process with exit 0 after the last turn.
- `followup-background-task` claude: a background task that ends in the live session adds none of its own text or tool events to a turn; the turn after it shows only its own reply.

## How to get to it (user POV)

- Type a line and press Enter while `reef-agent` runs in a terminal.
- Pipe prompts in: `printf 'second\nthird\n' | reef-agent --backend codex "first"`.

## Driving it with verify-reef

Preconditions:

- Baseline from the README.
- A prompts file outside the scratch dir, e.g. `printf 'What word did I ask you to remember? Answer with the word only.\n\nNow say done.\n' > ~/.reef-verify/<id>/followups.txt` (the blank line must be skipped).

- **Three turns, claude.** Run `ANTHROPIC_MODEL=sonnet REEF_VERIFY_RUN=<id> $H run --name followups-claude --stdin ~/.reef-verify/<id>/followups.txt -- --backend claude "Remember the word heliotrope. Reply ok."`. Meta shows `exit: 0` and `turn_end=3`, `usage` at least 3 (one or more per turn), `session=3`. `grep '"session"' ~/.reef-verify/<id>/followups-claude.jsonl | sort -u | wc -l` is `1`: one session id.
- **Memory.** Join the `text` lines between the first and second `turn_end` (`python3 -I -c` over the jsonl). They contain `heliotrope`.
- **Three turns, codex.** Run `REEF_VERIFY_RUN=<id> $H run --name followups-codex --stdin ~/.reef-verify/<id>/followups.txt -- --backend codex "Remember the word heliotrope. Reply ok."`. `exit: 0`, `turn_end=3`, `usage` at least 3, `session=1`. The second turn's text contains `heliotrope`.
- **Background task, claude.** Write `~/.reef-verify/<id>/bg.txt` with two lines: `Write about 250 words on how tides work. Do not use any tools.` and `What is 7 times 6? Reply with just the number.`. Run `ANTHROPIC_MODEL=sonnet REEF_VERIFY_RUN=<id> $H run --name followups-bg-claude --stdin ~/.reef-verify/<id>/bg.txt -- --backend claude "Run this with the Bash tool with run_in_background true: sleep 6; echo bgdone. Then reply with just: started."`. `exit: 0`, `turn_end=3`, `session=4` and `usage=5`: the extra `session` and `usage` are claude's own turn for the task, which proves the task ended. Turn 2 is running when it ends (sleep 6 against a reply of about 10 s). Join the `text` lines of each turn (`python3 -I -c` over the jsonl): turn 1 is `started.`, turn 2 is the essay, turn 3 is exactly `42`. No turn's text mentions the background command or `bgdone`. With `session=3` the task did not end during the run (or the turn ran too fast): repeat once with another `sleep` value; that run does not prove the line.
- **Wait for turn end.** In each jsonl, every `turn_end` but the last is followed by more `text` lines; there is never a second `turn_end` without text in between. `turn_end=3` proves the lines did not merge into one turn.

## Gotchas

- The helper feeds stdin from the file, so all lines arrive at once. `reef-agent` still sends each only after a `turn_end`; that is the behavior under test.
- Blank lines are skipped. Leading and trailing spaces are trimmed.
- Claude repeats the `session` line every turn with the same id; codex prints it once.
- A turn without tool calls has one `usage` line (claude sends one per model message). There is no session-total line on a normal exit; only Ctrl-C prints one.
- When a background task ends in a turn, claude finishes that turn, then runs a turn of its own for the task before the next prompt, even if one is waiting. The adapter drops that turn's text and tool events but keeps its `usage` line, so a turn can show a few more `usage` lines than it had model messages. The proof that it is gone is the turn text, not the usage count.
- An `error` line ends the process with exit 1; later stdin lines are not sent.

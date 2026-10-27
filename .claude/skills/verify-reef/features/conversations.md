# Direct conversations

A user starts a conversation with one Teammate, talks to it, comes back to an older conversation and
continues it. Each conversation is its own Space with its own agent session, and the Teammate runs with
its default access rules, never a Team's. Contract: `docs/server.md` (Spaces, Teammates).

## Sub-features

- `conv-own-session` two conversations with `sam` keep their own sessions: two `session_id`s in `turn`, and the second one's session file never has the first one's codeword.
- `conv-list` `GET /teammates/{id}/conversations` lists the Teammate's conversations, the one with the newest message first; an unknown Teammate is 404.
- `conv-resume` a send to an older conversation, also after a server restart, runs a turn with `--resume <its session id>` and the same `session_id`.
- `conv-access` a conversation's `space_member` row for `sam` has his default `write` (0) while his `team_member` row in `crew` has 1; his `claude` runs with `--disallowedTools Edit,Write,MultiEdit,NotebookEdit` and a Write is refused.
- `window-conversations` the sidebar lists the conversations under Conversations, newest first, `+` starts one by picking a Teammate, and the phone Spaces tab lists them too (`$H ui conversation`).

## How to get to it (user POV)

- Window: Conversations, `+`, pick a Teammate. Click an older row to continue it.
- REST: `POST /spaces {"projectId":1,"name":"...","teammateId":2,"kind":"conversation"}`, then `/spaces/{id}/send`.

## Driving it with verify-reef

Preconditions: baseline from the README. `ANTHROPIC_MODEL=sonnet` on `serve-start`, fresh data folder `conv`. Fake `claude` scenarios `sam-codeword`, `sam-no-codeword`, `sam-recall-codeword` and `write-refused`.

- **Setup.** `$H api POST /projects '{"name":"talk","path":"<scratch>/cwd"}'`, `$H api POST /teammates '{"name":"sam","backend":"claude","write":false}'` (2; the seeded `solo` lead is 1), `$H api PUT /teams/crew '{"roles":[{"name":"lead","lead":true,"backend":"claude"}]}'`, `$H api POST /teams/crew/members '{"teammateId":2}'`, `$H api PUT /teams/crew/members/2 '{"lead":false,"tools":[],"write":true,"permissionMode":""}'`.
- **Two sessions.** `$H api POST /spaces '{"projectId":1,"name":"first","teammateId":2,"kind":"conversation"}'`, send `Remember the codeword plum-31 for this conversation only. Use no tools. Reply with the codeword only.` and wait for the turn to be `done`. The same for Space 2 `second` with `Did I give you a codeword? Answer with the codeword, or none if I did not. Use no tools.` `$H sql "select space_id, session_id from turn"` has two different ids; `grep -c plum-31` on the two session files in `~/.claude/projects/<key>/` is above 0 for the first and `0` for the second; the second's `sam` message starts with `none` (`conv-own-session`).
- **List.** `$H api GET /teammates/2/conversations` has Space 2 then Space 1 (`conv-list`); `GET /teammates/99/conversations` is 404.
- **Continue.** `$H serve-stop`, `$H serve-start --data-name conv`, send `Which codeword did I just give you? Answer with the codeword only. Use no tools.` to Space 1 and wait for its second `done` turn. The turn has the first turn's `session_id`, the line of `fake-claude.log` for scenario `sam-recall-codeword` has `--resume <that id>`, the reply is `plum-31`, and the list is Space 1 then Space 2 (`conv-resume`).
- **Access.** `$H api POST /spaces '{"projectId":1,"name":"write","teammateId":2,"kind":"conversation"}'`, send `Use the Write tool to create the file t.txt containing the word nope. If that is refused, try once with Bash. Say what happened.` and wait. `$H sql "select space_id, name, write from space_member union all select team_id, name, write from team_member where name='sam'"` has `1|sam|0`, `2|sam|0`, `3|sam|0` and the team's `2|sam|1`; the `fake-claude.log` lines for `role=sam` all have `--disallowedTools Edit,Write,MultiEdit,NotebookEdit`, and `<scratch>/cwd/t.txt` does not exist (`conv-access`).
- **Window.** `$H serve-start --data-name ui2`, `REEF_VERIFY_CHROME=... $H ui conversation`: every line `PASS`, screenshots `ui/conversation-desktop-two.png`, `-desktop-continued.png` (1280x800) and `-phone.png` (390x844) (`window-conversations`). Open them.
- `$H serve-stop`, `$H cleanup`.

## Gotchas

- A conversation runs in the project folder: it has no work item and so no worktree.
- A conversation started with the default name `<teammate> <hh:mm>` is renamed by the provider after its first message; the REST steps give a name, the window scenario renames through the API.
- To refresh a recording, run the same prompt once with `REEF_VERIFY_CLAUDE=record`, and tell claude to use no tools: its auto-memory otherwise tries a file Write or a Bash `cat` of its memory folder.
- The sidebar refreshes its Spaces every 5 seconds, so the window scenario waits for the order.

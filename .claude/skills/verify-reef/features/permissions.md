# Permission prompts

A claude Bot that calls a tool outside its allowed tools does not fail silently: the call is held,
the Space shows a card with the tool, the command or path, and Allow once / Allow always / Deny, and
the Bot shows as waiting on the user while the other Bots keep working. The answer goes on with the
same turn. A prompt nobody answers expires, the turn ends with the denial, and the card stays so a
late Allow wakes the Bot to retry. Contract: `docs/server.md` (Permission prompts, `permission_prompt`,
Events), `docs/agents.md` (Permission prompts).

## Sub-features

- `prompt-shows` a Bot without `write` reading a file outside its folders stores a `waiting` `permission_prompt` (`tool` `Read`, `input.file_path`, `rule` `Read(//tmp/reef-prompt-outside/**)`), its turn stays `running`, and the Space's event stream sends `prompt` (the row) and `turn` `{"role":"dba","botId":2,"status":"waiting"}`.
- `prompt-allow` `POST /prompts/{id}/answer {"answer":"once"}` lets the same turn go on: the stream sends `running` then `done` for it, the tool row's result is the file and the Bot's reply quotes it.
- `prompt-deny` `{"answer":"deny"}` gives the call the tool result `denied by the user`; the prompt is `denied` with that reason.
- `prompt-chat-folder` a Read of `<data>/chats/<id>/note.txt` stores no prompt.
- `prompt-project` a Space's attached Project repository is passed to claude as `--add-dir <repo>` (fake log); with real claude a Read there stores no prompt.
- `prompt-other-bot` while one Bot waits, `@sam` in the same Space runs sam's turn to `done`; the prompt is still `waiting`.
- `prompt-expire` with `--prompt-wait 5s` an unanswered prompt becomes `expired`, reason `no answer from the user`, the call gets that text as its tool result and the turn ends `done`.
- `prompt-new-message` a new message to the waiting Bot denies its prompt with reason `a new message came; the user did not answer the prompt`, and the new message runs as the next turn.
- `prompt-stop` `POST /spaces/{id}/stop` denies the prompt with reason `stopped` and ends its turn `failed`, `interrupted`.
- `prompt-late-allow` `GET /spaces/{id}/prompts` still lists the expired prompt; `once` on it sets it `allowed` and `granted`, wakes the Bot with `I allowed your Read call that got no answer: ...`, and the retry reads the file with no new prompt (`granted` back to `0`); `always` on an expired one also puts the rule in `GET /bots/{id}` `tools` and the Space member's `tools`. An answered prompt is 409, an unknown answer 400.
- `window-prompts` the card in the Space with Allow once, Allow always and Deny, `waiting on you` under it, Allow once goes on to the reply; an expired card says `Denied: no answer from the user` with Allow once and Allow always and no Deny, and Allow once there runs the retry (`$H ui prompts`).

## How to get to it (user POV)

- Window: a Space with a claude Bot that has no `write`; ask it to read a file outside its folder. The card shows above the message box; the Bot's pill says `waiting on you`.
- REST: `GET /spaces/{id}/prompts`, `POST /prompts/{id}/answer {"answer":"once"|"always"|"deny"}`, the `prompt` and `turn` events of `GET /spaces/{id}/events`. `reef serve --prompt-wait DUR` (default `10m`).

## Driving it with verify-reef

Preconditions: baseline from the README. Fake `claude` scenarios `prompt-outside` (also the retry wake), `read-chat-folder` and `say-ok-one-word`. `mkdir -p /tmp/reef-prompt-outside && echo "outside-word: walrus-58" > /tmp/reef-prompt-outside/note.txt`. Wait for a prompt by polling `$H api GET /spaces/1/prompts` for `"status":"waiting"`, and for a turn end by polling `$H sql "select count(*) from turn where status='running'"` for `0`. `ASK='Use the Read tool to read the file outside your folders at /tmp/reef-prompt-outside/note.txt and reply with its contents.'`

- **Setup.** `$H serve-start --data-name p`. `$H api POST /bots '{"name":"dba","backend":"claude","write":false,"description":"You are the dba. Answer in one short sentence."}'` (2), the same for `sam` (3), `$H api POST /projects '{"name":"shop","path":"<scratch>/cwd"}'`, `$H api POST /spaces '{"name":"prompts","botIds":[2,3],"coordinatorBotId":2}'` (Space 1, free, runs in `<scratch>/p/chats/1`), `$H api PUT /spaces/1/projects/1`. Keep `curl -sN http://<serve.addr>/spaces/1/events > $E/sse.txt &` running.
- **Shows (`prompt-shows`).** `$H api POST /spaces/1/send '{"text":"'"$ASK"'"}'`. `GET /spaces/1/prompts` has prompt 1 `"tool":"Read","input":{"file_path":"/tmp/reef-prompt-outside/note.txt"},"rule":"Read(//tmp/reef-prompt-outside/**)","status":"waiting"`; `$H sql "select id, status from turn"` is `1|running`; `grep -A1 '^event: \(turn\|prompt\)' $E/sse.txt` has the `prompt` row then `{"role":"dba","botId":2,"status":"waiting"}`.
- **Other Bot (`prompt-other-bot`).** `$H api POST /spaces/1/send '{"text":"@sam Say ok in one word."}'`, wait for a `done` turn of Bot 3: turns are `1|2|running`, `2|3|done`, prompt 1 is still `waiting`, the stream has `sam` `running` then `done` after dba's `waiting`.
- **Allow (`prompt-allow`).** `$H api POST /prompts/1/answer '{"answer":"once"}'` is 200 `"status":"allowed","answer":"once"`. After the turn ends: turn 1 `done`, `$H sql "select result from tool_call where turn_id=1"` is `outside-word: walrus-58`, dba's last message is ``The note says: `outside-word: walrus-58` ``; the stream sends `running` then `done` for dba.
- **Deny (`prompt-deny`).** Send `$ASK (deny case)`, wait for prompt 2, `$H api POST /prompts/2/answer '{"answer":"deny"}'`: prompt `denied`, reason `denied by the user`; its turn's `tool_call.result` is `denied by the user`.
- **Chat folder (`prompt-chat-folder`).** `echo "chat-word: otter-34" > <scratch>/p/chats/1/note.txt`, send `Use the Read tool to read the file in your folder named note.txt and reply with its contents.`: the turn is `done`, its Read has `file_path` `<scratch>/p/chats/1/note.txt`, and `select count(*) from permission_prompt` is unchanged.
- **Project (`prompt-project`).** Every `fake-claude.log` line of the Space has `--add-dir <scratch>/cwd ... --permission-prompt-tool stdio` and `--permission-mode default`. The fake cannot show claude's own rule; the real check is in the record run below.
- **New message (`prompt-new-message`).** Send `$ASK (new message case)`, wait for the prompt, send `Say ok in one word.`: the prompt is `denied|deny|a new message came; the user did not answer the prompt`, its turn is `done` with that tool result, and the next turn is `done` with dba's `Ok.`.
- **Stop (`prompt-stop`).** Send `$ASK (stop case)`, wait for the prompt, `$H api POST /spaces/1/stop` (204): the prompt is `denied|deny|stopped`, its turn `failed|interrupted`, no `claude` child of the server is left.
- **Expire (`prompt-expire`).** `$H serve-stop`, `$H serve-start --data-name p --prompt-wait 5s`, restart the stream capture. Send `$ASK (wait case)`, wait for the prompt, `sleep 7`: the prompt is `expired|deny|no answer from the user` with `answered_at` 5 s after `created_at`, its turn `done`, its Read's result `no answer from the user`; the stream has `prompt` `expired`, then `running` and `done` for dba. `GET /spaces/1/prompts` still lists it.
- **Late allow (`prompt-late-allow`).** `$H api POST /prompts/<id>/answer '{"answer":"once"}'` is 200 `"status":"allowed","granted":true`; `$H api GET /spaces/1/direct-messages` has the guidance `I allowed your Read call that got no answer: Read(//tmp/reef-prompt-outside/**). Try it again.`; the next turn is `done` with the file as its Read result, the prompt count is unchanged and the prompt's `granted` is `0`. Then `$H api GET /bots/2` has `"tools":[]`, send `$ASK (always case)`, let it expire, answer `always`: `GET /bots/2` has `"tools":["Read(//tmp/reef-prompt-outside/**)"]`, `GET /spaces/1/members` lists it for dba, the retry turn is `done`. `POST /prompts/<id>/answer '{"answer":"once"}'` again is 409 `prompt N is allowed: conflict`; `'{"answer":"maybe"}'` is 400.
- **Window (`window-prompts`).** Build the web app and rebuild (`(cd web && npm ci && npm run build)`, `$H build`, `$H doctor`), `$H serve-stop`, `$H serve-start --data-name ui --prompt-wait 15s`, `$H ui prompts`: every line `PASS`, screenshots `ui/prompts-waiting.png`, `-allowed.png`, `-expired.png`, `-retried.png`. Open them.
- `$H serve-stop`, `$H cleanup`, `rm -r /tmp/reef-prompt-outside`.

Record or refresh the two recordings (real claude, about $0.20): `REEF_VERIFY_CLAUDE=record ANTHROPIC_MODEL=sonnet $H serve-start --data-name rec`, the Setup above with Bot `dba` only (`POST /spaces '{"name":"prompts","botId":2}'`) and the Project attached, then send `$ASK`, answer prompt 1 `once` once it is listed, then the chat-folder message with `<scratch>/rec/chats/1/note.txt` written first. `fake-claude.log` shows `record: recorded prompt-outside` and `record: recorded read-chat-folder`. In the same server, `Use the Read tool on <scratch>/cwd/repo-note.txt (the attached shop Project) and reply with its contents.` (no scenario, `record: not recorded`) is the real `prompt-project` check: its turn is `done`, its Read returns the file and `GET /spaces/1/prompts` stays `[]`.

## Gotchas

- A Bot with `write` on claude runs with no `--permission-mode`, so the user's own `permissions.defaultMode` decides; with `auto` claude never asks. Use a Bot without `write` (`--permission-mode default`).
- Real claude's `can_use_tool` for a Read has no `blocked_path`, so the row's `path` is `""`; the card shows `input.file_path`. A Bash call carries `blocked_path`.
- In a free Space `chats/<id>` is the working folder, so it gets no `--add-dir`; a work item's Space gets `--add-dir <data>/chats/<id>`.
- The fake replays the recorded tool result: the file's content in a fake run is the recorded `outside-word: walrus-58`, whatever the file holds now. After a deny it prints no reply text, so the Bot posts no message.
- Stop of a waiting turn sends the `prompt` event (`denied`, `stopped`) but no `turn` `failed` event; the window still clears `waiting on you`, and a new subscriber gets `idle`.
- The window scenario needs a `--prompt-wait` under 60 s, and one long enough to click Allow once first (15 s).

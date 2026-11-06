# Channel fixes (restructure step 3)

[Back to restructuring index](index.md)

> Status: done. Written 2026-10-26 as a plan (slice issue #65, tickets #46–#64) and built since. This is history: it says channel where Reef now says Space.

## Goal

Channels and agents work correctly and send no wasted text, before memory is plugged in. Done means every ticket below is closed, the channel eval passes, and `go test ./...` passes with no known failures.

## Why

An audit of a real channel ("browser issue": team small, 39 turns, 60 messages) and a review of `channel/`, `server/` and `agent/` found these problems:

- 83% of the text sent to agents was text they had already received or written. One wake was 90,755 characters. All message text in the channel was 63,841 characters.
- The cost shown was about $35.50; the real cost was about $4.70.
- Wakes can be lost, a dead session is resumed forever, and a read-only role can still change files.

## Order

```
1. Channel eval            measures everything below; fails today
2. Wasted text             tickets 2-4
3. Cost and tokens         tickets 5-8
4. Lost or stuck work      tickets 9-12
5. Permissions             tickets 13-16
6. Small fixes and tests   tickets 17-19
then: design the context assembler (next step), then memory wiring (step 4)
```

## Tickets

### 1. Channel eval (#46)

**What to build:** a `go test` package that replays a scripted channel, shaped like "browser issue", through the real server routing and the real `channel.Session`, with a fake agent at the `agent.Agent` boundary. It records every prompt, wake, turn and cost, and checks the numbers below. Like `memory/eval`, it prints its numbers with `-v`.

**Acceptance criteria:**
- [ ] The scenario has a lead, a developer and a tester; one long thread; a double post; an @name in plain text; and an app restart with a queued wake
- [ ] It checks: no message body is sent twice to one role in one session; no body appears twice in one prompt; wake size is at most the size of its new messages plus a fixed header; wasted wakes; summed turn cost equals the session cost; no wake is lost
- [ ] It prints: total characters sent, duplicate characters, largest wake, wake count
- [ ] Each check that fails today is marked with the ticket that fixes it, and that ticket removes the mark

**Blocked by:** none

### 2. A wake carries only unread messages, read when it runs (#47)

**What to build:** build the wake text when the turn starts, not when the message is routed. It holds the messages in the thread (or the channel) that this role has not been sent in this session, and never the role's own messages. A new session gets the thread once. This fixes the thread sent on every wake, the thread twice in one wake, and the old wake text.

**Acceptance criteria:**
- [ ] In a 30-message thread, each wake holds only the messages since the role's last turn (eval)
- [ ] Two messages in one thread that wake one role give one copy of the thread
- [ ] A message posted after the wake was queued but before it runs is in the prompt
- [ ] A role in a new session gets the thread once

**Blocked by:** 1

### 3. One post per turn (#48)

**What to build:** when an agent called `post_to_channel` during a turn, its plain reply is not posted as a second message. It stays on the turn.

**Acceptance criteria:**
- [ ] A turn that calls `post_to_channel` and ends with a reply leaves one message in the channel
- [ ] A turn with no `post_to_channel` still posts its reply
- [ ] The second message no longer wakes anyone (eval wake count)

**Blocked by:** 1

### 4. Only a handoff @mention wakes a role (#49)

**What to build:** the rule that decides when an @name wakes a role (decision 1). "I'll report to @lead later" wakes nobody.

**Acceptance criteria:**
- [ ] `@developer please merge` wakes the developer
- [ ] `I'll report to @lead later` wakes nobody
- [ ] `docs/product.md` gets the rule as a new decision

**Blocked by:** 1

### 5. Turn cost is the turn's own cost after a resume (#50)

**What to build:** a resumed claude reports the session's running total. The stored turn cost must be that total minus the total before the turn.

**Acceptance criteria:**
- [ ] A resumed session reporting totals 1.00 then 1.10 stores turn costs 1.00 and 0.10
- [ ] The inspector's per-role cost equals the session total in the eval

**Blocked by:** 1

### 6. Subagent and background tokens are counted once (#51)

**What to build:** count subagent tokens in the turn's usage. A background task turn that starts before the prompted turn ends never makes negative usage. A background task's answer is posted to the channel as the role's message (decision 2).

**Acceptance criteria:**
- [ ] A replayed stream with a subagent: usage events add up to the stream's `modelUsage`
- [ ] A replayed stream with an early background turn: no usage event is negative
- [ ] The background task's answer is posted to the channel as the role's message

**Blocked by:** 1

### 7. A crash keeps the turn's prompt and tokens (#52)

**What to build:** store the prompt when it is sent, and usage as it arrives, so a turn closed as "chat exited" keeps both. The next resume then does not re-send the persona.

**Acceptance criteria:**
- [ ] A turn running when the app quits keeps its prompt and its tokens so far
- [ ] The next resume of that role sends only the wake, not "Your persona or skills changed"
- [ ] Those tokens count toward the channel budget

**Blocked by:** 1

### 8. Codex interrupt keeps the stopped turn's usage (#53)

**What to build:** stopping a codex turn while events are unread still delivers its usage, without waiting the full second.

**Acceptance criteria:**
- [ ] Interrupt with unread events gives one usage event and returns in under 1 second

**Blocked by:** none

### 9. A missing session starts a new one (#54)

**What to build:** when resume fails because the backend has no such session, the role starts a new session with its first message.

**Acceptance criteria:**
- [ ] After "No conversation found", the next turn calls Start, and its prompt begins with the role intro
- [ ] The failed resume is not retried on every wake

**Blocked by:** 1

### 10. Queued wakes survive a restart (#55)

**What to build:** wakes held while a role is busy are stored, not kept only in memory.

**Acceptance criteria:**
- [ ] Lead posts `@architect @tester`, the architect is running, the app restarts: the tester's wake runs or waits as a hold the user can see
- [ ] The eval restart check passes

**Blocked by:** 1

### 11. A message sent while a chain starts is queued (#56)

**What to build:** a send during chain start is queued, as `docs/server.md` says, not refused.

**Acceptance criteria:**
- [ ] The second send returns 202 and both texts run

**Blocked by:** none

### 12. No wake is dropped silently (#57)

**What to build:** the four error paths that lose wakes keep them or record a failure.
- A user message after a hop-cap pause drops held direct messages.
- A failed direct-message read drops the turn's public wakes.
- A store error before the next wake leaves the wakes unreachable.
- Some start-up failures store no turn.

**Acceptance criteria:**
- [ ] A held direct message gets a `failed` response with body `dropped` after a user message
- [ ] After a failed direct-message read is fixed, `continue` runs the architect's wake
- [ ] A store error pauses the channel, so `continue` runs the held wakes
- [ ] `RunAs` for a removed role stores one failed turn with the wake text

**Blocked by:** none

### 13. A team edit reaches a running agent (#58)

**What to build:** when a role's write access, tools, model or permission mode changes, its live agent is restarted (resumed) with the new settings before its next turn.

**Acceptance criteria:**
- [ ] Give the lead write access mid-channel: its next turn has a checkpoint and edit tools
- [ ] Take write access away: its next turn has no edit tools

**Blocked by:** none

### 14. Read-only means read-only (#59)

**What to build:** the rule for shell commands that change files on a role without write access (decision 3).

**Acceptance criteria:**
- [ ] A turn of a read-only role whose tools allow `Bash(git commit:*)` holds the tree lock and gets a checkpoint
- [ ] A read-only role with only reading commands still runs as a reader

**Blocked by:** none

### 15. Undo points do not collide between two Reef copies (#60)

**What to build:** checkpoint refs name the data folder as well as the turn, so the installed app and the dev copy never overwrite each other's in one repo.

**Acceptance criteria:**
- [ ] Two data folders on one repo each checkpoint turn 1; rolling back in A restores A's files

**Blocked by:** none

### 16. Codex pre-approves every MCP server (#61)

**What to build:** every MCP server's tools are pre-approved on codex, not only `reef`.

**Acceptance criteria:**
- [ ] Servers `x` and `reef` both get approval mode `approve`

**Blocked by:** none

### 17. Small adapter fixes (#62)

**What to build:** store image tool results as `[image]`. Keep only the last 4 KiB of claude stderr.

**Acceptance criteria:**
- [ ] A `Read` of a `.png` stores a non-empty result
- [ ] 10 MB of stderr leaves the buffer near 4 KiB

**Blocked by:** none

### 18. Server tests pass (#63)

**What to build:** fix the 14 failing GitHub-poll tests (`FeatureLatestChannel` selects 12 columns, `scanChannel` scans 13) and the flaky `TestDirectChainEndClearsActiveTurn`.

**Acceptance criteria:**
- [ ] `go test ./...` passes 5 times in a row

**Blocked by:** none

### 19. An approved wake resets the hop count (#64)

**What to build:** an approved wake resets the hop count, as a retry does (decision 4). `docs/server.md` and decision 69 in `docs/product.md` say the token budget is checked only between turns (decision 5).

**Acceptance criteria:**
- [ ] 6 approved hops in a row do not pause the channel
- [ ] 6 hops with no approval still pause it at the cap
- [ ] Decision 69 is amended in `docs/product.md`

**Blocked by:** none

## Decisions

Settled 2026-10-26 (the recommended options).

| # | Decision | Why |
| --- | --- | --- |
| 1 | Only an @name at the start of the message or of a line wakes a role | Matches how handoffs are written; "I'll report to @lead" wakes nobody |
| 2 | A background task's answer is posted to the channel as the role's message | The user asked for that work |
| 3 | A role without write access but with shell commands that change files is treated as a writer: tree lock and checkpoint | Keeps such roles working, and safe |
| 4 | An approved wake resets the hop count | The user already looked at it |
| 5 | The token budget is checked only between turns; decision 69 is amended to say so | Stopping mid-turn leaves half-done edits |

## After this

1. **Design the context assembler:** done, R31–R36 in [decisions.md](decisions.md). It reads today's tables; memory plugs in during step 4.
2. **Step 4, memory wiring:** the MCP tools `remember`, `recall` and `read_memory` (R23), and the assembler wired into channels. Its own slice.

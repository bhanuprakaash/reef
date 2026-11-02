# Server

> **Target contract:** [Product decisions 143–155](product.md#decisions) define Bots, roster-only Crews, global conversations and execution boundaries. This document describes the existing runtime until those changes are implemented; its Team and Bot terms do not define the target. Use a fresh development data directory for the new model. No legacy migration or compatibility path is required, but preserve histories created by the new runtime.

`reef serve` keeps Reef's data in one SQLite file and serves it as a JSON REST API. The
store is [`store/`](../store), the routes are [`server/`](../server), the entry point is
[`cmd/reef/main.go`](../cmd/reef/main.go). The code is the source of truth. This doc says
what it does and why.

## Fresh development database for the target model

Use a new, isolated data directory when developing the new model. For example:

```sh
reef_data_dir="$(mktemp -d /private/tmp/reef-dev.XXXXXX)"
reef serve --data "$reef_data_dir" --addr 127.0.0.1:7701
```

This starts with a fresh database and leaves the existing data directory untouched. Use the same directory to test restart and history preservation after the new runtime creates records. The current runtime still applies its existing migrations; the target transition needs no legacy migration or compatibility program.

## Run

```
reef serve [--addr 127.0.0.1:7700] [--data DIR]
```

- `--data` is the data folder. It holds `reef.db` and `reef.lock`, and is created if
  missing. The default is `~/Library/Application Support/Reef`. Use `--data` for tests or a
  second instance.
- One server owns a data folder. `reef serve` locks `reef.lock` for as long as it runs; the
  lock is in `cmd/reef`, not in the store. A second `reef serve` on the same `--data` exits
  with `data folder ... is in use by another reef`, before it touches the data, whatever its
  `--addr`. A second instance needs its own `--data` and its own `--addr`.
- Other `reef` commands (the CLI, `reef mcp`, `reef chat`) do not take the lock. They open
  `reef.db` directly with `store.Open` while the server runs. `reef chat` takes its own
  `chat-<space id>.lock` while it runs, and so does the server for a live Space;
  restart recovery reads it (see below). WAL mode
  and a 5 second busy timeout let the processes share the file. Two processes that open a fresh folder at once
  both succeed: each migration runs in one `BEGIN IMMEDIATE` transaction and re-reads
  `user_version` inside it, so a migration is applied once.
- `--addr` is where to listen. The default is `127.0.0.1:7700`. The API has no login, so
  `reef serve` refuses an `--addr` whose host is not `localhost` or a loopback IP,
  including an empty host such as `:7700`, before it opens the data folder. It also refuses
  a request whose `Host` is not `127.0.0.1`, `localhost` or `::1` (403), and a `POST`,
  `PUT` or `PATCH` whose `Content-Type` is not `application/json` (415). This stops a web
  page on another origin from writing to it, including through DNS rebinding.
- The file opens in WAL mode with foreign keys on. Migrations in
  [`store/migrations`](../store/migrations) run at start, in order, each once. They only
  go forward. A file whose version is newer than the newest migration (made by a newer
  reef) is refused: `database is newer than this reef`.
- SIGINT or SIGTERM stops the server: it interrupts every running Space turn (stored
  `failed`, `interrupted`), stops every agent, then gives requests in flight one second.
  The agents stop in parallel, so stop time is about one agent stop (about a second), not
  one per live Space.

## Web UI

`reef serve` also serves the React app in [`web/`](../web). `web/dist` is embedded in the
binary with `go:embed` ([`web/embed.go`](../web/embed.go)). A fresh clone builds without
npm: `web/dist` holds only `.keep`, and a page load answers 503 `UI not built: run npm run
build in web/` until the app is built.

```
cd web && npm ci && npm run build
cd .. && go build -o reef ./cmd/reef
./reef serve
```

Then open `http://127.0.0.1:7700`. Build the web app before `go build`, because the binary
embeds `web/dist` as it was at build time. A Space's agent is the `claude` or `codex` CLI,
logged in and on `PATH`.

- Sidebar: a top row with the brand (it opens Today), search, the project switcher, then Inbox with
  its count, Board, Bots, Spaces and Conversations. Inbox and Board are one-line rows. Each section label
  has a + for a new Bot, Space or conversation, and the Board row has a + for a new Work item. A chevron
  on the section label collapses the section (the choice is kept in the browser, and the count shows
  while it is collapsed). Each section lists at most eight one-line rows and a See all N link to its
  full list: all pinned rows, then the newest others. The switcher shows the project as a coloured
  tile, its emoji or else its initial (All projects is a grid tile), and lists All projects first and then each project; the choice is a
  filter only. The switcher's Change icon item and the tile on the project page open a picker with an emoji grid, a Letter option and eight colours; each choice saves at once with `PUT /projects/{id}`. Under a project the sidebar reads `GET /spaces?project=ID`, so it lists the Spaces
  associated with that project; under All projects it reads `GET /spaces`. Switching never changes a
  Space's projects, context or sessions, and starts no chat. New Space and New conversation under
  All projects send `POST /spaces` with no `projectId`; New Work item needs a project (its + is
  disabled until one is picked). Spaces lists the free
  Spaces and the Spaces of Work items that are not yet
  in Review, Done or Dropped; its See all link opens `/spaces`, which lists them all. Conversations lists the
  conversations, newest first, with the Bot's creature; its See all link opens `/conversations`. Both pages have a
  search box and group their rows under Pinned, Today, Yesterday, Last 7 days and Older (by last message; empty
  groups are hidden). The sidebar has no Pinned
  section: pinned Bots, chats and Spaces (`GET /pins`) show a pin mark and sort to the top of their own section, under every
  filter. Bots lists the Bots of the newest chats under the filter, pinned Bots first, then the other Bots; its See all link opens `/bots`, and each row opens
  `/bots/ID/chat`. A Space or chat row has a … button and a right-click menu with Rename, Pin or Unpin
  and Delete; a Bot row's menu has only Pin or Unpin. The Bot page and its chat rows also pin and unpin. Typing in
  search replaces the list with `GET /find` results (Bots, chats, Spaces, Work items); a result opens
  `/bots/ID/chat`, `/spaces/ID` or `/work-items/ID`. `/bots/ID/chat` opens the Bot's latest chat under
  the selected project (`?resumable=1&project=N&limit=1`) or overall, and with none opens the Bot page, whose New
  chat starts one; the Bot page lists the chats 20 at a time with Load more. A Space loads its newest
  50 messages and a Load earlier messages button fetches the page before the oldest shown; a live refetch asks `after` the id 50 from the newest, so nothing loaded drops out, and a reply whose root is not loaded gets it with `?before=ROOT+1&limit=1`. Each Space row
  shows its time. The list scrolls; the foot is
  fixed and shows your name beside the Bots and Settings links. Below 768 px the sidebar is
  a sheet that the bottom bar's Menu button opens (Today, Board, Spaces, Needs you, Menu); a link in it,
  Escape or a tap outside closes it, and a Space opens full screen.
- Today (`/`): what waits on you, the project's work items, and capture.
- Space: bubbles with each author's creature (a Bot bubble is a slight lift on the page with a 1px edge) and an always-visible Reply button, replies folded under their thread root with a
  count, and a side panel for the team, a thread, the inspector, the pull request or the diff (its expand button opens
  the review page). The header's `PR #N` chip opens the pull request panel: title, state, base and head, review, checks and
  Open on GitHub; Merge asks to confirm, then does `POST /work-items/{id}/pr/merge`. The header has the Bot faces and the title, with the Work item's status, budget, `PR #N` and diff chips under it; on the right New chat (only in a 1:1 conversation: a new conversation with the same Bot), a Bots popup (the roster, Add Bot, and Bot settings, which opens the Bots side panel), the Browser and Terminal toggles (desktop), Wrap up (a Work item Space) and a "…" menu with Inspector, Attach to Work item and Open Work item. Below 768 px the toggles go, New chat is an icon, the faces open the Bots popup and Wrap up moves into "…". The running turn names its
  bot. Bots' direct messages (`GET /spaces/{id}/direct-messages`) are never bubbles: each conversation
  between two roles (from one public source message, or from none) is one compact item at the time of its
  first message, saying working, paused, failed, stopped or completed (the state of
  its newest chain) with its message count; it opens a Coordination panel with the exchange, the running direct turn's text and tool calls as they stream, a link to
  the public message it began from, and Continue or Retry for its state. A pause from the hop cap shows in
  the cap banner. A Space with rollbackable turns (`GET /turns?space=ID&rollbackable=1`)
  lists Checkpoints; Roll back on a row asks to confirm, then does `POST /turns/{id}/rollback`.
  A role whose newest ended turn has a tool call that claude refused for lack of permission (the
  result says it "requested permissions to use X" or "requires approval", or that an edit tool "is
  disabled for this session"), read from `GET /spaces/{id}/inspector`, gets a card
  "<Role> was denied <tool>" under the messages. A `Bash` call shows as `Bash(<first word>:*)`, and a
  compound command shows no card. Add to role reads `GET /bots/{id}`, appends the pattern to that bot's
  `tools` and saves with `PUT /bots/{id}`, then `POST /spaces/{id}/bots/{botId}/refresh` so that
  Bot's session in this Space adopts it (Add is an explicit grant; sessions otherwise keep their
  adopted settings); the card then says Added. The Bot's next wake starts a new context with it in
  `--allowedTools`. Add is disabled while a turn runs. A read-only role refused an
  edit tool shows "Needs write access" and a codex role "Codex ignores allowed tools", both with no Add.
  A checkpoint taken in a workspace the Space's work item does not have, such as a free Space's
  turn after an attach, is not listed. A conversation's side panel lists its open fixes (Project,
  branch, head, state, "origin could not be refreshed" note) with Abandon. A tool row shows the name of the Project it works in when
  the server names its `targets` (every Project it points into). A stopped or failed turn shows `Failed: <error>` with its tool rows; its partial
  reply is the stored message, so the live block does not repeat it.
- A closed Space shows "Wrapped" and Reopen where the Wrap up button and composer were.
- Work item page, with Edit, Close (asks to confirm) or Reopen, and its diff review page; project
  page (`/projects/{id}`) with its work items grouped by status in board order and its free Spaces, and a right rail with Purpose and Base branch (each saves when the field loses focus, no Save button) and the project memory (3 newest, Show all, one add row). A top-level item shows its acceptance lines, sub-items, brief versions, its Spaces and its proof (`Not submitted yet` until the owner submits; one section per repository with branch, commit, base, workspace folder and checks, the PR of the item's branch and each screenshot as an image); a sub-item shows the lines only. While a top-level item is in Review the page opens with a Review card: the wrap-up proposal (`No proof submitted` when there is none), a note for the owner, Send back (needs the note, `PATCH` with `queued` and `note`) and Done (`PATCH` with `done`). The page lists the work item's accepted `work_item` memory items and the project page the project's, each with Retire; the project page also adds an item (kind and body). The window reads and writes them through `/memory`, not `/decisions` or `/projects/{id}/memory`.
- Board (`/board`): the top-level work items of the current project in the columns Backlog, Queued, In progress, Needs you, Review, Done and Dropped, refreshed every 5 seconds; filters for type, owner and project are in the URL. Dragging a card from Queued to Backlog is `PATCH /work-items/{id}`; a Backlog card is queued by its Approve form (`POST /work-items/{id}/approve`), not by dragging; dropping a queued card on another queued card is `PUT /work-items/{id}/queue-position`. Any other drop is refused in the window with a message: those statuses are reached through the item's own actions.
- Inbox (`/inbox`): what waits on you from [`/inbox/events`](#inbox), in two groups, Needs you and
  FYI, with a count each. Every item is one compact row: the Bot's face, the Space name, a plain
  type label (Failed, Paused, Held, Review, Question, Replied, Sub-item, Proposed, Started, Space
  proposed), a one-line preview of its text with the markdown marks removed, and its age. A
  Needs-you row shows its one action: Retry for a failed turn, Continue for a pause or a held
  wake, Review for an item in review, Reply for a question. Hovering a row adds Open (Needs you)
  or Open and Dismiss (FYI). Clicking a row opens it in place to the rendered message with the
  same actions, and a blue dot marks a row not opened yet (kept in this browser). FYI has Dismiss
  all seen, which dismisses the FYI rows already opened. The window asks once, on a click, to
  show desktop notifications and then shows one for each new item since the page loaded, except
  the FYI items of Work items.
- Crews (`/teams`): lists Crews, creates one (`POST /teams`) with a name, an optional description (the `responsibility` field), Bots and an optional coordinator suggestion, deletes one (`DELETE /teams/{name}`) and edits one's Bots, saving with `PUT /teams/{name}`. A Crew has no queue, memory or capability editor: a Bot's profile is edited on the Bot. "New crew" (Duplicate opens it too) asks for a name, a description and a Crew to start from, and sends `POST /teams?from=<source>` with the source's Bot ids; it is rejected if that name is taken.
- Starting a Space (the New Space form) picks one or more Crews and previews their Bots without duplicates, each with an include box, and one coordinator. Picking another Crew adds its Bots and keeps the coordinator. Start sends `POST /spaces` with `botIds` and `coordinatorBotId`.
- Bots (`/bots`): lists bots and edits one's description, skills, backend, model, face and access
  rules (`PUT /bots/{id}`), the one place they are set. A Bot's page (`/bots/{id}`) shows its Teams, its
  Chats (`GET /bots/{id}/chats`: personal chats, the Spaces it is in and its consultations, each
  with a kind badge, a working dot while a turn runs, its origin chat for a consultation, and a
  link to `/spaces/{id}/bots/{botId}`, the read-only Bot activity view) and its memory items.
  Consultation Spaces are not in the Spaces lists; they belong to the Chats lists of both Bots.
  Resume chat opens the newest open chat (`GET /bots/{id}/conversations?resumable=1&limit=1`);
  New chat does `POST /spaces` with `botId`, `kind` `conversation` and the current project's id, if
  any, and opens the new chat. Opening the page starts nothing.
- Settings (`/settings`): a left sub-nav in three groups, You (Profile, Appearance, Notifications), This Mac (Terminal, Updates; desktop only) and Agents (Providers, Connections, Skills), with a count beside Providers, Connections and Skills. One section shows at a time, picked with `?section=` (the first when it is missing); below 768 px the nav is a list and a section opens on its own with a Settings back link. Each section has a title and a one-line lede.
  Profile saves as you make it (the name on leaving the field, a chip at once, `PUT /profile`): your name, occupation and working preferences (chips). Appearance: theme (dark, light or Match macOS, kept in `localStorage`). Notifications: the Mac notification permission. Providers: the claude and codex found on `PATH`, each with path, version and models, and added installs with a folder and Remove; add one with a kind, a name and a folder, typed or picked with Choose… on desktop; a role picks one in Teams.
  Connections: the library of connections (add, Edit, Delete; each row shows its kind, command or URL and how many Bots use it); a Bot picks them in its panel. Skills: one searchable list with the filters All, Library, Claude Code and Codex (the last two read from disk for the current project) and a detail pane with the description, source, files, the Bots that use it and Edit (written in Reef) or Update (imported); Import (a folder or a Git link) and New skill (an editor with SKILL.md and supporting files) sit above the list, and the Bots' skill Drafts with Save and Reject sit below it (the same draft card shows in the Space that proposed it).
- First run: one page with two steps. Your name with an optional occupation and working preferences (saved with `PUT /profile`), then the first
  project. A name with no project starts at the project step; a project with no name asks only
  for the name.

Client routes such as `/spaces/3` and `/inbox` have the same paths as the REST API. In the Go server, `GET /` and `HEAD /` always get the app's `index.html`; other `GET` or `HEAD` requests get it when the `Accept` header includes `text/html`. Every other request goes to the API, so `curl /projects` and the app's own `fetch` calls (`Accept: application/json`) are unchanged. A file that exists in `web/dist`, such as `/assets/...`, is served as is. The Host and Content-Type checks guard the API only, not these files.

Dev mode: run `reef serve --addr 127.0.0.1:7701 --data ~/Library/Application\ Support/Reef\ Dev`,
then `cd web && npm run dev` and open `http://localhost:5173`. Vite proxies the API paths to
`127.0.0.1:7701`, the dev copy's port and folder, so it never reaches the installed app on 7700.

## Errors

Every error has a JSON body: `{"error": "work item 9: not found"}`.

| Status | When |
| --- | --- |
| 400 | Bad JSON, an unknown field or data after the JSON body, a bad or missing field, a non-number id, an unknown parent id inside a body (`unknown project 9`) |
| 403 | The `Host` is not `127.0.0.1`, `localhost` or `::1`. Body is `{"error":"forbidden host"}` |
| 404 | The id in the path (or a required query filter) matches nothing, or no route knows the path. Body for an unknown path is `{"error":"not found"}` |
| 405 | The path exists, the method does not. Body is `{"error":"method not allowed"}` |
| 409 | A unique rule broke. A send to a Space with a wrap turn running, to a Space paused by a cap other than the hop cap or to a Space open in a `reef chat`; `continue` on a Space that is not paused or has a turn running; `retry` of a turn that is not [retryable](#spaces-live), or on a Space with a turn running or paused; `rollback` on a Space with a turn running or open in a `reef chat`, on a tree other than the checkpoint's, or on a tree a turn runs on; an approval that is not pending, or one approved while the Space is paused |
| 415 | A `POST`, `PUT` or `PATCH` without `Content-Type: application/json`. Charset parameters are fine |
| 503 | A send after the server began to stop. Body is `{"error":"server is stopping"}` |
| 500 | Anything else. Body is `{"error":"internal error"}`; the detail goes to the log |

An id in the path that matches nothing is 404; an id in the body that matches nothing is
400. Unknown JSON fields in a request are rejected (`{"error":"invalid: bad JSON: json: unknown
field \"work_item_id\""}`), so a misspelled field never turns into a silent default. The
server-set fields an entity returns (such as `id` and `createdAt`) are accepted and ignored. A parent
deleted while a write is in flight gives the same 404 or 400 as one that never existed.

## Entities

Ids are integers and are never reused after a delete. Instants (`createdAt`, `startedAt`, ...) are RFC 3339 UTC strings.
A type with `?` may be `null`. The headings below are the SQL tables in
[`001_init.sql`](../store/migrations/001_init.sql), migration 2
[`002_token.sql`](../store/migrations/002_token.sql) (`token`), migration 3
[`003_proposal.sql`](../store/migrations/003_proposal.sql) (`proposal`), migration 6
[`006_team.sql`](../store/migrations/006_team.sql) (`team`, `space.team_id`),
migration 7 [`007_thread.sql`](../store/migrations/007_thread.sql) (`message.parent_id`) and
migration 8 [`008_approval.sql`](../store/migrations/008_approval.sql) (`gate`, `approval`; the `gate` table is dropped again by migration 43) and
migration 9 [`009_approval_kind.sql`](../store/migrations/009_approval_kind.sql) (`approval.kind`) and
migration 10 [`010_role_write.sql`](../store/migrations/010_role_write.sql) (`role.write`) and
migration 11 [`011_turn_wake.sql`](../store/migrations/011_turn_wake.sql) (`turn.wake_text`, `turn.wake_parent`, `turn.retried`) and
migration 12 [`012_turn_checkpoint.sql`](../store/migrations/012_turn_checkpoint.sql) (`turn.checkpoint`, replaced by `turn_checkpoint` in migration 105) and
migration 18 [`018_profile.sql`](../store/migrations/018_profile.sql) (`profile`) and
migration 32 [`032_provider.sql`](../store/migrations/032_provider.sql) (`provider`, `role.provider`) and
migration 33 [`033_channel_name_auto.sql`](../store/migrations/033_channel_name_auto.sql) (`space.name_auto`) and
migration 36 [`036_turn_scopes.sql`](../store/migrations/036_turn_scopes.sql) (`turn.scopes`, `approval.scopes`) and
migration 37 [`037_turn_session_totals.sql`](../store/migrations/037_turn_session_totals.sql) (`turn.session_cost_usd`, `turn.session_input_tokens` and the other three token counts) and
migration 38 [`038_proposal_items.sql`](../store/migrations/038_proposal_items.sql) (`proposal.items`; drops `proposal.notes` and `proposal.decisions`, clears `proposal.retire` and rejects the wrap proposals still pending) and
migration 39 [`039_turn_compacted.sql`](../store/migrations/039_turn_compacted.sql) (`turn.compacted`) and
migration 43 [`043_drop_gates.sql`](../store/migrations/043_drop_gates.sql) (drops `gate`; deletes the `approval` rows of kind `gate`) and
migration 47 [`047_space_member.sql`](../store/migrations/047_space_member.sql) (`space_member`, `space.kind`) and
migration 50 [`050_team_queue.sql`](../store/migrations/050_team_queue.sql) (one busy work item per team; dropped by migration 103) and
migration 52 [`052_team_responsibility.sql`](../store/migrations/052_team_responsibility.sql) (`team.responsibility`) and
migration 59 [`059_team_fts.sql`](../store/migrations/059_team_fts.sql) (`team_fts`) and
migration 65 [`065_send_back_note.sql`](../store/migrations/065_send_back_note.sql) (`proof.send_back_note`) and
migration 66 [`066_inbox_fyi.sql`](../store/migrations/066_inbox_fyi.sql) (`inbox_fyi`) and
migration 67 [`067_teammate_colour.sql`](../store/migrations/067_teammate_colour.sql) (`bot.colour`) and
migration 68 [`068_direct_request_state.sql`](../store/migrations/068_direct_request_state.sql) (`direct_message.state`) and
migration 69 [`069_space_moved.sql`](../store/migrations/069_space_moved.sql) (`space.moved_to`) and
migration 71 [`071_turn_wake_message.sql`](../store/migrations/071_turn_wake_message.sql) (`turn.wake_message_id`) and
migration 76 [`076_bot.sql`](../store/migrations/076_bot.sql) (renames `teammate` to `bot`, `teammate_id` to `bot_id` and `persona` to `description`) and
migration 77 [`077_member_bot_only.sql`](../store/migrations/077_member_bot_only.sql) (`team_member` and `space_member` keep only the Bot id and `lead`) and
migration 78 [`078_space_drop_agent_settings.sql`](../store/migrations/078_space_drop_agent_settings.sql) (drops `space.backend` and `space.permission_mode`) and
migration 103 [`103_assignment.sql`](../store/migrations/103_assignment.sql) (`work_item` loses `team_id` and `suggested_team_id`, gains `owner_bot_id`, `approved_at` and `approval_key`; adds `work_item_participant` and `work_item_target`).
migration 105 [`105_turn_checkpoint.sql`](../store/migrations/105_turn_checkpoint.sql) (adds `turn_checkpoint`, one ref per turn and workspace; drops `turn.checkpoint`) and
migration 106 [`106_proof_repository.sql`](../store/migrations/106_proof_repository.sql) (adds `proof_repository`, one row per repository of a submit) and
migration 107 [`107_owner_transfer.sql`](../store/migrations/107_owner_transfer.sql) (adds `work_item.pending_owner_bot_id`) and
migration 108 [`108_target_proposal.sql`](../store/migrations/108_target_proposal.sql) (`work_item_target` keeps who proposed a repository, `proposed_by`, apart from who authorized it, `authorized_by`).
Migrations 4 and 5 add columns to `space` (see [Space](#space)); migration 40
[`040_space.sql`](../store/migrations/040_space.sql) renames the `channel` table and its columns to `space`.
Migrations 41 (bot and team member), 42 (Work item) and 54 (proof) are described with their tables below.

### project

| Field | JSON | Notes |
| --- | --- | --- |
| `id` | number | |
| `name` | string | required |
| `path` | string | required; the repo folder, stored as an absolute path with symlinks resolved |
| `purpose` | string | what the Project is for, in a line; default `""` |
| `icon` | string | an emoji shown on the Project's tile; default `""`, which shows the first letter of the name (migration 110 [`110_project_icon.sql`](../store/migrations/110_project_icon.sql)) |
| `colour` | string | the tile colour as `#rrggbb`; default `""`, which picks one of the window's eight preset colours by id. The window offers only those eight |
| `createdAt` | string | |

### profile

The person using Reef: one row, no id. The window asks for the name on first run and shows it
in the sidebar.

| Field | JSON | Notes |
| --- | --- | --- |
| `name` | string | required; trimmed |
| `occupation` | string | default `""`; trimmed. The window offers Developer, Designer, Product manager, Founder, Student and Other |
| `style` | string array | default `[]`; trimmed, empty entries dropped, kept as one newline-joined column. The window offers six working preferences |
| `updatedAt` | string | |

The server stores `occupation` and `style` as given, but a line break in `name`, `occupation` or a style entry is 400 (the context prompt is built from them); the window owns the option lists. The context prompt names the person and, when set, the occupation (except `Other`) and style (see [memory.md](memory.md#the-context-prompt)).

### project_memory

One body per project, kept as a column on `project` (migration 14
[`014_project_memory.sql`](../store/migrations/014_project_memory.sql)). Nothing loads or shows it
since decision 91: the context prompt and the project page read the project's memory items instead
(see [memory.md](memory.md#the-context-prompt)). Its routes still read and write it.

| Field | JSON | Notes |
| --- | --- | --- |
| `projectId` | number | from the path |
| `body` | string | default `""` |

### work_item

One row per Work item. A Work item has a `type` that is only a label; it never changes how it behaves. One level of nesting: a sub-item has a `parentId` and cannot have sub-items.

| Field | JSON | Notes |
| --- | --- | --- |
| `id` | number | |
| `projectId` | number | required on create; never changes. Deleting the Project deletes its own Work items; a foreign item that targets it is kept, and its active Space is paused so the item moves to `needs_you` (see `DELETE /projects/{id}`) |
| `parentId` | number? | `null` for a top-level item. Must be a top-level item of the same project, else `invalid: work item 2 is a sub-item and cannot have sub-items`; never changes. Deleting the parent deletes its sub-items |
| `type` | string | `feature` (default), `bug`, `incident`, `investigation`, `task` or `improvement`, else `invalid`. A `PUT` without `type` keeps the old one |
| `title` | string | required |
| `description` | string | default `""` |
| `status` | string | `backlog` (default), `queued`, `in_progress`, `needs_you`, `review`, `done` or `dropped`, else `invalid`. `PATCH` moves it by the rules in `PATCH /work-items/{id}`. A `PUT` without `status` keeps the old one |
| `ownerBotId` | number? | the Bot that owns the top-level item and leads its Space; `null` while the item has none. A sub-item reads its parent's |
| `proposedBy` | string | default `""`; set on create only |
| `approvedAt`, `approvalKey` | string? | `null` until the user approves the item (`POST /work-items/{id}/approve`); only an approved item is queued or runs. Never set by a client; moving a queued item back to `backlog` clears both |
| `participantBotIds` | array | ids of the Bots that take part, the owner first (the owner is always one); `[]` with no owner. They become the members of the item's Space when it starts. A sub-item reads its parent's |
| `queuePosition` | number? | `null` when not queued |
| `sentBack` | bool | `true` while the user's send-back note is waiting for the lead: the item came back from review and goes first in the queue |
| `targets` | array | the repositories the top-level item may change, each `{projectId, projectName, proposedBy, authorizedBy, createdAt}`, the item's own Project first. After a target Project is deleted, its row remains and `projectName` reads `deleted project N`. `proposedBy` is the Bot that proposed the repository, `""` for the user's own; `authorizedBy` is `user` once the user approved or added it and `""` while it is only proposed. One Project is one repository. A Space's association with a Project (`space_project`) is not a target and gives no write permission. A sub-item reads its parent's |
| `tokenBudget` | number | default 1,000,000; a `PUT` with 0 keeps the old one |
| `createdAt` | string | |
| `closedAt` | string? | `null` while the item is open. Set when a top-level item is dropped; never set by a client |
| `acceptanceLines` | array | `{position, text, checked, note}` in order, `position` counting from 1. On create and `PUT`, the array replaces all lines (`position` is the index in it); a `PUT` without `acceptanceLines` keeps them. A line with no `text` is `invalid` |

A `PUT` replaces every editable field: `title`, `description`, `type`, `status`, `tokenBudget`, `acceptanceLines`, and, while the item is in `backlog`, `ownerBotId` and `participantBotIds`. A `PUT` that sends neither `ownerBotId` nor `participantBotIds` keeps them; one that changes them is `400` unless the item is a top-level item in `backlog`. `queuePosition` is the queue's: a `PUT` keeps it. `targets` are read-only here: `POST /work-items/{id}/targets` adds one.

#### Queue

A top-level item joins the queue only by approval, `POST /work-items/{id}/approve`, which needs an owner (`400` otherwise); the user's `PATCH` from `backlog` to `queued` is `400` ("approve it to queue it"). Joining the queue puts the item last in the queue, `queuePosition` 1, 2, 3; moving it from `review` back to `queued` puts it first. Nothing limits how many items run at once: a queued item that is approved and has an owner moves to `in_progress` by itself, at once on a queue and at boot for a queued item left behind. The approval or status-change response returns the claimed item as `in_progress`; opening its Space, making workspaces and starting its first turn run in the background. The server then opens an active Space for a started item that has none, led by the owner with every participant a member, and sends the lead one message: `Work item N: <title>`, `Description:` and `Brief:`, each `none` when empty, one line each; the acceptance lines are in the lead's first prompt (see [memory.md](memory.md#the-context-prompt)). An inbox `started` FYI says the item started (see [Inbox](#inbox)). A sent-back item keeps its Space and gets no new prompt. A started item whose active Space was made by hand (`reef token`, or `POST /spaces` with the item) and has no message yet gets that one message in it, once. A first turn that cannot run leaves a `reef` note saying so and pauses the Space, which puts the item in `needs_you`; `POST /spaces/{id}/continue` clears the pause and starts the lead.

Admission is Reef's, in queue order, and keyed by the repositories, not by a Crew or a Bot: items on independent trees (also two items of one owner) run at the same time. Before the first turn the server makes the item's workspaces. An item with no ready workspace (every target paused: no valid base branch, a deleted Project) is not started: its Space gets a `reef` note naming each repository's cause and is paused (`no repository of this Work item is ready`), and the scan goes on to the next item. A started item gets one Space and one first prompt: the claim is the move to `in_progress` in one transaction, the Space is unique per active item, and the first prompt is sent only while the Space has no turn and no message but `reef`'s. After a restart, a started item with no Space gets it once and one with a Space but no first turn gets it once; a paused Space is left as it is. See [Restart recovery](#restart-recovery) for a cut turn.

The owner's `submit_for_review` runs in the `reef mcp` process, which the server cannot hear, so the server learns of a submit when the chain of turns of the submitting Space ends. It then opens the Space of any item the queue started meanwhile, the same way as after a `PATCH`, and runs a wrap-up turn for the lead (the same turn as `POST /spaces/{id}/wrap`), once per submit. A wrap-up that cannot start leaves a `reef` note in the Space: `Submitted for review, but the wrap-up did not start: <why>. Send /wrap to run it.` A submit that happens while the server is down, or a quit before the wrap-up started, starts its wrap-up at the next boot: the wrap-up runs for each item in review whose Space is open and has not had one.

#### proof

What the owner submitted a top-level item with (`submit_for_review`, see [memory.md](memory.md#reef-mcp)). One row per item, replaced by each submit; a sent-back item is submitted again. No REST write.

| Field | JSON | Notes |
| --- | --- | --- |
| `spaceId` | number | the Space the owner submitted from; the wrap-up runs there |
| `acceptanceLines` | array | the item's lines as they are now, with `checked` and `note` as the submit set them |
| `commands` | array | `{"command", "output"}` each, as submitted, that name no repository; `[]` when none |
| `repositories` | array | one per workspace of the item at submit, in workspace order: `{"workspaceId", "projectId", "projectName", "branch", "head", "baseRef", "dir", "dirty", "checks"}`. `projectName` is `deleted project N` if that Project was later deleted. `checks` are the submitted commands whose `project` is that Project's name. No PR or MR field: opening one is the provider slice; the window shows `Not opened yet` |
| `screenshots` | array | absolute paths of files under the data folder; `[]` when none |
| `submittedAt` | string | RFC 3339 |

The row also keeps whether the wrap-up was started (`wrap_started`), which is not in REST. The PR of the proof is the item's branch PR: `GET /work-items/{id}/pr`. Migration 54 [`054_proof.sql`](../store/migrations/054_proof.sql); the repositories are the `proof_repository` table (one row per workspace, replaced by each submit), migration 108.

### work_item_github

The last GitHub PR state the [GitHub poll](#github-poll) posted about a Work item, kept across
a restart so it does not repeat a note. No REST route; tables `work_item_github` (one row per
item: `pr_number`, `pr_state`, `pr_url`) and `work_item_github_check` (one row per
`(work item, check name, run key)` of the item's PR: `run_key`, `woken`; a row's existence
means it was noted), migration 42
[`042_work_item.sql`](../store/migrations/042_work_item.sql).

### brief

One row per version. A work item's brief is its latest version.

| Field | JSON | Notes |
| --- | --- | --- |
| `id` | number | version order |
| `workItemId` | number | from the path |
| `body` | string | required |
| `author` | string | required |
| `createdAt` | string | |

### role_notes

One row per work item and role. Nothing reads them since decision 91: a role's own items are in
scope `bot`. Their routes still read and write them.

| Field | JSON | Notes |
| --- | --- | --- |
| `workItemId` | number | from the path |
| `role` | string | from the path |
| `body` | string | may be empty |
| `updatedAt` | string | |

### Space

| Field | JSON | Notes |
| --- | --- | --- |
| `id` | number | |
| `kind` | string | `work_item` (the one active Space of a work item), `free` (no work item), `conversation` (a direct conversation with one Bot) or `consultation` (one Bot asked by another Bot's conversation, see [Consultations](#consultations)). Set by the server |
| `projectId` | number | the work item's project for a Space of a work item, and then it never changes. For a free Space or a conversation it is the Space's lowest-id associated project, `0` when it has none: it is optional on create and the association is kept in `space_project` (see [Space projects](#space-projects)) |
| `workItemId` | number? | set on create. It changes only from `null`, once, when [attach](#attach-a-space-to-a-work-item) gives a free Space its work item. `null` is a free Space or a conversation. Must be a work item of the project given with it, and a work item has one active Space (`409` for a second). Deleting the work item deletes the Space |
| `name` | string | required |
| `role` | string | the current name of the Space's lead Bot, read live so a rename shows at once. A message's or turn's own `author` or `role` stays as recorded |
| `botId` | number? | the id of the Space's lead Bot, from its member rows. The window matches faces and working state by it, never by `role` |
| `paused` | string | why the Space is paused (see [Caps](#caps)). Empty when it is not paused. Set by the server only; a client cannot set it |
| `tokenBudget` | number | tokens the Space may use before it pauses: its top-level Work item's `tokenBudget`, shared with the item's other Spaces, or its own when it has no item. Starts at 1,000,000. Set by the server only |
| `tokensUsed` | number | only in `GET /spaces/{id}`: the tokens counted against `tokenBudget` (see [Caps](#caps)) |
| `team` | string | the name of the Team the Space's members were copied from. `solo` when none is given. Empty for a Space made from one Bot. Set on create; deleting the Crew clears it to `""`. The Space keeps its own member list (`space_member`: Bot ids and the lead flag), so editing the Team later does not change a running Space. A Space has no `backend` or `permissionMode`: every turn runs on its Bot's profile |
| `createdAt` | string | |
| `closedAt` | string? | `null` while the Space is open. Set when its wrap-up is approved and when its work item is dropped, cleared by `POST /spaces/{id}/reopen`. A closed Space takes no turn (409) |
| `movedTo` | number? | `null` unless the Space is a conversation that [moved](#spaces-live) to the Space with the id. A moved conversation takes no turn (409) |

### Space projects

A Space with no work item belongs to no project until it is associated with one: zero, one or
several, in the `space_project` table (`space_id`, `project_id`). The association is context. It is
neither a working directory nor permission to change a repository, and filters never change it.

- `POST /spaces` with `projectId` associates the new Space with it at once; with no `projectId` and no
  work item the Space starts with none. A Space of a work item is associated with the item's project.
- `GET /spaces?project=ID` lists the Spaces associated with the project, so a Space with two projects
  is listed under both. `GET /spaces/{id}/projects` lists a Space's projects. `PUT` and `DELETE
  /spaces/{id}/projects/{projectId}` add and remove one (a Space of a work item is 400: it stays in its
  work item's project); each change that does something writes a note by `reef` in the Space's history, `You associated Project X with this conversation.` or `... removed Project X from this conversation.` (a Bot's change through the MCP tools carries its `botId`). The Space's messages, turns and sessions do not change when its projects do.
- Moving a conversation to a new Space (`@name`) copies its projects to the new Space.
- A Space's context prompt names the projects as `Associated projects, for context only; you do not
  work in them: <name> (<id>), ...`, never their folders. A Space with no project has no `project`
  memory scope and cannot `propose_work_item`.
- Attach (`POST /spaces/{id}/attach`) takes any top-level work item, whatever the Space is
  associated with: the Space moves to the work item's project and is associated with it too, and its
  other associations stay. Its `space` memory items move to the work item only when no other Project is associated; otherwise they stay in the Space scope.

### pin

Migration 82 [`082_pin.sql`](../store/migrations/082_pin.sql). `kind` (`bot`, `chat`, `space`),
`target_id` (no foreign key, since it points at a Bot or a Space; triggers delete the pin with its
target) and `createdAt`; the key is `(kind, target_id)`.

### skill

Migration 84 [`084_skill_library.sql`](../store/migrations/084_skill_library.sql). `skill` is one library
package: `name` (unique), `description`, `source_kind` (`folder` or `git`), `source`, `ref`, `dir`,
`revision` and the current `version`. `skill_file` holds `(skill_id, version, path, body, executable)`
(migration 95 added `executable`, set when a file read from disk has an execute bit; a file written in
Reef is not executable, and an edit keeps the bit of a file whose path stays), one row per
file of a version; a version is never changed, an update adds the next one, and deleting the skill
deletes its files. Migration 92 [`092_skill_draft.sql`](../store/migrations/092_skill_draft.sql) lets
`source_kind` be `reef` (a skill written in Reef, with empty `source`, `ref`, `dir` and `revision`) and
adds `skill_draft`: a skill a Bot proposed with `propose_skill`, with its `space_id`, `bot_id`, `name`,
`files` (JSON `[{path, body}]`), `status` (`pending`, `saved` or `rejected`), `skill_id` once saved, and
`created_at` and `decided_at`. A draft is not in the library and no Bot's skills mention it.

### message

| Field | JSON | Notes |
| --- | --- | --- |
| `id` | number | |
| `spaceId` | number | required |
| `parentId` | number? | the id of the thread root this message replies to. `null` for a top-level message. It must be a top-level message of the same Space: a reply to a reply is 400. Set on create |
| `author` | string | required; free text, for example `user` or a role |
| `body` | string | required |
| `createdAt` | string | |
| `exchangeId` | number? | the exchange (direct request) this message stands for, in the origin conversation: the note that says it was asked, and the reply posted when it was answered. The window links both sides |

A top-level message is the root of a thread. The author of the root owns the thread.

### turn

One agent run in a Space.

| Field | JSON | Notes |
| --- | --- | --- |
| `id` | number | |
| `spaceId` | number | required |
| `role` | string | required |
| `backend` | string | required; `claude` or `codex` |
| `sessionId` | string | default `""`; set on create, while the turn runs, or when it ends |
| `status` | string | `running`, `done` or `failed`. A new turn is always `running` |
| `error` | string | default `""` |
| `usage` | object | `inputTokens`, `outputTokens`, `cacheReadTokens`, `cacheWriteTokens` (numbers, all four always present). The SQL columns are `input_tokens` and so on. The turn's own tokens: for claude, the session total the turn reported minus the total the session's previous turn reported (see `sessionUsage`), so a resumed turn's subagent and side-call tokens count; a running turn holds the sum of its messages so far |
| `startedAt` | string | |
| `endedAt` | string? | `null` while running |
| `wakeText` | string | default `""`; the text the turn ran: the unread messages of the wake, without the context prompt. Empty for a turn from before this field existed and for a wrap-up turn |
| `wakeParent` | number? | the thread the wake was in, `null` for the top level. Set on create with `wakeText` |
| `directMessageId` | number? | the direct request or response the turn was woken by; `null` for a public wake. Kept so retry preserves private reply routing |
| `retried` | boolean | `true` once a retry of this failed turn has started. Set by `retry`, never on create |
| `checkpoints` | object[] | one `{"workspaceId", "projectId", "ref"}` per workspace snapshotted before the turn, `ref` being `refs/reef/checkpoints/<data folder>/<turn id>/<workspace id>` in that workspace's repository (worktrees of one repository share their refs, so each workspace has its own). `[]` for a turn of a role without `write` and without a file-changing shell command in its `tools` (see [Spaces live](#spaces-live)), for a turn of a Space with no work item and no quick fix, and for a workspace whose repository has no commit. Set by the server, never by a request |
| `model` | string | default `""`; the backend's model name, set from the agent's first event once the turn ends |
| `costUsd` | number? | `null` until the turn ends with a cost; codex turns never get one. A failed turn gets one too, when claude reported one. The turn's own cost: the session's running total the turn reported minus the total the session's previous turn reported. A session stored before totals were kept (all its turns have `sessionCostUsd` null) is taken to have cost, so far, its largest `costUsd` plus the `costUsd` of the turns stored after it: a resumed process held a running total, a long-lived one stored its first total and then each turn's own share |
| `sessionCostUsd`, `sessionUsage` | number?, object? | not in the turn's own JSON; the running totals of the turn's whole session as claude reported them when the turn ended (`sessionUsage` has the four token counts). `null` for a turn from before migration 37 [`037_turn_session_totals.sql`](../store/migrations/037_turn_session_totals.sql), for a turn that ended with no report (a stopped or crashed turn) and for every codex turn. The SQL columns are `session_cost_usd`, `session_input_tokens`, `session_output_tokens`, `session_cache_read_tokens`, `session_cache_write_tokens`. A turn's usage is its total minus the latest total before it plus the `usage` of the stopped turns after that one, so a stopped turn's tokens are not counted twice |
| `prompt` | string | not in the turn's own JSON (`GET /turns`, `GET /turns/{id}`); the full text sent to the agent (context preamble and notes included), stored when it is sent, so a running turn has it; a turn whose send fails stores it when it ends. Empty for a turn that failed before its prompt was built. `GET /spaces/{id}/inspector` has its own view of it |
| `reply` | string | not in the turn's own JSON; the plain reply of a done turn that already posted with `post_to_space` (only the text before its last post when work followed it and the rest was posted), kept here and not stored as a message (see [Routing](#routing)). `""` for every other turn. `GET /spaces/{id}/inspector` has its own view of it |

### tool_call

One tool call of a turn. Input and result are cut to 4 KB each. There is no update or
delete; deleting the turn deletes its tool calls.

| Field | JSON | Notes |
| --- | --- | --- |
| `id` | number | |
| `turnId` | number | required |
| `name` | string | the tool's name |
| `input` | string | cut to 4 KB |
| `result` | string | default `""` until the call's result arrives, then cut to 4 KB |
| `parentId` | number? | the `Agent` call a claude subagent's call nests under, set from the event's `parentToolId`; absent for a top-level call, and for a call whose parent was not seen (shown flat) |

### decision

No agent, CLI command or window page reads or writes this table since decision 91: a decision is a
memory item of kind `decision` (`remember`, `reef decide`, `POST /memory`). Its routes still work.

| Field | JSON | Notes |
| --- | --- | --- |
| `id` | number | |
| `workItemId` | number | required |
| `messageId` | number? | optional. Must be a message in a Space of the same work item. `null` if that message is later deleted |
| `body` | string | required |
| `why` | string | default `""` |
| `author` | string | required |
| `createdAt` | string | |
| `retiredAt` | string? | `null` while the decision is live. Set when it is retired: stale, duplicate or replaced. A retired decision is never edited or deleted, and still reads |
| `replacedBy` | number? | the decision that replaced it, if one did; `null` for a plain retire |
| `replaces` | array? | input to create, echoed in its response and never in a read: ids of live decisions of the same work item that the new decision retires, with their `replacedBy` set to it. Any other id is `invalid: decision 9 is not a live decision of work item 1` and nothing is written |

### team

A reusable roster of existing Bots, shown as a Crew. A team has at most one lead, its suggested coordinator. `reef
team apply` (see [memory.md](memory.md#teams)), `PUT /teams/{name}` and `POST /teams` all take Bot ids and a lead
flag only, through the same validation: they never create a Bot and take no per-member configuration. The
seed team `solo` has one member, the Bot `lead`; it cannot be deleted or have members changed.

| Field | JSON | Notes |
| --- | --- | --- |
| `id` | number | |
| `name` | string | required; unique; letters, digits, `_` and `-` only. Applying a name that exists replaces that team's members in one transaction |
| `responsibility` | string | default `""`; what the team is for. Nothing else reads it |
| `roles` | array | the members, the lead first, then the others in the order given. A request sends `{"botId", "lead"}` for each, nothing else; a response shows each as a [role](#role) read from its Bot |

Deleting a team (`DELETE /teams/{name}`) is `invalid` for `solo`. Its Bots, its Spaces and their members stay; its Spaces lose the team link.

### bot

One persistent agent with one global profile, which can sit in several teams and Spaces. Migration 41
([`041_teammate.sql`](../store/migrations/041_teammate.sql)) added it as `teammate`; migration 76 renamed it
`bot`. The seed is one default bot `lead` (`claude`, `write`), the lead of `solo`.

| Field | JSON | Notes |
| --- | --- | --- |
| `id` | integer | the id of its memory `bot` scope, so a rename keeps its items |
| `name` | string | required; unique; same rule as a role name |
| `description` | string | what the bot is for, in plain words. It goes at the start of the bot's first prompt |
| `skills`, `backend`, `model`, `provider`, `face`, `colour` | | as a [role](#role) |
| `tools`, `write`, `permissionMode` | | the bot's access rules, read at every start of its session from every team and Space it sits in |
| `connections` | array of integers | the ids of the [connections](#connection) the bot selects. None are selected until you pick them: a connection is never on for every Bot. An unknown id is `invalid` |

A bot that sits in a team cannot be deleted: the error is `invalid:` and names the team. A team has at
most one lead; a second one is `invalid`. `solo` keeps its one lead and takes no member changes.
A `team_member` or `space_member` row holds only the bot id and `lead`. A member's role name is the bot's
name, so renaming the bot renames the role everywhere and leaves messages, memberships and turns pointing at
the same id; messages and turns keep the name they were written under. Team requests never rewrite a bot:
edit its profile with `PUT /bots/{id}`.

### role

A bot as it sits in a team or Space: the fields of its profile, plus `lead` from its membership. A role has no
setting of its own.

| Field | JSON | Notes |
| --- | --- | --- |
| `name` | string | the bot's name: unique; letters, digits, `_` and `-` only, and not `user` or `reef`. It is the `author` of what the role posts, and the name an `@mention` uses |
| `lead` | boolean | exactly one role of a team is the lead. The lead answers a message that nobody else owns |
| `write` | boolean | default `false`. Only a role with `write` may change files; it gets a checkpoint and the tree's write lock. claude: without it the role never gets `Edit`, `Write`, `MultiEdit` or `NotebookEdit` (they are dropped from `tools` and passed as `--disallowedTools`, so an allow rule in the user's or project's `.claude/settings.json` cannot grant them) and runs in the `default` permission mode, so `Bash` runs only as `tools` lists it. A claude role without `write` cannot list plain `Bash`, `Bash(*)` or `Bash(:*)` in `tools`: it would still change files through the shell, so saving the bot is `invalid`; list patterns such as `Bash(go test:*)` instead. A role saved before this check existed has it dropped from `tools` at session start too, the same as the edit tools. `Bash` patterns are trusted as given, so `Bash(git diff:*)` also lets `git diff --output=file` write a file: such a role counts as a writer (see the write lock below). With it the role keeps its `tools` and permission mode (it still needs `acceptEdits`, or `Edit` in `tools`, to edit unasked) and also gets `Task`. codex: without it the sandbox is `read-only`; with it `workspace-write` unless the role's `permissionMode` is set. The seeded `solo` lead has `write` |
| `backend` | string | required; `claude` or `codex` |
| `permissionMode` | string | `""` is the backend default; claude: `default` or `acceptEdits`; codex: `read-only` or `workspace-write` |
| `model` | string | optional; the model the role runs on, passed to the backend when its agent starts or resumes: claude `--model NAME` (`opus`, `sonnet`, `haiku` or a full model id), codex the thread `model`. Left out or `""` is the backend default |
| `provider` | string | optional; the `name` of an added [provider](#provider) whose binary the role runs instead of the `claude` or `codex` on `PATH`. It must exist and have `kind` equal to the role's `backend`, else the bot is `invalid`. Left out or `""` runs the one on `PATH` |
| `description` | string | goes at the start of the role's first prompt, under `## You`, and again, marked as changed, at its next wake once it or `skills` differs from what the role's session last got |
| `skills` | array of strings | skill refs, one per entry, trimmed: `reef:name`, `claude:name` or `codex:name` (see `GET /skills`); each is letters, digits, `:`, `_` or `-`, and an empty, repeated, source-less or other ref is `invalid`. Each is copied when the Bot adopts its settings in a Space and listed in the session instructions (see [memory.md](memory.md#the-context-prompt)), on claude and codex alike. claude: a role that picks skills uses only these copies. `--disallowedTools` gets `Skill(name)` for every claude skill on disk, picked or not, so the Bot uses the copy its instructions list and not the live file (no pick gives a `--allowedTools` rule) (the scan of `GET /skills`, taken when the role's session starts, with the role's working folder). A role that picks none keeps every skill and gets no skill rule. Skill rules only add entries to the `write` and `tools` rules. codex: no rule; it finds `~/.agents/skills` by itself (see [agents.md](agents.md#skills)). A bot body that gives `skills` as a string is `invalid` |
| `face` | string | optional; the creature the window draws for the role: `jellyfish`, `whale`, `penguin`, `shark`, `octopus`, `turtle`, `seahorse`, `otter`, `clownfish`, `manta-ray` or `pebble`. Left out, the window picks one by the role's name (`lead` jellyfish, `architect` whale, `developer` penguin, `tester` shark, else `pebble`). Any other value is `invalid` |
| `colour` | string | optional; the creature's body colour: `orange`, `red`, `yellow`, `green`, `teal`, `blue`, `violet` or `pink`. Left out, the creature's own. Any other value is `invalid` |
| `tools` | array of strings | claude: passed as `--allowedTools` with `mcp__reef__*`, so these tools and the reef tools run without asking; the tools of a selected connection do not (list `mcp__<connection name>__*` here to allow one). codex: ignored |

Every member of a Space, its lead included, runs with the backend and permission mode of its bot.

A Bot adopts its settings in a Space at the Bot's first turn there: table `bot_session` keeps one row per adoption, with the Bot's `backend`, `provider`, `model`, `description`, `skills`, `tools`, `connections`, `write` and `permissionMode` as they were (`connections` holds each selected connection in full, name and settings, so editing or deleting a connection later changes no session that has it), a `status` (`active`, `replaced` or `revoked`) and `createdAt` and `endedAt`. `skills` holds one entry per pick, `{ref, description, version, dir, compatible, reason}`: the skill's files are copied into `dir` (`<data>/bot-sessions/<id>/skills/<source>/<name>/`, relative paths kept, `0700` for a file that had an execute bit and `0600` otherwise) in the same transaction that makes the row, so a failed copy leaves the previous row active, a `reef:` pick at its library version (`version` is that number) and a `claude:` or `codex:` pick as read from disk (`version` is a hash of its files; the originals are only read). A pick that uses `CLAUDE_` plugin variables is not `compatible` with codex and a pick that is missing is not compatible with either; neither has a `dir`, and `reason` says why. Later library updates or disk edits change no existing row's copy: only a new row, from a new Space or a refresh, copies again. A renewal keeps its row. A Space has at most one `active` row per Bot, and each turn stores its row as `botSessionId`. A turn runs with the active row, not with the Bot: editing the Bot, a restart of the server, an automatic renewal of a lost backend session and a running agent change nothing, and the agent is not restarted. Resume looks only at turns of the active row. A Space that adopts the Bot later, such as a new conversation, gets the Bot's settings as they are then. `POST /spaces/{id}/bots/{botId}/refresh` is what replaces the active row with the Bot's current settings.

### connection

A tool connection set up once in Reef: an MCP server a Bot is offered when it selects the connection. It is a local process (`command`, `args`, `env`) or an HTTP server (`url`), exactly one of the two. Both claude and codex take both kinds. It names the server the backend shows the tools under: `mcp__<name>__<tool>`. `name` is unique, letters, digits, `_` or `-`, and not `reef`, which is the reef server. A connection is only tools offered to the Bots that select it: it starts no listener and no Channel, and gives no authority to send anything anywhere. A session gets the connections its Bot selected when the session adopted its settings, next to the `reef` server; changing a Bot's selection reaches new sessions and a Refresh, not a session that is running. REST masks secrets: each `env` value and each `args` entry is `****`, and so are the userinfo and the query of `url`. `PUT` sends the mask back to keep the stored value (an `env` key or an `args` position with no stored value behind a mask is `invalid`; args are matched by position), and a local process saved with no `env` keeps its stored one. `bot_session` rows keep the full configuration, and no REST response shows it.

### provider

A claude or codex install the user added, next to the ones found on `PATH`. The ones on `PATH`
are not stored: `GET /providers` finds them each time, with `exec.LookPath`, the same lookup the
adapters use when a role names no provider.

| Field | JSON | Notes |
| --- | --- | --- |
| `name` | string | required; unique; letters, digits, `_` and `-` only, and not `claude` or `codex` (those name the ones on `PATH`) |
| `kind` | string | required; `claude` or `codex` |
| `folder` | string | required; an absolute path to an existing folder that holds an executable named like the kind: `FOLDER/KIND`, else `FOLDER/bin/KIND` |
| `createdAt` | string | |

A role runs a provider's binary by naming it in `provider`. The check runs when the bot is
saved (the provider exists, its `kind` is the role's `backend` and its folder still holds the
binary) and again when the role's agent starts, because the binary may be gone from the folder
after that. A start that fails the check
is a `failed` turn whose `error` says why, for example `role developer: no claude binary in
/opt/work`; nothing falls back to the `PATH` binary, as that would run a different one than the
user picked. A role with no `provider` is unchanged. A provider a role names cannot be removed.

### approval

A wake that a pause holds. The SQL table is `approval`; its rows have kind `hold`.

| Field | JSON | Notes |
| --- | --- | --- |
| `id` | number | |
| `spaceId` | number | |
| `from` | string | the role whose message woke it |
| `to` | string | the role that would run |
| `messageId` | number | the message that woke it; `0` for a direct wake |
| `directMessageId` | number? | the direct message that woke it, when the wake is direct |
| `text` | string | the wake as it was routed, for example `[Space 5, message 28]\nlead wrote: @architect design a form`; the turn's own text is built when it runs |
| `status` | string | `pending`, or `approved` once `continue` has claimed it, or `rejected` once a user message dropped it |
| `createdAt` | string | |

A hold is never listed or decided through the API, see [Caps](#caps). Deleting the Space
deletes its approvals.

### token

Lets one bot's `reef mcp` act as one role in one Space. No REST route: `reef
token`, `reef chat` and the server (one per bot session) make them, `reef mcp` reads
them, and the one that made a token deletes it when its session ends. See
[memory.md](memory.md#tokens).

| Field | JSON | Notes |
| --- | --- | --- |
| `token` | string | the key; 64 hex characters from 32 random bytes |
| `spaceId` | number | required. Deleting the Space deletes the token |
| `role` | string | required; the author of what the bot writes |
| `createdAt` | string | never expires |

### proposal

A wrap-up proposal: the new brief and the memory items to save.
`propose_wrap` makes one. `propose_brief` makes a brief proposal: only the brief, made
mid-space. Approving it adds a brief version and leaves the memory items and the
Space as they are; the Space's agent is not stopped. A Space has one pending proposal at most: a new wrap rejects the older pending ones of any kind, a new brief rejects the older pending briefs and is refused with `invalid: space 1 has a wrap-up waiting for approval` while a wrap is pending, and approving a brief on a closed Space is `invalid: space 1 is closed`. `reef chat` prompts for approval,
and `reef wrap approve` or `reef wrap reject` decides it; the server offers the same over REST (see [Wrap-up and
attach](#wrap-up-and-attach)). See [memory.md](memory.md#wrap-up).

| Field | JSON | Notes |
| --- | --- | --- |
| `id` | number | |
| `spaceId` | number | required; must be a Space of a work item. Deleting the Space deletes the proposal |
| `role` | string | required; author of the brief and items it applies |
| `kind` | string | `wrap` (default) or `brief`; any other is `invalid: unknown proposal kind "x"` |
| `brief` | string | required, not empty; the full new brief |
| `items` | array | `{"kind", "body", "why", "scope", "replaces"}` each; `body` required. `scope` is `work_item` (the default `propose_wrap` fills in), `project` or `bot`. `replaces` is optional, ids of live memory items it replaces. `[]` by default. The SQL column holds them as JSON text. Approving saves each with `remember`: see [memory.md](memory.md#wrap-up) |
| `retire` | array | ids of live memory items that are stale or duplicate; approving retires them. `[]` by default. The SQL column holds them as JSON text. `propose_wrap` checks that each `retire` and `replaces` id is an accepted item in a scope the role can read (`invalid: memory 9 is not a live item in this space's scopes`) and stores nothing otherwise; `CreateProposal` itself does not read memory. An id claimed twice (in `retire` and a `replaces`, or two `replaces`) is kept for its first claim, replaces before `retire`. At approve an id that is already retired is skipped |
| `status` | string | `pending` (default), `approved` or `rejected`. Decided once |
| `createdAt` | string | |
| `decidedAt` | string? | `null` while pending |

### space_proposal

A Bot's suggestion for a new Space, made with the `propose_space` tool (migration 81
[`081_space_proposal.sql`](../store/migrations/081_space_proposal.sql)). It creates no Space: it
shows in the inbox, the user accepts or changes it in the new Space form, and dismissing the
inbox item deletes it.

| Field | JSON | Notes |
| --- | --- | --- |
| `id` | number | |
| `proposedBy` | string | required; name of the proposing Bot |
| `name` | string | required, not blank |
| `botIds` | array | ids of the Bots, de-duplicated; at least one, each must exist (`invalid: unknown bot 9`). The SQL column holds them as JSON text |
| `coordinatorBotId` | number | one of `botIds`; the first when sent as 0 |

## REST

`{id}` is a number. Lists come back as `[]` when empty, ordered by id (oldest first).
Create returns 201 and the row. Update returns 200 and the row. Delete returns 204 and no
body. Deleting a row deletes what hangs off it by foreign key: a project takes its
work items, Spaces; a work item takes its briefs, notes, decisions and Spaces;
a Space takes its messages, turns, tokens, proposals and approvals; a turn takes its
tool calls.

| Route | Filters and body | OK | Notable errors |
| --- | --- | --- | --- |
| `GET /health` | | 200 `{"status":"ok"}` | |
| `GET /status` | | 200 `{"running":N}`, N the Spaces whose live state is not idle (a turn is starting, running or stopping); the desktop app polls it to update when idle | |
| `GET /projects` | | 200 | |
| `POST /projects` | `name`, `path`, `purpose` (optional, plain words), `icon` and `colour` (optional) | 201 | 400 missing field, or a `path` that is not an absolute path to an existing folder |
| `GET /projects/{id}` | | 200 | 404 |
| `PUT /projects/{id}` | `name`, `path`, `purpose` (optional; empty clears it), `baseBranch` (optional; the branch a new work item starts from and its PRs target; empty means the repo's default branch), `icon` and `colour` (optional; empty clears them) | 200 | 400 (same `path` rule, a `baseBranch` that is not a branch name, or a `colour` that is not `#rrggbb`), 404 |
| `DELETE /projects/{id}` | | 204; its own Work items are deleted. Every workspace directory of the Project and its own items, and checkpoint refs taken there, are removed; branches are kept. A Space with no Work item stays without this Project. A foreign Work item that targets this Project is kept; if it has an active Space, that Space is paused and the item moves to `needs_you` with a note. Its target and workspace rows stay as history: the target name reads `deleted project N`, and the workspace row has state `removed` with note `project <name> was deleted`, although its directory is removed. | 404, 409 a workspace has uncommitted changes or a targeted item is running (nothing is removed) |
| `GET /projects/{id}/memory` | | 200; `body` is `""` when none has been set. Unused since decision 91 | 404 |
| `PUT /projects/{id}/memory` | `body`; replaces the old memory | 200. Unused since decision 91 | 404 |
| `GET /profile` | | 200 | 404 no name set yet |
| `PUT /profile` | `name`, `occupation`, `style`; replaces the old profile | 200 | 400 empty name or a line break in a field |
| `GET /work-items` | `?project=ID`, `?parent=ID`, `?status=S`, `?type=T` | 200, with each item's `acceptanceLines` | 400 bad id, status or type |
| `POST /work-items` | `projectId`, `parentId`, `type`, `title`, `description`, `acceptanceLines`, `ownerBotId`, `participantBotIds` and the other fields above; `status` and `queuePosition` are ignored: a new item is in `backlog`, `PATCH` moves it | 201 | 400 unknown project, unknown parent, a parent that is a sub-item, bad type, unknown Bot, an owner or participants on a sub-item |
| `GET /work-items/{id}` | | 200 | 404 |
| `PUT /work-items/{id}` | the editable fields above; `status` is ignored, `PATCH` moves it | 200 | 400 (owner or participants changed while the item is not in backlog, or on a sub-item), 404 |
| `PATCH /work-items/{id}` | `status`, and `note` to send back; the user moves the item: queued to backlog (which withdraws the approval; backlog to queued is `POST /work-items/{id}/approve`), review to done (top-level) or back to queued, any status to dropped (a top-level item that drops stops its running turns, closes its Spaces and sets its `closedAt`; a closed Space or one of a dropped item takes no turn, 409). Review to done approves the Space's pending wrap proposal first (R44), never another kind; with none it closes the Space. An item in review with no proof is only moved. Review to queued needs a `note`: it rejects that wrap-up, and the lead gets the note as a user message when the queue restarts the item. Either is 409 while a turn runs in the Space. A top-level item with an owner that is approved starts at once (see Queue). The queue, the Space's pause and continue, `submit_for_review` and the lead move it by their own rules (R10, R41, R47) | 200 | 400 `invalid: work item N: <rule>` for a move the user may not make, or a parent to review while a sub-item is open; 404 |
| `PUT /work-items/{id}/queue-position` | `position`, 1 to the number of queued items; the item takes that position and the items between shift by one | 200 the item | 400 the item is a sub-item or not queued, or `position` is outside the queue; 404 |
| `POST /work-items/{id}/approve` | `key` (required: one value per approval, a retry or double click sends the same), `ownerBotId`, `participantBotIds` and `targetProjectIds` (what the user decided; each left out keeps the proposal's; the owner is always a participant, and the item's own Project is always a target). Only a top-level item in `backlog` with an owner. One transaction sets owner, participants, `approvedAt` and `approvalKey`, keeps exactly the targets in `targetProjectIds` (a Project not yet a target is added) and authorizes them as the user's, dropping every proposed target left out, queues it last and starts the queue. The same key again returns the item as it is, with no second queue entry or Space. Adding a Bot to one of the item's Spaces (the user, or approving a member proposal) makes it a participant | 200 the item | 400 no key, no owner, unknown owner or participant Bot, unknown Project, a sub-item; 404; 409 the item is approved under another key or is not in `backlog`, or the key belongs to another item |
| `PUT /work-items/{id}/participants` | `ownerBotId` (number or `null`), `participantBotIds`; the owner is added to the participants when missing. Only a top-level item in `backlog` | 200 the item | 400 unknown Bot, participants with no owner, a sub-item, an item not in backlog; 404 |
| `POST /work-items/{id}/owner` | `botId`; the user hands a top-level item that is not `done` or `dropped` to that Bot. The answer is `{workItem, pending, note}`. With no Space, or no turn running in the item's Space, one transaction makes the Bot the owner and a participant, adds it to the Space if it is not a member, makes it the Space's only coordinator and stores a handover: a user guidance exchange addressed only to the new owner with request key `owner:<item>:<bot>:<n>` and a message in the Space `Ownership moved from <old> to <new>.`, so a retry or restart delivers it once. The old owner stays a member and gets no wake. While a turn runs, `pendingOwnerBotId` is set and the transfer applies when the turn ends, or at the next start if the server restarts first (the Space is then paused with the handover waiting; continue delivers it); a Stop applies it at once. A later request replaces a pending one, and one for the current owner withdraws it. Requests already addressed to a Bot (exchanges, holds) keep that Bot as their destination; a message with no mention goes to the new coordinator after the transfer. Workspaces, branches and targets do not change. `PUT /spaces/{id}/coordinator` never changes the owner | 200 applied; 202 pending (`note` `applies after the current turn`) | 400 no `botId`, unknown Bot, a sub-item, a `done` or `dropped` item; 404 |
| `GET /work-items/{id}/targets` | | 200 the item's targets (its parent's for a sub-item) | 404 |
| `POST /work-items/{id}/targets` | `projectId`; the user adds the Project's repository to a top-level item. The row records `authorizedBy` `user`. If the item has an active Space it gets a `reef` message `Added repository <project> to this Work item, authorized by user.` Workspace setup then runs in the background; if it is ready, a second `reef` message says `Repository <project> is ready: folder <folder>/ under your working folder, on branch <branch>, base <base>.` | 201 the target | 400 unknown project, a sub-item; 404; 409 the Project is already a target |
| `DELETE /work-items/{id}` | | 204; the work item's workspaces and the checkpoint refs of its turns are removed, its branches are kept | 404, 409 a workspace has uncommitted changes (nothing is removed) |
| `GET /work-items/{id}/brief` | | 200 latest version | 404 no work item, or no brief yet |
| `PUT /work-items/{id}/brief` | `body`, `author` | 201 new version | 400, 404 |
| `GET /work-items/{id}/brief/versions` | | 200 oldest first | 404 |
| `GET /work-items/{id}/notes/{role}` | | 200. Unused since decision 91 | 404 no work item, or no notes for the role |
| `PUT /work-items/{id}/notes/{role}` | `body`; replaces the old notes | 200. Unused since decision 91 | 404 no work item |
| `DELETE /work-items/{id}/notes/{role}` | | 204. Unused since decision 91 | 404 no work item, or no notes for the role |
| `DELETE /work-items/{id}/targets/{projectId}` | the user takes a repository off a top-level item in `backlog` | 204 | 400 the item's own Project, a Project that is not a target, a sub-item; 404; 409 the item is not in `backlog`, or the Project has a workspace |
| `GET /work-items/{id}/workspaces` | | 200 the [workspace](#workspaces) rows of the item's targets, oldest first, `[]` before any is made (a sub-item answers with its parent's). Never makes one | 404 |
| `POST /work-items/{id}/workspaces` | no body needed | 200 the rows of every target after making each missing workspace: `[{"id", "projectId", "workItemId", "spaceId", "name", "dir", "branch", "base", "baseRef", "baseSource": "origin"\|"local", "state": "creating"\|"ready"\|"paused", "note", "createdAt", "updatedAt"}]`. A ready workspace is left alone; a `creating` or `paused` one is tried again in the same row and folder | 404; 409 a folder taken by another workspace; 500 a git failure not about the repository, after the other targets were made (that row stays `creating` with the error as `note`) |
| `GET /work-items/{id}/diff` | query `scope=current` for the changes against each workspace's HEAD only | 200 the changes of every ready workspace against the commit its branch and the project's base branch last shared, uncommitted and untracked files included, as `[{"path", "status": "added"\|"modified"\|"deleted", "hunks": [{"header", "lines": [{"kind": "add"\|"del"\|"ctx", "old", "new", "text"}]}]}]`; a path starts with its workspace's folder (`<project id>-<project name>/`), so each repository is a section of its own; `[]` when the work item has no workspace yet | 404 |
| `GET /work-items/{id}/proof` | | 200 the [proof](#proof): `{"spaceId", "acceptanceLines", "commands", "repositories", "screenshots", "submittedAt"}` | 404 the item was never submitted for review |
| `GET /work-items/{id}/proof/screenshots/{n}` | | 200 the image of screenshot `n` (from 0) of the proof | 404 no proof, no such screenshot, or the file is not an image under the data folder |
| `GET /work-items/{id}/pr` | | 200 `{"pr": {"number", "title", "state": "OPEN"\|"MERGED"\|"CLOSED", "url", "isDraft", "reviewDecision", "base", "head", "canMerge", "checks": [{"name", "state": "pass"\|"fail"\|"pending"}], "body", "additions", "deletions", "files": [{"path", "additions", "deletions"}]}}`, from `gh pr view work-item/<id>` run in the project's folder; `canMerge` is true only when `state` is `OPEN` and gh calls it `MERGEABLE`; `{"pr": null}` when the branch has no PR | 404, 502 gh is not on `PATH` or failed, with its text |
| `POST /work-items/{id}/pr/merge` | no body needed | 200 the same body as `GET /work-items/{id}/pr`, after `gh pr merge <number> --merge` (a merge commit) | 404 no work item or no PR, 409 gh refused, with its text, 502 |
| `GET /skills?project={id}` | `project` is optional; without it the list has the library and the user-level disk skills, with it also the project's | 200 one list of `{ref, name, description, source, version?, compat}`, where `compat` is `{claude, codex}`, each the reason the skill cannot be used on that backend or `""` when it can (a package that cannot be read is incompatible with both; one that uses a `CLAUDE_` variable such as `${CLAUDE_PLUGIN_ROOT}` is incompatible with codex): the library's skills (`source` `reef`, `ref` `reef:name`, with their current `version`), then those read from disk, never stored (`source` `claude` or `codex`, `ref` `claude:name` or `codex:name`; a plugin skill is `claude:plugin:skill`). Equal names from different sources are separate entries, told apart by `ref` and `source`. claude: `~/.claude/skills`, the project's `.claude/skills` and the skills of the plugins in `~/.claude/plugins/installed_plugins.json` installed for the user or for the project path. codex: `~/.agents/skills` and the project's `.agents/skills`. A disk name found twice is listed once; each source is ordered by name. A skill whose name is not letters, digits, `:`, `_` or `-`, or whose file cannot be read, is left out, and a malformed `installed_plugins.json` gives no plugin skills; each is logged and the rest are listed | 400 `project` not a number, 404 |
| `GET /skills/library` | | 200 every library skill by name, each `{id, name, description, sourceKind: "folder"\|"git"\|"reef", source, ref, dir, revision, version, createdAt, updatedAt}`; `source` is the folder path or the repo link, `ref` and `dir` are the Git branch or tag and sub-folder (empty when not given) and `revision` the commit sha of a Git import | |
| `POST /skills/import` | `path` (a skill folder), or `repo` with optional `ref` (branch or tag) and `dir` (the package's folder inside the repo) | 201 the library skill at version 1. A folder is read, never run: every file is stored with its path relative to the folder, `.git` is skipped, and a symlink fails the import. A repo is cloned (`git clone --depth 1`) into a temporary folder that is removed afterwards. The package needs a `SKILL.md` whose frontmatter has a `name` of letters, digits, `:`, `_` or `-`. The skill and its files are saved together or not at all | 400 neither or both of `path` and `repo`, a missing folder, no `SKILL.md`, a bad name, a symlink, a `dir` outside the repo, or a clone that failed (its message says why; nothing is saved); 409 the name is in the library |
| `POST /skills` | `name`, `files` (`[{path, body}]`) | 201 a new library skill (`sourceKind` `reef`) at version 1. `files` needs a `SKILL.md` whose frontmatter `name` is `name`; each path is relative with `/` separators, not repeated and not under `.git`. Nothing is run and no Bot gets the skill: a Bot picks it as `reef:name` like any other | 400 a bad name or path, no `SKILL.md`, a frontmatter name that differs from `name`; 409 the name is in the library |
| `GET /skills/{id}` | | 200 the library skill with `files`: `[{path, body}]` of its current version, ordered by path | 404 |
| `PUT /skills/{id}` | `files` | 200 the skill after saving `files` as its next version. Only a skill written in Reef; the name cannot change. Older versions stay, and a Bot session that already copied the skill keeps its copy | 400 an imported skill, a rule of `POST /skills` broken, or a `SKILL.md` that names another skill; 404 |
| `GET /skill-drafts` | optional `space` | 200 the pending drafts, newest first, each `{id, spaceId, botId, name, files, status, skillId, createdAt}` with every file's `body` | 400 `space` not a number |
| `POST /skill-drafts/{id}/save` | optional `update` (boolean) | 200 the library skill: new at version 1, or, when `update` is true and the name is a skill written in Reef, its next version. The draft becomes `saved` | 404; 409 the draft is not pending, the name is in the library and `update` is not true, or the name is an imported skill |
| `POST /skill-drafts/{id}/reject` | no body needed | 204; the draft becomes `rejected` and nothing else changes | 404; 409 the draft is not pending |
| `POST /skills/{id}/update` | no body needed | 200 the skill after re-reading its recorded source (the same folder, or the same repo, `ref` and `dir`) as the next version. Older versions stay in `skill_file`; nothing re-reads a source unless this is called | 400 the source can no longer be read, or its `SKILL.md` names another skill, or the skill was written in Reef (edit it with `PUT`); 404 |
| `GET /providers` | | 200 the detected `claude` and `codex` (those found on `PATH`), then the added ones oldest first, each `{name, kind, detected, folder, path, version, createdAt}`. `detected` is `true` for the ones on `PATH`, which have no `folder` or `createdAt`. `path` is the binary that would run (left out when an added install's binary is gone). `version` is the trimmed output of `path --version`, run for all of them at once with a 3 s limit, and left out when it fails or is late. Models and login state are not listed | |
| `POST /providers` | `name`, `kind`, `folder` | 201 the added provider, in the shape `GET` gives | 400 bad name, `kind` not `claude` or `codex`, `folder` not an absolute path to a folder, or no executable of that kind in it; 409 `name` is taken, or is `claude` or `codex` |
| `DELETE /providers/{name}` | | 204 | 400 `claude` or `codex` (the ones on `PATH` cannot be removed), 404, 409 a bot still names it (the error names the bot; change that bot first) |
| `GET /connections` | | 200 every connection by name, each `{id, name, command, args, env, url, createdAt}` with secrets masked (see [connection](#connection)) | |
| `POST /connections` | `name`, and `command` with optional `args` and `env`, or `url` (`http` or `https`) | 201 the connection | 400 neither or both of `command` and `url`, a bad name or url; 409 name taken |
| `PUT /connections/{id}` | every field | 200 | 400, 404, 409 name taken |
| `DELETE /connections/{id}` | | 204; it leaves every Bot's selection, and the sessions that already have it keep it | 404 |
| `GET /teams` | | 200 all teams by name, each with its `id` and its roles, read from their Bots; a role has `connections`, the ids the Bot selected | |
| `POST /teams` | `name`, `responsibility`, `roles`: each member is `{"botId", "lead"}` and nothing else; a Bot is never created here. `?from=<source>` only requires that `source` exists | 201 the team as `GET /teams/{name}` shows it | 400 a member with no `botId`, an unknown Bot, a field other than `botId` and `lead`, or the usual `invalid: ...` team rules; 404 `?from=` names no team; 409 the name exists |
| `GET /teams/{name}` | | 200; each role shows `write` and `connections` | 404 |
| `PUT /teams/{name}` | the team JSON `reef team apply` takes (`name`, when given, must match the path; `roles` are `{"botId", "lead"}`); replaces the team's members with the same validation. `?from=<source>` instead requires `source` to exist and `name` not to, checked and inserted in one statement so two concurrent copies to the same name cannot both win | 200 the applied team, as `GET /teams/{name}` | 400 the same `invalid: ...` text `reef team apply` gives, or a body name that does not match the path; 404 `?from=` names a team that does not exist; 409 `?from=` names a team that already exists |
| `DELETE /teams/{name}` | | 204; the Crew's Spaces stay, with no team | 400 `solo`, 404 |
| `GET /bots` | | 200 every bot by name, each `{id, name, description, skills, backend, model, provider, face, colour, tools, connections, write, permissionMode}` | |
| `POST /bots` | the bot fields above | 201 the bot | 400 the same `invalid: ...` text a role gets, 409 name taken |
| `GET /bots/{id}` | | 200 | 404 |
| `PUT /bots/{id}` | every field | 200 | 400, 404, 409 name taken |
| `DELETE /bots/{id}` | | 204 | 400 the bot is in a team or a Space (names it), 404 |
| `GET /bots/{id}/conversations` | `?project=ID` (only those associated with the Project), `?limit=N` (at most 200; without it all), `?before=ID` (only those after that conversation in the list, so the last id of a page is the next `before`), `?resumable=1` (leave out the conversations that moved to a Space) | 200 the Bot's direct conversations as Space previews, the one with the newest message first (a conversation with no message sorts by its creation). Pages follow the list order by (newest activity, id), so they never repeat or skip one unless a listed conversation gets a message while the pages are read. `?resumable=1&project=N&limit=1` is the Bot's latest chat to resume under Project N and `?resumable=1&limit=1` its latest overall; `[]` means none, and the server never creates one: the client offers New chat (`POST /spaces`). Start one with `POST /spaces` and `botId`; continue any by sending to it: its Bot resumes its session. A conversation runs with the Bot's profile, never a Team's | 400 bad id, 404 |
| `GET /bots/{id}/chats` | `?project=ID`, `?limit=N` (at most 200), `?before=SPACE_ID`, paged like `/bots/{id}/conversations` | 200 the Bot's chats, newest activity first, as `{kind, spaceId, botId, label, origin?, active, closed, lastActivity}`. `kind` is `chat` (a personal chat; label is its title), `space` (a Space the Bot is a member of, or a work item or free Space it left after taking part, not one it only sent exchanges into; label `<Bot> · <Space>`, such as `Alex · Login Space`, with ` (left)` after it once the Bot left) or `consultation` (label `Consultation from <asker> · <first words of the first question> #<Space id>` for the consulted Bot, `Consultation with <Bot> · <first words> #<Space id>` for the asker, whose entry opens the consulted Bot's activity). A consultation's `origin` is the requesting chat `{spaceId, label}`, derived from its first request's `origin_space_id` and `from_bot_id`. `active` is true while a turn of the Bot (any turn in a consultation) runs; `botId` is the Bot whose activity the entry opens. Reads stored rows only | 400, 404 |
| `GET /spaces/{id}/bots/{botId}/activity` | | 200 `{space, botId, label, origin?, messages, exchanges, turns}`: what the records hold of that Bot in that Space, a Bot that left included (label ends ` (left)`). `messages` are those it wrote, those that woke it and those linked to its exchanges (every message in a chat or consultation); `exchanges` are its direct messages with their state; `turns` are its turns each with `toolCalls` (`{id, name, input, result, parentId?}`, as the backend exposed them). Nothing about model reasoning. A pure read: no turn, session or workspace is created, so it works on a closed Space whose workspace and backend are gone | 400, 404 the Space, or the Bot never took part in it |
| `POST /teams/{name}/members` | `botId`, `lead` | 201 the role as `GET /teams/{name}` shows it | 400 unknown bot, a second lead, or `solo`; 404 team; 409 already a member |
| `PUT /teams/{name}/members/{id}` | `lead` | 200 the role | 400 as `PUT /teams`, a second lead; 404 |
| `DELETE /teams/{name}/members/{id}` | | 204 | 400 `solo`; 404 |
| `GET /spaces` | `?project=ID` (the Spaces associated with the project, see [Space projects](#space-projects)), `?workItem=ID` | 200 each Space also has `lastMessage`: `{author, body, createdAt}` of its newest message (a thread reply counts; `body` is cut to its first 200 characters), `null` when it has none | 400 bad id |
| `POST /spaces` | `projectId` (optional for a Space with no work item, which then starts with no project; required with `workItemId`), `workItemId`, `name`, `team` (a team name; default `solo`), `role` (empty or `lead`; the Space's `role` becomes the name of the team's lead role), `botIds` and `coordinatorBotId` (the Space's members are exactly these Bots, de-duplicated, led by the coordinator or the first Bot; `team`, when given, only names the Crew it started from), `botId` (instead of `team`: a Space with no Team and one member, that Bot as lead; `kind` `conversation` makes it a direct conversation, which cannot have a work item). A Space takes no `backend` or `permissionMode`: its Bots decide. With `workItemId` the Space is made of the item's owner (coordinator) and participants, whatever `team`, `botIds` or `coordinatorBotId` say (a sub-item has no Space of its own, and an item not approved yet has none: 400 "approve it first"). A `name` is required; one that is the new-space form's default, `<team or bot name> <hh:mm>` (`solo 14:32`, `solo 02:32 PM`), marks the Space as waiting for a provider name, see [Space names](#space-names) | 201 | 400 unknown project, work item, team or Bot, work item of another project, a field the table does not list, a role other than `lead` |
| `GET /spaces/{id}` | | 200 | 404 |
| `PUT /spaces/{id}` | `name` (required); work item and role stay unchanged, and so do the projects (see the project routes below). A name that differs from the stored one ends any wait for a provider name | 200 | 400 bad JSON or any other field, 404 |
| `DELETE /spaces/{id}` | stops the Space first if it is live; a send during the delete is 404 | 204 | 404, 409 the Space is the proof's Space of a work item in review |
| `POST /spaces/{id}/send` | `text`, `parent` (optional: the id of a top-level message of this Space; the message is stored as a reply in its thread) | 202, no body; while a message chain starts or runs the message is stored and its wakes [queue](#routing) behind the current turn | 400 empty text, an unknown field (`role` is not a field), a `parent` that is not a top-level message of this Space, or `/wrap` on a quick session, 404 (also while the Space is being deleted), 409 wrap or promote turn running, Space closed, Space paused (a hop cap pause does not count), open elsewhere or stopped before its turn began, 503 |
| `GET /spaces/{id}/events` | | 200 `text/event-stream` | 404 |
| `GET /spaces/{id}/inspector` | | 200 `{roles, turns}`. `roles` has one entry per role (one per Bot id, or per name for a turn with no Bot; `role` is the name on its newest turn): `role`, `botId`, `turns`, `usage` (summed), `costUsd` (summed, `null` when no turn of the role has a cost); `turns` has one entry per turn of the Space: `id`, `role`, `botId`, `startedAt`, `status`, `model`, `usage`, `costUsd`, `prompt`, `reply`, `toolCalls` (`name`, `input`, `result`, `targets`, and `children` when it is an `Agent` call: its subagent's calls, same shape, not repeated at top level). `targets` is a list of `{"projectId", "projectName", "workspaceId"}`, present when the call's `file_path`, `path`, `notebook_path`, `cwd` or `command` points into at least one workspace of the Space (its work item's, or the quick fixes of a Space with none), by its absolute path or its folder relative to the agent's working folder; a call that points into none has none. The live `agent` stream carries the same `targets` on `tool_call` events | 404 |
| `GET /spaces/{id}/direct-messages` | `?after=ID` (only newer ids) | 200 all direct requests and response attempts in the Space, oldest first; each has `id`, `spaceId`, `chainId`, `replyToId`, `sourceMessageId?`, `from`, `to`, `body`, `status` (`sent`, `done` or `failed`), `state` (a request only: `waiting`, `delivered`, `answered`, `failed`, `stopped` or `cancelled`), `kind`, `originSpaceId?`, `originMessageId?`, `createdAt` | 400 bad id, 404 |
| `GET /exchanges/{id}` | | 200 `{request, response}`: the exchange request (a direct request with `originSpaceId`; `spaceId` is its destination) and its newest response, `null` until there is one | 404 |
| `POST /exchanges/{id}/retry` | no body needed | 200 as `GET`; moves a `failed` or `stopped` exchange back to `waiting` and delivers it again | 404, 409 not `failed` or `stopped` (two retries at once: one wins), or has a `done` response |
| `POST /exchanges/{id}/cancel` | no body needed | 200 as `GET`; a `waiting` exchange becomes `cancelled` | 404, 409 not `waiting` |
| `GET /search` | `?workItem=ID&q=TEXT` (both required), `limit=N` (default 20) | 200 hits across the work item's public messages and all its direct messages, newest first, `[]` when none. Each hit has `spaceId`, `messageId`, `kind` (`public` or `direct`), `author` and `snippet` (about 120 characters around the match, case ignored for ASCII); a direct hit also has `from`, `to` and `chainId`, and its `messageId` is the direct message id | 400 missing or bad `work item`, missing `q` |
| `POST /spaces/{id}/reopen` | no body needed | 204; clears `closedAt` | 404 |
| `POST /spaces/{id}/stop` | no body needed | 204, also when the Space is not live or does not exist | |
| `POST /spaces/{id}/continue` | no body needed | 204, no body | 404, 409 not paused or a turn running |
| `POST /spaces/{id}/bots/{botId}/refresh` | optional `model` and `backend`, written to the Bot first | 200 `{"pending":false}`: the Bot's active adopted settings are replaced by its current ones and its agent is closed, so its next turn starts a new backend context with a handover (see [memory](memory.md)). 202 `{"pending":true}` when a turn is running and the Bot has an active session (`{"pending":false}` when it has none: its next turn adopts): the refresh is kept on the Bot's active session (`refresh_pending`, so a restart keeps it) and applied before the next turn of the Space; a refresh that fails stops waiting, posts one note in the Space and fails no turn; refreshing again retries it | 400 unknown backend; 404 not a member |
| `POST /bots/{id}/revoke` | no body needed | 200 `{"sessions":n,"tokens":n,"toolCalls":n}`: in one transaction every active `bot_session` of the Bot, in every Space, becomes `revoked`, every token of the Bot is deleted and every tool call of a running turn of those sessions that has no result gets the result "stopped: the Bot was revoked; the call may or may not have finished". A Space with a live session interrupts the running turn only when it is the Bot's own, and closes the Bot's agent (at once when idle, else when the running chain of turns ends) and cancels its running memory review; other Bots' turns go on. Each Space that had a session gets one message from reef saying the Bot was revoked and that tool effects are not undone. History is kept; the Bot's next turn adopts its current settings and starts a new backend context with a handover (see [memory](memory.md)) | 404 unknown Bot |
| `POST /spaces/{id}/wrap` | no body needed | 202, no body | 400 a quick session (no work item); 404, 409 and 503 as send |
| `POST /spaces/{id}/attach` | `workItemId` (required) | 200 the Space | 400 no `workItemId`, Space already has a work item or is a conversation, work item of another project, project not a git repo with a commit; 404; 409 a turn running, or the work item already has an active Space |
| `GET /spaces/{id}/projects` | | 200 the projects the Space is associated with, by id, `[]` when none | 404 |
| `PUT /spaces/{id}/projects/{projectId}` | no body needed | 200 the Space's projects after it; doing it twice changes nothing | 400 a Space of a work item, which stays in its work item's project; 404 Space or project |
| `DELETE /spaces/{id}/projects/{projectId}` | | 200 the Space's projects after it; the Space, its history and its sessions stay, and so does a project it was not associated with | 400 a Space of a work item; 404 Space |
| `GET /spaces/{id}/workspaces` | | 200 the quick fixes the conversation holds (made by `open_workspace`), abandoned ones included, oldest first: the workspace fields (`id`, `projectId`, `name` (the fix), `dir`, `branch`, `base`, `baseRef`, `baseSource`, `state`, `note`) plus `projectName`, `head` (short commit id of a ready tree, else `""`) and `dirty` (uncommitted changes). `[]` when none. They survive a restart: `head`, `branch`, `base` and `dir` are what delivery of the fix reads | 404 |
| `POST /spaces/{id}/workspaces/{workspaceId}/abandon` | | 204 the fix is given up: its tree is removed, its branch stays in the repository, state `removed`; doing it again changes nothing. A checkpoint turn of the Space can no longer be rolled back into it | 404 Space or workspace; 409 uncommitted changes in the tree (commit or discard them first), or a turn of the Space is running |
| `GET /spaces/{id}/members` | | 200 the Space's members, lead first: the role fields of a [team](#team) role, plus `session` for a Bot with an active session: `{backend, model, permissionMode, write, tools, skills: [{ref, compatible, reason}], connections (names only), refreshPending, stale}`, what the session runs with. `stale` is true when the Bot's own settings differ from it, a selected connection has changed content, or a picked `reef:` skill has a newer library version (a skill read from disk is not compared) | 404 |
| `POST /spaces/{id}/members` | `botId`, or `crew` (a team name) | 201 the member, and a `<Bot> joined the Space` notice from Reef. A user addition needs no approval; only a Bot's `propose_member` waits for the user. The newcomer's first prompt carries a handover of where the Space stands (goal, brief, sub-items, decisions, last messages) that points to `read_thread`, `search_sessions` and `recall`; the members read the notice and the new roster with their next wake. `crew` adds each of its Bots that is not yet in the Space, never changes the coordinator, and is 200 with the Space's members. On a conversation: 202, the same as the user writing `@name` at the start of a line (see [Spaces live](#spaces-live)) | 400 unknown Bot, or the conversation's own; 404; 409 already a member or its name is taken, or a turn is running |
| `DELETE /spaces/{id}/members/{botId}` | optional `coordinator` (the remaining member that leads in its place; required when the Bot leads) and `replacement` (a Bot added first, for the last Bot to leave or as the successor) | 200 `{"pending":false}` once the Bot is out, or 202 `{"pending":true}` while a turn runs: the Bot leaves at the end of that turn, which finishes. Leaving deletes the member row, ends the Bot's session, drops the wakes held or queued for it, gives each request waiting for it the `cancelled` state and a failed response, and posts `<Bot> left the Space`. Nothing wakes the Bot afterwards. A pending removal is stored on the member row (`leaving`, shown as `leaving: true` in the members and the Space's roster): the Bot gets no new wake, and the removal is applied when any chain ends, including a claim that runs no turn, and at server start. Holds and queued wakes of a removed Bot are dropped, so Continue never starts a non-member | 400 a conversation; 404 not a member; 409 the Bot leads and `coordinator` is not a remaining member, or it is the last Bot and no `replacement` is given |
| `GET /spaces/{id}/member-proposals` | | 200 the pending proposals of a Bot to add a Bot: `id`, `spaceId`, `botId`, `botName`, `proposedBy`, `reason`, `status`, `createdAt` | 404 |
| `POST /member-proposals/{id}/approve` | no body | 200 the proposal; adds the Bot as `POST /spaces/{id}/members` does | 404; 409 not pending, or already a member |
| `POST /member-proposals/{id}/reject` | no body | 200 the proposal | 404; 409 not pending |
| `PUT /spaces/{id}/coordinator` | `botId` | 200 the Space's members, that Bot the only coordinator. Changes where messages that mention nobody go, and nothing else: no Work item, owner or other member changes. Closes the Space's idle agent so the next turn starts with the new coordinator | 400 not a member; 404; 409 a turn is running |
| `GET /spaces/{id}/proposal` | | 200 the Space's pending proposal | 404 none pending |
| `POST /proposals/{id}/approve` | no body needed | 200 the proposal, `itemsAdded` and `warnings` (a line for each saved item that looks like a stored one or has no vector; left out when none); a wrap closes the proposal's Space and stops its agent, a brief proposal does neither | 400 already decided, 404 |
| `POST /proposals/{id}/reject` | no body needed | 200 the proposal | 400 already decided, 404 |
| `GET /pins` | | 200 every pin, newest first, `[{kind, id, title, createdAt}]`; `kind` is `bot`, `chat` (a personal conversation) or `space` (any other Space), `title` the target's current name. Pins are global: no Project filters them | |
| `PUT /pins/{kind}/{id}` | | 204; pinning again keeps the first pin time | 400 `kind` not `bot`, `chat` or `space`, 404 no such target of that kind (a chat is not pinnable as a space) |
| `DELETE /pins/{kind}/{id}` | | 204, also when it was not pinned. Deleting a Bot or Space removes its pins | |
| `GET /find` | `?q=TEXT` (required), `limit=N` (per kind, default 20, at most 200) | 200 `[{kind, id, title, projectId?}]` of the Bots, chats, Spaces and Work items (`workItem`, with its `projectId`) whose name or title contains `q`, case ignored for ASCII, in that order and each by name, `[]` when none. `kind` and `id` open the original resource | 400 no `q` |
| `GET /messages` | `?space=ID` (required), `?after=ID` (only newer ids). `?limit=N` (at most 200, default 50) and `?before=ID` give a page instead: the newest `limit` messages with an id below `before` (all ids when it is left out), oldest first; the first id of a page is the next `before`, and an empty page is the start of the history | 200, each with `parentId` | 400 no Space, 404 unknown Space |
| `POST /messages` | `spaceId`, `parentId`, `author`, `body` | 201 | 400 unknown Space, missing field, or a `parentId` that is not a top-level message of that Space |
| `GET /turns` | `?space=ID`, `?status=running\|done\|failed`, `?retryable=1` (only the turns `retry` accepts), `?rollbackable=1` (needs `space`; only the turns with `checkpoints`, all taken in workspaces of the work item the Space belongs to now, so `rollback` is not a 409 for the tree) | 200 | 400 bad id or status, or `rollbackable` without a Space, 404 `rollbackable` on an unknown Space |
| `POST /turns` | `spaceId`, `role`, `backend`, `sessionId`, `wakeText`, `wakeParent`, `directMessageId` | 201 | 400 bad backend or unknown Space, public message or direct message |
| `GET /turns/{id}` | | 200 | 404 |
| `PATCH /turns/{id}` | `status` (`done` or `failed`), `error`, `usage`, `sessionId`. With no `status`, only `sessionId` is set and the turn stays `running` | 200 | 400 bad status, negative usage, empty `sessionId` without a status, `error` or `usage` without a status, or already ended; 404 |
| `POST /turns/{id}/retry` | no body needed | 202, no body; the wake of the failed turn runs again as a new turn of its role, and the failed turn is `retried` | 404, 409 not retryable (see [Spaces live](#spaces-live)), or as send for a Space with a turn running or paused, 503 |
| `GET /inbox` | | 200 the items waiting on the user, newest first; `[]` when none. See [Inbox](#inbox) | |
| `GET /inbox/events` | | 200 `text/event-stream` | |
| `POST /inbox/dismiss` | an item as `GET /inbox` lists it (`kind` and `id`, or for `reply` `spaceId` and `messageId`) | 204, no body; an FYI item is gone for good, see [Inbox](#inbox) | 400 bad body or an item that names nothing, 404 no such FYI item or a message that is not the Space's, 409 the item is in Needs you |
| `POST /turns/{id}/rollback` | no body needed | 204, no body; every workspace the turn checkpointed is as it was before the turn, and `reef` posts `Rolled back to before turn N.` | 400 the turn has no `checkpoints`, 404, 409 the Space has a turn running, a `reef chat` has it open, a checkpoint was taken in a workspace its work item does not have, or a turn of any role runs on the work item's root, 503 |
| `GET /decisions` | `?workItem=ID` (required) | 200, retired decisions too, with `retiredAt` and `replacedBy`. Unused since decision 91 | 400 no work item, 404 unknown work item |
| `POST /decisions` | `workItemId`, `messageId`, `body`, `why`, `author`, `replaces` | 201. Unused since decision 91 | 400 missing field, unknown work item, a message of another work item, or a `replaces` id that is not a live decision of the work item |
| `GET /decisions/{id}` | | 200. Unused since decision 91 | 404 |
| `GET /memory` | `?scope=KIND:ID` (required, such as `work_item:3` or `project:1`) | 200 the accepted items of that scope, the most recently accepted first, as objects (`id`, `kind`, `body`, `why`, `scope`, `author`, `role`, `botId`, `status`, `retiredAt`, `replacedBy`, `createdAt`; no `links`). `botId` is the id of the Bot that wrote the item, `null` for the user; `author` stays the name recorded then; `[]` when none | 400 no scope or one that is not `kind:id` |
| `POST /memory` | `kind`, `body`, `why`, `scope` (`KIND:ID`) | 201 the item as for `GET /memory`, written with author and role `user`, so always accepted, plus `similar` (`[{id, body}]`, stored items that look like it) and `embedError` (why no vector was stored), each left out when empty | 400 unknown kind or scope kind, empty body, bad scope |
| `GET /memory/pending` | | 200 the candidates waiting for the user, oldest first, as for `GET /memory` plus `links` (`{rel, to}`: `source`, `replaces`) | |
| `POST /memory/{id}/approve` | no body needed | 200 the item, `accepted`; the items it replaces are retired with `replacedBy` set, in one transaction. Approving an accepted item changes nothing | 400 not a candidate, 404 |
| `POST /memory/{id}/reject` | no body needed | 200 the item, `rejected`; what it would have replaced stays accepted | 400 not a candidate, 404 |
| `POST /memory/{id}/retire` | no body needed | 204, no body; sets `retiredAt` | 400 not an accepted item (already retired, a candidate), 404 |

A `POST` with no body (`stop`, `continue`, `wrap`, `approve`, `reject`) still needs
`Content-Type: application/json`, or the guard answers 415. These routes ignore the body,
so an empty one or `{}` both work. `send` and `attach` read a JSON body, and an empty one
is 400.

Create requests take the whole object from the table above. A field you leave out takes
its default. Other `PUT` requests replace the whole object. Update behavior is
route-specific: `PATCH /turns/{id}` is the only partial update.

## Spaces live

`POST /spaces/{id}/send` runs the Space's agents through
[`space.Open`](../space/session.go), the code `reef chat` uses. A Space has a
[team](#team): the lead answers the user, and the other roles are specialists that run when
a message wakes them.

- The server stores the user message, then answers 202. The turns run in the background, not
  in the request. The text `/wrap` runs the wrap-up turn (400 on a Space with no work item, before any
  agent turn). Every other text is a plain message. A send with `parent` stores the message as a reply in that
  thread.
- In a direct conversation, `@name` of another Bot at the start of a line (the first one that names a
  Bot) moves it: the lead writes a summary for that Bot in a turn of the conversation, then the
  conversation gets `movedTo` and a new free Space starts with the lead and that Bot, no Team. The summary is the new Space's first message, from the lead, starting `@name`, and
  wakes the Bot; the conversation's own messages never reach it. An `@name` inside a line moves nothing.
- Each role of the team has its own agent session, its own `reef mcp` token, its own turns and
  its own session id. The role's backend, permission mode, tools and MCP servers are set up
  for it from the settings its Bot adopted in the Space (see [role](#role)). Its first prompt starts with `## You`,
  then its description and skills (see [memory.md](memory.md#the-context-prompt)).
- An agent starts when its first turn runs, not when the Space opens. A first message that
  wakes a specialist starts no lead. A role that already has a session resumes it by its own
  newest session id (see [memory.md](memory.md#reef-chat)). An agent that fails to start, for any
  reason (the agent, its session lookup, its token, its folder or its first prompt), shows
  as one `failed` turn of its role, with the start error, or `interrupted` when a stop cut the
  start short. So does a wake whose text cannot be built. A wake for a role the team no longer has is a `failed`
  turn too, with the wake text, on the lead's backend (`claude` when the Space has no member). The wake text of a wake that never started is the
  text the wake was routed with, not the messages it would have sent. The server logs every failed turn. When the backend says the session is
  missing (claude `No conversation found`, codex `no rollout found for thread id`), that turn
  fails and the role's next wake starts a new session, with its intro, not the dead one.
- <a id="workspaces"></a>Workspaces. Each target of a top-level work item gets one workspace, a
  git worktree of that Project's repository on branch `work-item/<work item id>`, in
  `<data folder>/worktrees/item-<work item id>/<project id>-<project name as a slug>`, one folder
  per item with one tree per target. `POST /work-items/{id}/workspaces` records each row as
  `creating` first, then fetches the Project's base branch from `origin` and adds the tree from
  `origin/<base>` under the `add <dir>` lock: `ready`, `baseSource` `origin`. When the fetch
  fails and the local `<base>` exists, the tree starts from it: `ready`, `baseSource` `local`, and
  `note` `origin could not be refreshed: <error>; using local <base> at <sha>`. When neither
  exists, or the folder is no git repo with a commit, the row is `paused` with the reason in
  `note` and only that repository waits; the others are made. A new note is also posted as a
  `reef` message in the item's active Space. A Project with no base branch uses the branch
  `origin/HEAD` names, else the local default branch (when the repository has an `origin`, `note`
  says `origin's default branch is unknown; using local <branch> at <sha>`), never an assumed
  `main`. Only authorized targets get a workspace: one still proposed by a Bot gets none. A
  `creating` row found again (a restart cut the creation) is finished in the same folder, keeps
  the edits already there, and keeps its base and note. No workspace is rolled back because another failed.
- An agent of a Space under a work item runs in the work item's root folder,
  `<data folder>/worktrees/item-<work item id>`, which holds one workspace per target, so one
  claude or codex process reaches every repository by a relative path. Each workspace is on
  branch `work-item/<work item id>`. A sub-item has none of its own: its workspaces, branch and PR
  are its parent's, its Spaces run in the parent's root, so the diff and PR routes of a sub-item
  answer for the parent, and deleting a sub-item keeps the trees. The workspaces are made (see
  Workspaces above) before the first start of any agent of the work item and again at each start
  that finds one missing; every Space of the work item shares them. The first prompt lists each
  target: Project name, folder, branch, base and where the base came from, or why it is paused.
  A start with no ready workspace fails as a `failed` turn whose error is the reason, such as
  `work items need the project folder to be a git repo with a commit: <path>:` and git's error; a
  paused target beside a ready one does not stop the agent. A branch that is checked out elsewhere
  (in the main checkout, or in a worktree of another project on the same repo) cannot be added:
  the start fails with `branch work-item/<id> is checked out at <path>; switch that checkout to
  another branch`. A Space with no work item runs in `<data folder>/chats/<Space id>`, a folder of
  its own that is made when missing and is no git repo; `DELETE /spaces/{id}` removes it. It can
  never be given the root or a workspace of a work item: a workspace folder held by a work item is
  409 for any other owner. A project the Space is associated with is context only, never its
  folder: repository work needs a work item (attach the Space to one). The work item is read at
  each agent start, so an attached Space moves to the work item's root at its next wake. A
  workspace folder that was deleted is pruned and made again at the next wake: an agent running in
  a missing root is closed, and the next one resumes the session in the new root.
- Before each turn of a role with `write` on a work item, the server stores a checkpoint of every
  ready workspace of the item: a commit of HEAD plus the whole working tree, untracked files
  included and ignored ones left out, at `refs/reef/checkpoints/<data folder>/<turn id>/<workspace id>` in that
  workspace's repository, where `<data folder>` is the first 12 hex digits of the sha256 of the
  absolute data folder path, so two Reef copies on one repo never overwrite each other's. One
  `turn_checkpoint` row per workspace holds the ref; they are the turn's `checkpoints`. A Space
  with no work item gets one for each ready quick fix it holds (`open_workspace`), none without. It is built in a temporary index that starts as a copy of
  the real one, so the index, HEAD and the files do not change. If a checkpoint fails, the turn
  is stored `failed` with git's error and the agent gets no prompt. A repo with no commit has
  nothing to snapshot: it gets no checkpoint. A role without `write` gets none, unless it is a
  writer in all but name (below). The checkpoint commit has two parents:
  HEAD, then a commit of the index as it was, so a rollback can bring back what was staged.
  Its message records the directory and the branch it was taken in (`dir:` and `branch:`
  lines; no branch when HEAD was detached).
- One write turn at a time runs on a tree, across all Spaces and processes on the data
  folder: the work item's root folder (so every Space of an item and of its sub-items takes
  turns, while different work items and chats run at once), or the Space's own folder. Nothing
  else limits how many turns run: no global and no per-Bot maximum, only this lock and the
  caps. A wake of a role with `write` waits for
  the write turn that runs on its tree, then runs. Its turn row is created when it starts, so
  `startedAt` is after the other turn's `endedAt`. A stop ends the wait, and the turn is
  stored `failed` as `interrupted`. A claude role without `write` whose tools allow a shell
  command outside a short list of reading commands (`git status`, `ls`, `cat`, `head`,
  `tail`, `grep`, `wc`, `pwd`) counts
  as a writer: it takes the lock and gets a checkpoint, as `Bash(git commit:*)` or `Bash(rm:*)`
  could change files. `rg` is not a reading command: `rg --pre` runs a program. Neither are
  `git log`, `git diff`, `git show` and `git blame`: `--output=<file>` makes them write a file. Only the
  tools the role keeps count: a plain `Bash` or `Bash(*)` is dropped from a role without `write`,
  so it does not make the role a writer. A role without `write` and with only reading commands is not held: it runs beside a
  write turn, and counts as a reader of the tree for its turn, which only a rollback
  respects. Making a workspace takes a lock of its own, so two Spaces waking at once add it once.
  The locks are `flock` files in `<data folder>/locks/`, named from a hash of the tree path
  (`.lock` for the writer, `.read` for readers), so `reef serve` and `reef chat` share them.
  A waiting write turn polls its lock every 20 ms. A rollback takes the writer lock without
  waiting and checks the reader file, so a read-only turn of any process refuses it.
- `POST /turns/{id}/rollback` puts every workspace the turn checkpointed back, each in its own
  repository and with no change to the others, as the turn's checkpoint saw it: the branch the checkpoint was
  taken on is checked out at the HEAD it had, so commits made since, and a branch the turn
  switched to, are left behind (a detached checkpoint resets whatever is checked out); tracked
  and untracked files get the content they had, the same changes are staged, and files made
  since are removed. Ignored files are left alone. It claims the Space like a send. It is
  409, never a wait, when a checkpoint was taken in a workspace the Space's work item does not
  have (a free Space's turn after an attach), when a `reef chat` holds the Space, or when a
  turn of any role of any Space runs on the work item's root (the write lock is held, or a
  read-only turn is a reader). There is no atomic rollback across repositories: if one restore
  fails, the others are still tried and `reef` posts `Rollback to before turn N stopped partway. Restored: ... Not restored: ...` when some were restored, before the error is returned. A reader that starts during the restore is not held
  back. A turn with no `checkpoints` is 400. Then `reef` posts `Rolled back
  to before turn N.` in the Space. The agent's session is not rewound. When a turn of
  a role that has run before in the Space is sent, its prompt gets the lines `reef wrote: ...`
  that `reef` posted since that role's last turn began, after the header line, so the role
  learns of the rollback (and of a restart failure note) even when its wake is a retry; a role
  that never ran gets none. The wake kept as `wakeText` does not have them.
- `DELETE /work-items/{id}` cleans what Reef made for the work item. It stops the agents of the
  work item's Spaces first (a running turn is stopped, as when a Space is deleted). A
  workspace with uncommitted changes (untracked files count) is 409 with `work item <id> has
  uncommitted changes in <path>; commit or discard them first`, and nothing is removed.
  Otherwise every workspace is removed with `git worktree remove`, then the root folder, the work
  item rows are deleted, and last the `refs/reef/checkpoints/<data folder>/<turn id>/<workspace id>` refs of
  every turn of its Spaces are deleted, in the repository of the workspace each was taken in.
  The workspaces go first because that is the step that can refuse or fail, and a
  later wake of a work item makes them again; the refs go after the rows that name them, so a
  failure there leaves refs that no turn points to, and the delete is 500. The branch
  `work-item/<id>` is kept in every repository: it is the user's work. A work item with no
  workspace folder (no agent ever started) only has its rows deleted. Deleting a Space keeps
  workspaces and refs.
  Right after the rows go, the memory items of the work item's `work_item:<id>` scope are
  closed: pending candidates are rejected and accepted items retired, so nothing of it is left
  in `reef memory pending`. Retired and rejected items are kept and still read.
- `DELETE /projects/{id}` does the same for every work item of the project and every workspace
  of the project in any work item or quick fix, and deletes the checkpoint refs taken in its
  repository and those of the turns of every Space of the project. A Space with no work item is
  not deleted: it only loses the association, and keeps its other projects. It checks every
  workspace first: one with uncommitted
  changes is 409, and nothing is removed. Then it removes all workspaces,
  deletes the project rows, and last deletes the refs, in the same order and with the same
  failure rule as a work item delete. Branches are kept. Its memory items are closed the same
  way, in the project's `project:<id>` scope and in the `work_item:<id>` scope of each of its
  work items.
- The lead stays alive between turns, until `POST /spaces/{id}/stop` or the server stops. A
  specialist's agent is stopped, and its token deleted, when its turn ends. Its next wake
  resumes its session. A background task a specialist started ends with its agent, and its
  answer is not posted. While no turn runs, the lead's events are read, so the answer of its
  background task is posted as its message when it arrives (decision 84).
- The server holds `chat-<id>.lock` while the Space has a live session, so `reef chat
  --space ID` fails with `space 1 is open elsewhere: conflict`. A send to a Space a
  `reef chat` holds is 409 too.
- `stop` interrupts a running turn (stored `failed`, `interrupted`), drops the wakes still
  waiting (so does a turn that fails: its chain ends and the wakes behind it do not run; the
  next send starts a chain that posts a message from reef naming the roles of those wakes, and
  a restart before that send holds them instead), then stops every agent of the Space, deletes their tokens and releases the lock.
  It also works while the send is still checking the caps or starting an agent: the send then
  answers 409 `space 1 was stopped`, and no turn runs. Once the send has changed something
  or stored the user message, it does not give up: it answers 202 as
  usual, and its turn runs with the stop already in effect, so it is stored `failed`,
  `interrupted` and the agent gets no prompt. A turn that fails for another reason also stops
  every agent of the Space; the next turn opens them again, each by its own session id.
- `DELETE /spaces/{id}` does the same as `stop` before it deletes the Space.
  `DELETE /work-items/{id}` and `DELETE /projects/{id}` do it for each of their Spaces.
  From the start of that stop until the delete is done, a send to the Space is 404
  `space 1 is being deleted`, so no agent starts for a Space that is going away.
- Server shutdown does the same for every Space before it stops listening.
- A turn keeps the wake it ran as `wakeText`, `wakeParent` and `directMessageId`. `POST /turns/{id}/retry` on a
  `failed` turn runs that wake again for the same role as a new turn in a new chain, the way an
  approve does, with no new user message: the new turn answers in the same public thread or
  direct exchange, and what its
  reply wakes is routed as usual. The failed turn is marked `retried` before the turn starts, so a
  second retry is 409; if the turn cannot start, the mark is taken back. Nothing retries a
  turn by itself. A turn is retryable when it is `failed`, not `retried`, has a `wakeText`, and
  no later turn of its role ran in the Space: a turn that a later one supersedes is 409. A
  turn the user stopped (`interrupted`) is retryable, since the user may want its wake run
  again. A routed wake that never sent is built again from the
  messages not yet sent, so a retry never resends a whole thread; a turn that sent runs its kept text again. A wrap-up turn keeps no wake, so it is never retryable: the user wraps
  again. `GET /turns?retryable=1` lists the same turns, which is what the window shows
  as failed turns with a Retry button. The retry counts as a message of the user for the hop cap.

### Routing

When a turn ends, the server reads the messages stored since the last one it read and routes
those written by the role of the turn that ended: its reply, which is stored as a message from
the role, and any message it posted with the `post_to_space` tool (see
[memory.md](memory.md#reef-mcp)). A turn that posted with `post_to_space` has one message: its
plain reply at the end of the turn is not stored as a message, so it wakes no one a second time.
The turn keeps it (`reply`, shown by the inspector). The exception is a turn that called another
tool after its last post: the text it wrote after that post (trimmed, when not empty) is stored as
one more message and the turn keeps the text before it (decision 158). A post counts when its tool call did not
fail. A turn that did not post, a failed or stopped turn, and a direct turn (its reply is the
direct response) store the reply as before. Other messages, such as a `POST /messages` from the user in
the middle of a chain, are skipped: they start no turn and do not reset the hop count. Each
routed message goes through the router, [`space.Route`](../space/route.go), plain code with
no model. In order:

1. A reply from the user in a thread goes to the thread's owner, the author of the root. When
   the user owns the thread, it goes to the lead. An agent's reply in a thread has no owner
   rule: it wakes only the roles it mentions.
2. Otherwise every `@name` that is a role of the team goes to that role, each once, when it
   is a handoff: it starts the body or a line (spaces and tabs may come before it), or it
   follows other `@name`s at the start with only spaces or commas between them, as in
   `@architect, @tester do X`. A name later in a sentence wakes no one: `I'll report to @lead
   later` and the second name of `@architect and @tester do X` are not handoffs. The sender
   and unknown names are skipped. The message is stored once. When it wakes more than one
   role, each wake's header is followed by `Asked together: @a, @b` naming all of them.
3. Otherwise a message from the user goes to the lead.
4. Otherwise nobody: a message from an agent that hands off to no one wakes no one, in a thread or not.

The lead is the Space's `role`: on a Space whose role is `developer`, `@lead` names no
role, so the message goes to `developer` and no `lead` session starts.

A role never wakes itself. The user's own message is routed by the same rules, so a send
with `parent` on an architect's thread goes to the architect, and `@architect ...` goes to the
architect with no lead turn. `POST /messages` stores a message and wakes nobody. The `/wrap`
turn is not routed (see below).

A wake is a turn of the woken role. Routing gives it the text it was routed with: a line
`[Space 3, message 12]` for a top-level message, or `[Space 3, message 12, thread 9]` for a
reply (the Space, the waking message and, for a reply, its thread root), then the user's
text as it is, or `<author> wrote: <body>` for an agent's message. That text is what the wake keeps if it never starts. The wake also carries its scopes, the threads
(0 for the top level) of the messages it was routed with, as data from routing, never read back
from message text, so a message that quotes a header changes nothing. The turn's own text is built
when the turn starts, not when the message is routed, from what the role has not been sent in its session:
the messages of the thread (or, for a top-level message, the Space's top level) that no
earlier turn of the role in that session sent, oldest first, and never the role's own
messages or reef's. It starts with a line naming the newest of them, in the same form. When
there is more than one, all but the newest come after `New in the thread:` (or `New in the
Space:`) as `<author>: <body>`, and the newest follows as the user's text or `<author>
wrote: <body>`. A message stored after the wake was routed, before it runs, is in the text.
A turn keeps what it sent for each scope (`turn.scopes`, the newest message of each): the role's
later turns in the same session skip every message up to it, so a thread is sent once, and a
new session, after a lost one, gets the thread again once. A turn from before this column
kept nothing, so the first wake after an upgrade sends each thread once more. A scope with
nothing read, a new session's first wake, shows only its last 20 unread messages, with a line
saying the older ones are in `read_thread`. A wake that finds nothing unread runs no turn and starts no agent: an earlier turn
already sent what woke it. One routing pass wakes a role once: when several messages,
top-level or in any thread, wake the same role, for example two `post_to_space` calls of
one turn that name `@architect`, they are one wake, and its text has one block for each thread, with its `Asked together` line. The woken
role's reply is stored in the thread the header of the first message names (`parentId` is the
thread root). When a top-level message woke the role, the reply is top-level too: it is the
root of a thread the role owns, and the user's reply in it goes to the role.

The first turn and the wakes after it are one chain. They run one at a time, in the order
the messages were stored, and the messages each turn stores are routed in turn and queued at
the end. The Space has a turn running until the chain is done, so a `wrap`, an `attach`, a
`continue`, a `rollback` or a `close` during the chain is 409. A `send` during a message chain
is accepted: the message is stored, and its wakes join the end of the chain's queue and run after the current turn and the wakes it
queued. A `send` that finds a message chain starting waits until it runs, so the message is stored
after the one the chain starts with and ids follow the order of the sends; if the chain never
starts, the send starts a chain of its own. A `send` while a wrap, an attach, a `continue` or a
`rollback` starts is 409. Turns still run one at a time. If the chain ends first (a failed turn, a stop, a
routing failure), a queued message's wakes are held, not run. When the hop cap or the token
budget stops the next wake, it is held and `continue` runs it (see [Caps](#caps)). A turn that fails ends the chain; the wakes still waiting are dropped. An example chain: the user asks the lead; the
lead's reply says `@architect ...`; the architect runs and posts a plan, a new thread it
owns; the chain ends because the plan names no one. The lead gets no turn after the plan.

`send_to_role` stores a direct request outside the public `message` table. When its sender's
turn ends, only the named role wakes, and a turn that also handed off in its reply wakes both
in the order the messages were stored. Reaching the
hop or token cap pauses the chain with the held direct wakes. Direct and public wakes share
the Space's hop count, so switching between them never resets it. Its prompt starts `[Space N, direct message N, chain
N, source: direct request]`, names the sender, and carries the body. The recipient's ordinary
reply is one direct response in the same chain (`replyToId` points to the request) and wakes
the sender with `source: direct response`. The sender's ordinary reply to that response is
not stored or routed. A further exchange requires an explicit `send_to_role`. The user reads
all direct records with `GET /spaces/{id}/direct-messages`; an MCP token reads only records
whose `from` or `to` is its role. `GET /search` finds public and direct messages together for
the user; the MCP `search_sessions` returns a direct hit only to its sender or recipient.
A stop that sheds a queued direct wake, and a user
message that drops a held direct wake after a hop cap pause, each give the request a `failed`
response with body `stopped` or `dropped`, so the window tells them from a delivery that failed.
A request its sender sent in a turn that ends stopped or failed, or that a restart cuts, gets a `failed`
response with body `stopped` or `failed` (a restart gives `failed`). Closing a Space, by approving its
wrap-up or closing its work item, gives each request still `waiting` in it a `stopped` response. The
sender's next prompt carries a line `reef wrote: Your direct request to <role> failed: <first line>` for each of
its `failed` requests whose failed response came after the sender's last sent turn. Wrap-up prompts list the
Space's direct messages, marking a `failed` attempt `(failed)`. Partial output from a failed recipient turn is appended as
a `failed` response attempt: it is inspectable but never routed. Retry appends a `done`
response; a partial unique index allows only one `done` response per request, and only that
response wakes the sender. `sourceMessageId` is the public message whose wake the sender was answering when it called
`send_to_role`, read from the sender's running turn; a request sent from a direct wake keeps the
source of that wake, and a request sent from a turn with no public wake has `null`. `send_to_role` is
refused with `invalid:` unless its sender has a running turn. A response keeps the source of
its request. Public `GET /messages` history is unchanged.

A request's `state` is what the window reads. It starts `waiting`, is `delivered` once a turn runs
for it, and is `answered` when its `done` response is stored or its recipient ended its turn with
nothing to say. It is `failed` or `stopped` when its wake or its response's wake did not run to
the end: the recipient's turn failed or was interrupted, a held wake was dropped or shed, or a
restart cut it. A response has no response of its own, so a failed or dropped wake of it fails
the request, and a request whose answer never reached its sender never reads `answered`.
Retrying that wake puts `answered` back.

### Exchanges

An exchange is a direct request that lives in its destination Space (`spaceId`) and records the
conversation it came from (`originSpaceId`, and `originMessageId`, the note there). `kind` is
`request`, `question` or `guidance`; `requestKey` makes creating one idempotent, so a second create
with the same key returns the first row and starts nothing. Its response is the usual response row;
when the origin is another Space it is also posted there as a message from the recipient (a failed
one reads `No reply: <state>, <reason>`), and both messages carry the exchange id. A request has
one more state, `cancelled`, for an exchange withdrawn while `waiting`, or whose recipient was
removed from the Space: a request that waited for a removed Bot, an exchange or an ordinary
same-Space one, gets a failed response `cancelled: <Bot> was removed from the Space`, so it stays
readable with its original body.

Delivery is `deliverExchange`: it queues the wake behind the running chain's current turn, or starts
a chain when the Space is idle. A paused Space leaves it `waiting`, and `continue` delivers it with
the held wakes. A closed Space, or a recipient that is not a member, fails it with a response that
says why. A turn is created for an exchange only from `waiting`, moving it to `delivered` in the same
transaction, so delivering the same exchange twice runs it once. A wake whose exchange is no longer
`waiting` is skipped: it runs no turn, and nothing is routed or woken for it. When the destination
has a turn running that cannot take the wake (a wrap-up, a continue or refresh in progress), the
exchange stays `waiting` and the Space is paused, so `continue` delivers it; delivery and retry
still answer success. A response, its request's state and the reply in the origin conversation are
stored together.

The reply to a Bot asker is also stored with its wake: a hold on the origin conversation, in the same
transaction as the response. It is released like any hold: at once when the origin is idle or running,
when the origin is paused by `continue`, when a refresh, member removal or continue holding the origin for a moment ends (the origin is not paused for it), and after a restart (every open Space holding a pending hold
is paused at start). It is claimed once, so a reply wakes its asker once. An exchange an asking turn
created is delivered only when that turn ends well; when the turn fails or is stopped, the exchange
is `cancelled` and the origin reads `No reply: the asking turn failed` (or `stopped`).

After a restart `answered` and `failed` exchanges are unchanged. A `waiting` one stays `waiting`;
at start, every open Space holding one is paused (`app restarted: wakes were waiting to run`) and
`continue` delivers it. A `delivered` one whose turn the restart cut becomes `failed` with the
response `interrupted while running; check its activity before retrying`: what the turn did is
unknown, so it is never replayed, and the user retries it on purpose. An exchange that already has a
`done` response cannot be retried.

### Consultations

A Bot in a direct conversation asks another Bot with the `consult` MCP tool. The question runs in a
Space of kind `consultation`, titled `Consultation from <asker> · <first words of the first question> #<Space id>`: one member, the consulted Bot,
leading it, no work item and no Team. It takes the Bot's own profile and permissions as a
direct conversation does, runs in its own chat folder, and refuses `POST /spaces/{id}/members`
(`400`). The consulted Bot has no `consult` tool, so consultations do not nest; its prompt tells it
to name another specialist it needs in its reply. The asker's conversation stays a conversation: it
is not moved and gains no member, and nothing is written into the consulted Bot's own
conversations.

Each question is an exchange of kind `request` in the consultation, with the asker's conversation
as `originSpaceId`. The tool stores it, with the note `Asked <Bot>: <question>` in the origin, and
the server delivers it when the asker's turn ends. The reply is the response row, posted in the
origin as a message from the consulted Bot, and it wakes the asker there with `[Space N,
consultation N, exchange N]` and the reply. A related follow-up passes the same `consultation` id
and adds another exchange to it; a question without one makes a new consultation. The key of a
tool call is its turn, Bot, consultation and question, so repeating a call in a turn returns the
same exchange. Messages that stand for an exchange carry its id, so the window links the origin
chat to the consultation and each question and reply has its own link.

The consultation is an ordinary Space for the rest: its history stays readable, its Bot's session
is refreshed and revoked with the Bot's others, and a restart fails a cut consultation turn with
the interruption note and does not replay it.

### Sending to a Space

A Bot in a direct conversation sends a question or an instruction to a Bot that already works in a
Space the conversation refers to ([Space references](#space-references)) with the `send_to_space`
MCP tool. Nothing is added to either Space and no session is made: the exchange runs as a direct
wake of the recipient in the destination Space, in the Bot's existing session there (the Space's
live chain queues it behind a running turn, or starts one when idle). The exchange lives in the
destination Space with kind `question` or `guidance`, links the conversation, and posts `Sent to
<Bot> in <Space>: <text>` there as a message linked to it. Its key is the turn, Space, Bot, kind
and text, so a repeated call returns the same exchange.

A question's reply is posted in the conversation as a message from the recipient linked to the
exchange, and wakes the asker with `[Space N, question to Space N, exchange N]` and the reply, as
a consultation reply does. An instruction is what the user told the Bot to pass on: it reaches the
recipient as `User instruction relayed by <Bot> from Space N (...): <text>` (see [Guiding a
Bot](#guiding-a-bot)), and its reply is posted as a receipt but wakes nobody. Both are decided by the exchange state, shown by the window
on the linked message with links to the Space and the Bot's activity:

| Destination | Outcome |
| --- | --- |
| Open and idle or busy | `waiting` until its turn starts, then `delivered`, then `answered` |
| Paused | stays `waiting` ("Waiting for Space to resume"); sending never resumes the Space, `POST /spaces/{id}/continue` delivers it once |
| Closed, or the Bot is not a member | `failed` at once with the reason, posted as `No reply: ...` in the conversation; no turn starts |
| Cut by a restart while running | `failed` with the interruption note, never replayed; `POST /exchanges/{id}/retry` runs it again |
| Withdrawn while waiting (`POST /exchanges/{id}/cancel`) | `cancelled` |

Every outcome keeps the origin and destination links of the exchange.

### Guiding a Bot

The user guides a member Bot from its activity view (`/spaces/{id}/bots/{botId}`) or from a
consultation Space, with `POST /spaces/{id}/bots/{botId}/guidance {"body", "key"}`. It makes an
exchange of kind `guidance` from the user, with the Space as its origin and destination, and
stores `To <Bot>: <text>` there as a user message linked to it; the response is 201 with the
exchange (`{request, response}`). `key` is the client's own id: sending it twice makes one
exchange. A closed Space is 409, a Bot that is not a member 404, an empty `body` or `key` 400.

The Bot receives `Guidance from the user (sent at <time>, message <id>): <text>` as a direct wake
in its existing session. The Space's live chain runs guidance at the next turn boundary, after the
whole running turn, ahead of any Bot handoff waiting behind it (guidance among themselves in the
order sent). The end of a tool call is not a boundary, so guidance stays `waiting` until its own
turn starts. Ordinary user messages keep their order, and run in stored order with guidance, all
of them ahead of Bot handoffs. A Space held for a moment by a refresh, a member removal or a
continue, or by a turn that cannot take the wake, delivers its waiting exchanges, guidance first,
when that ends, with no continue; only a Space that cannot run then (shutdown) is paused. A paused Space keeps guidance `waiting`
("Waiting for Space to resume"), sending never resumes it, and `POST /spaces/{id}/continue` runs
it first. `POST /spaces/{id}/stop` is separate: it stops the running turn and keeps waiting guidance
`waiting`; the next turn the Space starts runs it first.

The `To <Bot>: <text>` message is the Space's record that user guidance happened: it links the
exchange, whose own turn holds the detail, and nothing else is copied into the conversation. Every
other member of the Space also gets the guidance, once: the next turn it starts after the guidance
turn started, whatever woke it, begins with `User guidance (to <Bot>, message <id>): <text>`.
The `To <Bot>: <text>` line is then left out of that wake's "New in the Space" list, and out of the
guided Bot's wakes before its guidance turn runs and after, which get it as its own guidance. A Bot's
"New in the Space" list likewise leaves out the replies to what it asked from that Space, which it
gets as reply wakes. A Bot that joined after the guidance
was given does not get it.
Wakes already queued behind the guidance therefore start after it and carry it. A turn already
running when the guidance arrives is not changed and is never described as corrected. Only Bot
handoffs and Bot messages queue behind guidance; messages the user sent earlier keep their place
ahead of it.

An instruction a Bot sends with `send_to_space` relays the user's words, so it is treated the same
way: it runs ahead of Bot handoffs, and the other members get it once, as `User instruction
relayed by <Bot> from Space N (to <Bot>, message <id>): <text>`.

Guidance given in a consultation also reaches the Bot that asked: its next turn in the asking
conversation, after the consulted Bot answered the guided turn, begins with `User guidance in
Consultation N (to <Bot>): <text>` and the consulted Bot's reply, once. It starts no turn by itself.

The REST API has no authentication and listens on localhost only (as for every route), so a Bot
with a shell can call this route too and forge guidance. That is accepted for a personal local app.

### Space references

A user message can refer to a Space with the token `#[Name](space:ID)`. The composer writes it
when the user picks a Space after `#` (work item, free and consultation Spaces, never personal
chats); the id is the identity, so two Spaces with the same name are two different references. On
`POST /spaces/{id}/send` the server stores each referenced id in `message_ref(message_id,
space_id)`. The row has no foreign key on the Space: when the Space is later deleted the reference
stays and resolves as unavailable. The window renders the token as a chip linking `/spaces/{id}`.

A reference supplies context only. It adds no Bot or Project to either Space, does not move a
personal chat (only a line that starts with `@name` does), gives no execution authority and starts
no turn, session or workspace in the referenced Space. The wake text of the message lists each
reference (`Referenced Space N 'Name' (kind, closed)`, or `unavailable: it was deleted`) and points
at `read_space` ([docs/memory.md](memory.md#reef-mcp)).

### Space names

A Space created with the new-space form's default name has `space.name_auto` set to 1.
The first user message sent to it (`POST /spaces/{id}/send`, not a wrap command)
takes the one naming attempt (`name_auto` 2) and asks the lead's backend, in the background, for a name: one
throwaway turn in one shared empty folder (`reef-name` in the temp folder), with no tools (claude on `haiku`, codex
`read-only`), so it makes no turn or message. The reply is cut to the convention: its first
line, lowercase words of letters and digits (a hyphen splits words), at most four words and 40 characters
(`fix login redirect loop`). The name is stored only if the attempt is still the Space's, in one
statement, and `name_auto` goes to 0; a `PUT /spaces/{id}` with another name, which sets
`name_auto` to 0, got there first and wins. A failed call, an empty name or a stopped server keeps the
current name and is not tried again. Spaces that existed before the column keep `name_auto` 0.

### Events

`GET /spaces/{id}/events` is a Server-Sent Events stream. It works for a Space that
is not live: it waits. A `: keepalive` comment comes every 15 seconds. Seven kinds of event:

- `turn`: `{"role":"architect","status":"running"}` when a turn of that role starts, then
  `{"role":"architect","status":"done"}` or `{"role":"architect","status":"failed","error":"interrupted"}`
  when it ends. A chain of wakes sends one pair for each turn, with no `idle` between them. A
  wrap turn in which the agent made no proposal ends `{"role":"lead","status":"done","note":"no
  proposal made"}`. `{"status":"idle"}` has no role. It is sent only when a subscriber
  connects and no turn runs. A turn woken by a direct message carries `"directMessageId":N`, the id of that direct message, in each of its frames; the Space shows such a turn
  as working only, without its text or tool calls; the open Coordination panel of its exchange shows its text and tool rows live while it runs, and only the stored direct messages once it ends.
- `agent`: one [agent event](agents.md) as JSON, as it comes, from the turn that is running.
- `pause`: `{"reason":"..."}` when a cap pauses the Space: when a send finds the
  token budget used, or when the hop cap holds a wake. `{"reason":""}` when `continue`, or a user message after a hop cap pause, clears the pause.
- `note`: `{}`, sent when a message is posted outside a turn, such as a [GitHub
  poll](#github-poll) note, so a subscriber knows to re-read `GET /messages`.
- `name`: `{}`, sent when the provider's name replaces a Space's default name, so a subscriber knows to re-read `GET /spaces`.
- `direct`: `{}`, sent when a chain ends without running a queued direct wake and gives it a
  `failed` response after the last `turn` event, so a subscriber knows to re-read `GET /spaces/{id}/direct-messages`. Every
  other change to a direct message happens before a `turn` event that ends its turn.

A subscriber is brought up to date when it connects, so a reconnect needs no other
request to know the state. The server keeps the events of the running chain in memory. A
subscriber that connects during a chain first gets `turn` running and every `agent` event
of the chain so far, then the live ones. One that connects between chains first gets
`turn` idle. Either gets `pause` with the reason when the Space is paused. It does not get
The stored messages
(`GET /messages`) are the history. A slow subscriber only lags; it never holds
up the agent. When the server stops, a subscriber gets the last frames before the
stream ends.

```
event: turn
data: {"role":"lead","status":"running"}

event: agent
data: {"kind":"tool_call","toolId":"t1","toolName":"Bash","input":{"command":"ls"}}

event: agent
data: {"kind":"text","text":"@architect design the form"}

event: turn
data: {"role":"lead","status":"done"}
```

### Inbox

`GET /inbox` lists what waits on the user, in two groups. Needs you and the replies of FYI are
read from the rows of Spaces, holds, turns, messages and Work items, so they have no table and a
restart keeps them; the FYI items of Work items are rows of `inbox_fyi`. Each item has `kind`,
`group`, `spaceId` (0 for an item of a Work item, which has no Space), `workItemId`, `text` and
`at`, and the id to act with:

| `group` | `kind` | Listed while | `text` | Also has | Cleared by |
| --- | --- | --- | --- | --- | --- |
| `needs_you` | `paused` | the Space's `paused` is not empty and the pause holds no wake: the reason is a cap (token budget, tool calls) or a routing or store failure | the reason | | `continue`, or a user message after a hop cap pause |
| `needs_you` | `held` | the same, and the pause holds a wake: the hop cap, or a token budget that ran out in the middle of a chain | the reason and `(N wakes held)` | | `continue` runs the held wakes; a user message after a hop cap pause drops them |
| `needs_you` | `failed` | a turn is retryable (see [Spaces live](#spaces-live)) and its error is not `interrupted`: a turn the user stopped is a decision, not an item | `architect failed: server restarted` | `turnId` | `retry`, or any later turn of that role, so of several failed turns only the newest shows |
| `needs_you` | `review` | a top-level Work item is in review | the item's title | `workItemId` | the user sets Done or sends it back |
| `fyi` | `sub_item` | a Bot made a sub-item with `create_sub_item`; `workItemId` is the sub-item | who made it, and its title | `id` | `POST /inbox/dismiss` |
| `fyi` | `proposed` | a Bot proposed an item with `propose_work_item`; `workItemId` is the new Backlog item | who proposed it | `id` | `POST /inbox/dismiss` |
| `fyi` | `started` | the queue moved an item to in progress, including one sent back from review | `Started from the queue: ` and its title | `id` | `POST /inbox/dismiss` |
| `fyi` | `space_proposed` | a Bot proposed a Space with `propose_space`; `workItemId` is null, `proposal` has `id`, `proposedBy`, `name`, `botIds` and `coordinatorBotId` | who proposed it, and its name | `id` | `POST /inbox/dismiss`; the window opens the new Space form with the proposal filled in, and dismisses it when the Space is created |
| `needs_you` | `question` | the Space's newest message is from an agent (not `user` or `reef`) and woke nobody: no turn running, no item in Needs you for the Space, and the Space's newest turn was not stopped (the part of a reply a stop kept is not waiting); and the message ends in a question mark, ignoring trailing spaces and the marks `*`, `_`, `)` and `"` | the first 2000 characters of that message | `messageId` | any new message, such as a send |
| `fyi` | `reply` | the same, and the message does not end in a question mark | the first 2000 characters of that message | `messageId` | any new message, such as a send; or `POST /inbox/dismiss`, which hides it until a newer message arrives |

`at` is the time of the failed turn's end, of the message, of the proof for `review`, of the row
for the FYI items, and for `paused` and `held` the end of the Space's newest turn. A Space that
has a chain running lists no `question`, `reply`, `paused`, `held` or `failed` item, since the chain is not
over and `continue` or `retry` would be 409.

`POST /inbox/dismiss` takes an item as `GET /inbox` lists it. An FYI item of a Work item is
deleted, so it does not come back, and a `reply` is hidden. An item in Needs you (`failed`, `paused`, `held`, `review`, `question`) is 409, as in `a paused item cannot be dismissed, it leaves when its cause is resolved`.

`GET /inbox/events` is one Server-Sent Events stream for all Spaces, with a
`: keepalive` every 15 seconds. Its only event is `inbox`, whose data is the whole list as
`GET /inbox` returns it. The first one comes when the stream connects; after that one comes each
time the list differs from the last one sent. The list is read again after every event of every
Space (turn, agent, pause), when a chain ends, and after every request that is not a
`GET`, since a delete or a `POST /messages` can change it with no Space event. A change
a `reef` process makes straight in the database shows at the next keepalive tick at the latest.
When the server stops, a stream sends the last list and ends.

```
event: inbox
data: [{"kind":"reply","group":"fyi","spaceId":5,"workItemId":2,"text":"Plan: add the form and a test.","at":"2026-10-14T09:00:10.123456Z","messageId":28}]
```

## Wrap-up and attach

`POST /spaces/{id}/wrap` does what a send of the text `/wrap` does: it runs the wrap-up
turn of [`Session.Wrap`](../space/session.go) (see [memory.md](memory.md#wrap-up)). The
agent calls `propose_wrap`; when the turn is done, `GET /spaces/{id}/proposal` returns the
pending proposal, or 404 when the agent made none. `POST /proposals/{id}/approve` and
`/reject` decide it with the same effect as `reef wrap approve` and `reject`. Approve
saves the proposal's items, retires its `retire` ids, closes the Space (`closedAt`) and stops its agent, and returns `itemsAdded`, the items it saved (one already saved in its scope with the same body is skipped; each carries a `source` link to the proposal), and `warnings`. Two approvals of one proposal at the same moment run one after the other: the second is `400 already approved`. Saving the items comes first and is not part of the transaction that saves the brief and closes the Space: when it fails, the proposal stays pending and the Space open, and approving again finishes without saving an item twice; a closed Space refuses sends, wraps and wakes with 409 until `POST /spaces/{id}/reopen`. A brief proposal (`propose_brief`, `kind` `brief`) does not close the Space or block its turns or a wrap; only a
pending wrap blocks another wrap (409 `space 2 has a wrap-up waiting for approval`). A proposal that is already decided is
400, like `invalid: proposal 1 is already approved` in the CLI.

### Attach a Space to a work item

`POST /spaces/{id}/attach` gives a free Space a work item: `{"workItemId": 3}`, a work item
of the Space's project (its lowest-id project, see [Space projects](#space-projects)). A Space that has a work item, and a direct conversation, are 400.
The Space must be idle (409 while a turn runs), the project a git repo with a commit, and
the work item without another active Space (409 `work item 3 already has an active Space`);
a refused attach changes nothing. The attach sets the Space's `workItemId` and `kind`, moves
its `space` memory items to the `work_item` scope of the work item, stops the Space's agents,
so the next wake of each role resumes its session in the work item's root folder, and posts a
message from `reef`: `This Space is now attached to work item "<title>" (<id>). It works in
<root folder> on work-item/<id>.`, with ` Uncommitted changes stay on the main checkout.`
after it when the main checkout has uncommitted changes. No agent turn runs.

## Caps

Two caps pause a Space. All are constants in the code, not settings.

- Tokens per Work item: the input, output and cache write tokens of the turns of every Space of
  the item's top-level Work item (sub-items included), of every role, cache reads not counted,
  against the item's `tokenBudget` (1,000,000). A Space with no item counts its own turns
  against its own budget. A running turn
  counts the usage it has streamed so far. A send, or a wake in a chain, that finds the sum at or
  over the budget pauses the Space with `token budget of 1000000 used: 1000312 tokens`. The budget is
  checked only between turns: a turn that crosses it runs to its end.
- Agent hops: a hop is a turn that an agent's message woke. The turn that answers the user's
  message is not a hop. Only talk-only turns count: a turn with no tool call, or whose tool calls
  are all Reef's own MCP tools (`mcp__reef__*`). Any other tool call, such as `Edit` or `Bash`,
  resets the count to 0. The server counts hops per Space since the last user message or the
  last such tool call. The 7th hop does not run: the Space is paused with `hop cap: 6 agent hops without a user
  message`, and the `turn` event of the turn before it is followed by a `pause` event. The wake
  that did not run, and the wakes behind it, are stored as holds.

A send to a paused Space is 409 with the reason in the error, except after the hop cap: that
pause is for agents, so a user message clears it, resets the hop count to 0, drops the held
wakes and runs. `POST /spaces/{id}/continue` clears the pause. When the token sum is at or
over the budget it also sets the Work item's `tokenBudget` (the Space's own with no item) to the
sum plus 1,000,000. `continue` on a Space that is not paused, or has a turn running,
is 409.

Held wakes: when a wake cannot run because of a pause (the hop cap, or a token budget that ran
out in the middle of a chain), the server stores it and the wakes behind it as `hold`
rows of the `approval` table. `continue` claims the Space's pending holds once, resets the
hop count to 0 and runs them in order as a new chain, with `pause` `{"reason":""}` as the
chain starts. If the chain cannot start, the Space stays paused and keeps the holds. That
chain can reach the hop cap again, and then pauses again. Sending a user message resets the
hop count to 0 too, and drops the holds; a dropped direct request gets a `failed` response with
body `dropped`. The holds are stored, so
they survive a restart; the hop count is not, so a chain that runs after a restart counts from 0.

The same pause holds work when the server cannot
read the messages or the direct messages a turn stored to route them, or when a store error
stops the check before the next wake. The Space pauses with the error as the reason. These wakes are held in memory only, since the database failed, until the server stops and stores them. `continue` does the routing read again from
the message and the direct message before the turn's reply, and runs what that wakes. If it fails again the Space
stays paused.

No role has a tool call cap: a turn runs until the agent ends it, the user stops it, or the
token budget is used. Before each line, `reef chat` runs the same pause and token budget check as send (`space.CheckCaps`)
and prints the reason instead of running the line. The line `/continue` does what
`continue` does (`space.Continue`). See [memory.md](memory.md#reef-chat). Hops do not
exist in `reef chat`: it wakes nobody.

## GitHub poll

`reef serve` polls GitHub for each top-level work item that has a workspace of its home Project, every `REEF_GH_INTERVAL`
(default 60s), once right away and then on that interval until the server closes: `gh pr
view work-item/<id> --json number,title,url,state,isDraft,mergeable,reviewDecision,headRefName,baseRefName,statusCheckRollup,body,additions,deletions,files`, run in the project's
folder. `statusCheckRollup` items are either a `CheckRun` (`name`, `status`, `conclusion`,
`detailsUrl`, `completedAt`) or a `StatusContext` (`context`, `state`, `targetUrl`,
`startedAt`); both are read.

When the PR first appears, changes state (`open`, `merged`, `closed`) or a check run
finishes (a `CheckRun` whose `status` is `COMPLETED`, or a `StatusContext` whose `state` is
not `PENDING`/`EXPECTED`), the work item's active Space (its one open Space) gets
one message from author `reef`: `PR #12 open: <url>` or `<check name> succeeded`/`failed`.
A check run is identified by its name plus its `detailsUrl`/`targetUrl` (falling back to
`completedAt`/`startedAt`) as its run key; a rerun of the same check name, such as CI run
again on a later push, is a different run key and is noted again, and two runs of the same
name in one rollup (a matrix job, two workflows) are two rows, both noted. The state posted
is stored one row per `(work item, name, run key)` (migration 42
[`042_work_item.sql`](../store/migrations/042_work_item.sql), tables
`work_item_github` and `work_item_github_check`), so a restart does not repeat it and a second
poll with the same answer posts nothing. No `gh` on `PATH`: nothing is posted, logged once.
`gh` erroring (not logged in, no PR for the branch): nothing is posted, logged once per
work item for that cause; logged again once the cause changes. A work item with a workspace but no
Space yet has nowhere to post: the note is recorded as sent only once it is actually posted,
so it is not lost and posts once the first Space exists.

Every failed run in a rollup (a `CheckRun` whose `conclusion` is `FAILURE`, `ERROR`,
`TIMED_OUT` or `STARTUP_FAILURE`, or a `StatusContext` whose `state` is anything but
`SUCCESS`/`PENDING`/`EXPECTED`) also wakes the `developer` role in that Space, once per run,
also across restarts (`work_item_github_check.woken`): several failed runs in the same rollup,
not yet woken, are one wake, not one each, its text naming every one of them plus the last 50
lines of `gh run view <run id> --log-failed` for each run whose id can be read from the
`CheckRun`'s `detailsUrl` (`/actions/runs/<id>/`); no run id, no log for that run. A Space
whose team has no `developer` role gets the note only. The wake text, including any log
fetch, is built before the Space is claimed, so a user send that arrives during the fetch
finds the Space still idle and is not refused. The wake runs as its own chain, the way a
retried turn does (decision 39), attributed to author `reef` rather than `user`: it counts as a hop like any agent-originated wake, so a run of
failing checks still pauses at the hop cap instead of resetting the count every time; the
Space's other caps (pause, token budget) also still apply. A Space with a turn running, or
paused (including by the hop cap), cannot take the wake on that poll; the note still posts,
the wake is retried on a later poll once the Space is idle, and the busy cause is logged
once, not every poll.

## Append-only

The API has no way to change or remove these rows.

- Messages and decisions: create and read only. `PUT`, `PATCH` and `DELETE` on
  `/messages/{id}` and `/decisions/{id}` return 405. The only way a message or decision
  goes away is a cascade from its Space or work item. Deleting a Space does not delete
  its decisions; they only lose `messageId`. A decision's body is never edited, but it can
  be retired (`retiredAt`, `replacedBy`) by a new decision that replaces it or by an
  approved proposal; see decision 76.
- Briefs: `PUT /work-items/{id}/brief` adds a version and never edits one. Old versions
  stay in `/brief/versions`. Role notes are not versioned; `PUT` replaces them.
- Turns: created `running`, may get a `sessionId` while running, ended once with `PATCH`
  to `done` or `failed`. After that every `PATCH` returns 400. A turn has no delete route
  and no other edit, except that a `failed` turn is marked `retried` once, by `retry`. Restart
  recovery keeps a session id set this way.

## Restart recovery

A turn is `running` only while a server owns it. If the server stops, the agent process
dies with it, so a `running` turn in the file can no longer finish. At start, once it
holds the data folder and has bound `--addr` but before it serves requests, `reef serve`
fails them all, except the turns of a live `reef chat`. A second `reef serve` never gets
this far on a folder in use, so it cannot fail the turns of the server that owns it:

- Each `running` turn becomes `failed`, with `error` `server restarted` and `endedAt` set.
- Each affected Space gets one message from author `reef`, naming the turns. For turns 2
  (developer) and 3 (tester) in one Space:

  `Turn 2 by developer, turn 3 by tester failed: server restarted. The worktree may be dirty; retry or roll back.`

- With one turn: `Turn 2 by developer failed: server restarted. The worktree may be dirty; retry or roll back.`
- Only a Work item's Space has a worktree: the note of a free Space, a direct conversation or a consultation ends after `server restarted.`
- The server logs `marked N running turns failed: server restarted`.
- A Work item's Space that is not paused, whose item is `in_progress` and whose turn was cut (or whose first message was stored but whose first turn never began) is then paused with the reason `interrupted while running; check its activity and changes before continuing`, which puts the item in `needs_you`. What the cut turn did cannot be told, so it is not run again until the user has looked and continues. If the cut turn took checkpoints, another `reef` message reports the changed files in each repository separately, for example `Interrupted by a restart. Since the turn started, app: 1 file changed; lib: no changes.` A Space the user or a cap paused keeps its own reason, and pending holds stay held. Nothing else resumes paused work.
- A `running` turn in a Space whose `chat-<space id>.lock` a live `reef chat` holds is
  left `running`, and that Space gets no message from `reef`. `reef chat` is a separate
  process, so a server restart does not kill its agent, and its turn can still finish.
  The count in the log does not include it. A chat that died leaves its lock free, so
  its turn is failed like any other.

An agent may have left half-done edits in the worktree, so the message tells the user to
check before retrying. A turn failed this way keeps its `wakeText`, so `POST /turns/{id}/retry`
can run it again (see [Spaces live](#spaces-live)).

A chain of wakes does not run on after a restart, but its waiting wakes are kept. When the
server stops, the wakes a chain had queued behind the running turn, and the wakes held in memory
because the database failed, are stored as holds, and the Space is paused with `app restarted:
wakes were waiting to run` unless it is paused already. The pause shows in the inbox, and
`continue` runs the holds. A routing read that failed before the stop is done again then, and what it
wakes is held too; if it fails again, the Space gets a message from `reef` saying which messages woke
nobody. Only the hop count is lost: a chain that runs after a restart counts
from 0. A pause is stored with its holds, so both survive too.
A direct request that did not run still gets its `failed` response.

## Try it

Start a fresh server, then run these in order. Ids and timestamps follow from an empty
data folder; your timestamps will differ.

```
reef serve --data /tmp/reef-demo
```

```
$ curl -s -X POST -H 'Content-Type: application/json' localhost:7700/projects -d '{"name":"shop","path":"/tmp"}'
{"id":1,"name":"shop","path":"/tmp","baseBranch":"","createdAt":"2026-10-13T09:00:00.123456Z"}

$ curl -s -X POST -H 'Content-Type: application/json' localhost:7700/work-items -d '{"projectId":1,"type":"bug","title":"Login fails","acceptanceLines":[{"text":"no 500"}]}'
{"id":1,"projectId":1,"parentId":null,"type":"bug","title":"Login fails","description":"","status":"backlog","proposedBy":"","queuePosition":null,"tokenBudget":1000000,"createdAt":"2026-10-13T09:00:01.123456Z","closedAt":null,"acceptanceLines":[{"position":1,"text":"no 500","checked":false,"note":""}],"ownerBotId":null,"participantBotIds":[],"targets":[{"projectId":1,"projectName":"reef","proposedBy":"","authorizedBy":"user","createdAt":"2026-10-13T09:00:01.123456Z"}]}

$ curl -s -X POST -H 'Content-Type: application/json' localhost:7700/work-items -d '{"projectId":1,"parentId":1,"type":"task","title":"Reproduce"}'
{"id":2,"projectId":1,"parentId":1,"type":"task","title":"Reproduce","description":"","status":"backlog","proposedBy":"","queuePosition":null,"tokenBudget":1000000,"createdAt":"2026-10-13T09:00:01.223456Z","closedAt":null,"acceptanceLines":[],"ownerBotId":null,"participantBotIds":[],"targets":[{"projectId":1,"projectName":"reef","proposedBy":"","authorizedBy":"user","createdAt":"2026-10-13T09:00:01.123456Z"}]}

$ curl -s -X PUT -H 'Content-Type: application/json' localhost:7700/work-items/1/brief -d '{"body":"Email and password login","author":"user"}'
{"id":1,"workItemId":1,"body":"Email and password login","author":"user","createdAt":"2026-10-13T09:00:02.123456Z"}

$ curl -s -X POST -H 'Content-Type: application/json' localhost:7700/spaces -d '{"projectId":1,"workItemId":1,"name":"build","backend":"claude"}'
{"id":1,"projectId":1,"workItemId":1,"name":"build","role":"lead","paused":"","tokenBudget":1000000,"team":"solo","createdAt":"2026-10-13T09:00:03.123456Z","closedAt":null}

$ curl -s -X POST -H 'Content-Type: application/json' localhost:7700/messages -d '{"spaceId":1,"author":"user","body":"Start with the form"}'
{"id":1,"spaceId":1,"parentId":null,"author":"user","body":"Start with the form","createdAt":"2026-10-13T09:00:04.123456Z"}

$ curl -s -X POST -H 'Content-Type: application/json' localhost:7700/memory -d '{"kind":"decision","body":"Use sessions, not JWT","why":"One server, no token refresh","scope":"work_item:1"}'
{"id":1,"kind":"decision","body":"Use sessions, not JWT","why":"One server, no token refresh","scope":"work_item:1","author":"user","role":"user","botId":null,"status":"accepted","retiredAt":null,"replacedBy":null,"createdAt":"2026-10-13T09:00:05.123456Z"}

$ curl -s -X POST -H 'Content-Type: application/json' localhost:7700/turns -d '{"spaceId":1,"role":"developer","backend":"claude"}'
{"id":1,"spaceId":1,"role":"developer","backend":"claude","sessionId":"","status":"running","error":"","usage":{"inputTokens":0,"outputTokens":0,"cacheReadTokens":0,"cacheWriteTokens":0},"startedAt":"2026-10-13T09:00:07.123456Z","endedAt":null,"wakeText":"","wakeParent":null,"retried":false,"checkpoint":null}

$ curl -s -X PATCH -H 'Content-Type: application/json' localhost:7700/turns/1 -d '{"status":"done","sessionId":"3c1296cc","usage":{"inputTokens":4,"outputTokens":123,"cacheReadTokens":22898,"cacheWriteTokens":0}}'
{"id":1,"spaceId":1,"role":"developer","backend":"claude","sessionId":"3c1296cc","status":"done","error":"","usage":{"inputTokens":4,"outputTokens":123,"cacheReadTokens":22898,"cacheWriteTokens":0},"startedAt":"2026-10-13T09:00:07.123456Z","endedAt":"2026-10-13T09:00:08.123456Z","wakeText":"","wakeParent":null,"retried":false,"checkpoint":null}

$ curl -s -X PATCH -H 'Content-Type: application/json' localhost:7700/turns/1 -d '{"status":"failed"}'
{"error":"invalid: turn 1 already ended"}

$ curl -s "localhost:7700/messages?space=1&after=1"
[]

$ curl -s -X DELETE -w ' %{http_code}\n' localhost:7700/messages/1
{"error":"method not allowed"}
 405
```

To see restart recovery: create a turn, stop the server with Ctrl-C, start it again. The
log shows `marked 1 running turns failed: server restarted`, `GET /turns/2` is `failed`, and
`GET /messages?space=1` ends with the message from `reef`.

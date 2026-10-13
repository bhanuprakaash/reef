# Decisions

A user records a decision on a feature: what was decided, why, who decided, and
optionally the message it came from. Decisions are append-only: they are added and read,
never edited or removed one by one.

## Sub-features

- `decision-create` `POST /decisions` needs `featureId`, `body` and `author`; `why` defaults to empty; `messageId` is optional.
- `decision-message` a `messageId` must be a message in a channel of the same feature, else 400.
- `decision-read` `GET /decisions?feature=ID` lists a feature's decisions, oldest first; `GET /decisions/{id}` gets one.
- `decision-append-only` `PUT`, `PATCH` and `DELETE` on a decision are 405.
- `decision-cascade` deleting the message's channel keeps the decision and sets `messageId` null; deleting the feature deletes its decisions.

## How to get to it (user POV)

- `curl` against `reef serve`: `/decisions?feature=ID`, `/decisions/{id}`.

## Driving it with verify-reef

Preconditions:

- Baseline from the README. Fresh server: `$H serve-stop`, then `$H serve-start --data-name decisions`. Ids below assume it.
- Setup: `$H api POST /projects '{"name":"shop","path":"/tmp"}'`, `$H api POST /features '{"projectId":1,"slug":"login","name":"Login"}'`, `$H api POST /features '{"projectId":1,"slug":"signup","name":"Signup"}'`, `$H api POST /channels '{"projectId":1,"featureId":1,"name":"build","backend":"claude"}'`, `$H api POST /channels '{"projectId":1,"featureId":2,"name":"build","backend":"claude"}'`, `$H api POST /messages '{"channelId":1,"author":"user","body":"Use sessions"}'` (message 1), `$H api POST /messages '{"channelId":2,"author":"user","body":"Email only"}'` (message 2).

- **Create.** `$H api POST /decisions '{"featureId":1,"messageId":1,"body":"Use sessions, not JWT","why":"One server, no token refresh","author":"user"}'` gives 201 id 1 with `"messageId":1`. `$H api POST /decisions '{"featureId":1,"body":"No social login","author":"architect"}'` gives id 2, `"messageId":null`, `"why":""`.
- **Create errors.** All 400: `$H api POST /decisions '{"featureId":1,"body":"x"}'` (no author), `$H api POST /decisions '{"featureId":1,"author":"user"}'` (no body), `$H api POST /decisions '{"body":"x","author":"user"}'` (no feature), `$H api POST /decisions '{"featureId":99,"body":"x","author":"user"}'`, `$H api POST /decisions '{"featureId":1,"messageId":99,"body":"x","author":"user"}'` and `$H api POST /decisions '{"featureId":1,"messageId":2,"body":"x","author":"user"}'` (message 2 is in feature 2's channel).
- **Read.** `$H api GET '/decisions?feature=1'` lists 1 and 2; `$H api GET '/decisions?feature=2'` is `[]`; `$H api GET /decisions` gives 400; `$H api GET '/decisions?feature=99'` gives 404; `$H api GET '/decisions?feature=x'` gives 400. `$H api GET /decisions/1` gives 200; `$H api GET /decisions/99` gives 404.
- **Append-only.** `$H api PUT /decisions/1 '{"body":"edit"}'`, `$H api PATCH /decisions/1 '{"body":"edit"}'` and `$H api DELETE /decisions/1` give 405 `{"error":"method not allowed"}`; `$H api POST /decisions/1 '{}'` gives 405 too. `$H sql "select id, body from decision"` still shows the original text.
- **Cascade.** `$H api DELETE /channels/1` gives 204; `$H api GET /decisions/1` is still 200 with `"messageId":null`. `$H api DELETE /features/1` gives 204; `$H api GET /decisions/1` and `$H api GET '/decisions?feature=1'` give 404; `$H sql "select count(*) from decision"` is 0.

## Gotchas

- A decision cannot be fixed or retracted through the API; to correct one, add a new decision that supersedes it.
- `author` is free text, not checked against a role list.
- The `messageId` rule is checked once, on create; losing the message later only nulls the link.

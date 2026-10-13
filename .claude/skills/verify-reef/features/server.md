# Server

A user (or the app) starts `reef serve --data DIR`, which keeps Reef's data in one SQLite
file and answers a JSON REST API on `127.0.0.1`. One server owns a data folder at a time.
The API refuses requests from other web pages, answers every error in JSON, and refuses a
database made by a newer reef.

## Sub-features

- `server-lifecycle` starts, answers `GET /health`, stops on SIGINT with exit 0 inside a second, and keeps its data for the next start.
- `server-lock` a second `reef serve` on the same data folder exits 1 with `data folder ... is in use by another reef`, on the same or a new addr, and changes nothing.
- `server-localhost` listens on 127.0.0.1 only.
- `server-guard` a foreign `Host` gets 403, a `POST`/`PUT`/`PATCH` without a JSON `Content-Type` gets 415.
- `server-errors` every error has a JSON body: 400, 403, 404, 405, 409, 415; an unknown path is 404.
- `server-newer-db` a data folder whose `user_version` is newer than the newest migration is refused.
- `server-odd-names` a data folder name with spaces, quotes or non-ASCII letters, or nested, works.

## How to get to it (user POV)

- `reef serve [--addr 127.0.0.1:7700] [--data DIR]` in a terminal; Ctrl-C stops it.
- Any HTTP client on the same Mac, for example `curl localhost:7700/health`.
- Never run it without `--data`: the default folder is the user's real data.

## Driving it with verify-reef

Preconditions:

- Baseline from the README: `build` and `doctor` done. Start fresh: `$H serve-start --data-name server`.

- **Lifecycle.** `$H api GET /health` gives `HTTP 200` and `{"status":"ok"}`. `serve.log` has `listening on 127.0.0.1:<port>, data in .../server`.
- **Localhost only.** `lsof -nP -iTCP -sTCP:LISTEN -a -p $(cat /private/tmp/reef-verify/<id>/serve.pid)` lists `127.0.0.1:<port>`, not `*:<port>`.
- **Guard, host.** `$H api -H 'Host: evil.example' GET /projects` gives 403 `{"error":"forbidden host"}`. `$H api -H 'Host: localhost' GET /projects` and `$H api -H 'Host: [::1]:7700' GET /projects` give 200.
- **Guard, content type.** `$H api -H 'Content-Type: text/plain' POST /projects '{"name":"a","path":"/a"}'` and `$H api -H 'Content-Type:' POST /projects '{"name":"a","path":"/a"}'` give 415. `$H api -H 'Content-Type: application/json; charset=utf-8' POST /projects '{"name":"shop","path":"/tmp/shop"}'` gives 201. `$H sql "select count(*) from project"` is 1: the refused calls wrote nothing.
- **JSON errors.** Each body is `{"error": ...}`: `$H api POST /projects '{bad'` 400, `$H api POST /projects '{"name":"a","path":"/a"} {}'` 400 (data after the body), `$H api POST /projects '{"name":"a"}'` 400, `$H api GET /projects/abc` 400, `$H api POST /features '{"projectId":99,"slug":"a","name":"A"}'` 400 `unknown project 99`, `$H api GET /projects/99` 404, `$H api GET /nope` 404 `{"error":"not found"}`, `$H api DELETE /health` 405 `{"error":"method not allowed"}`, `$H api PUT /projects '{}'` 405. The 409 is in `projects-features`.
- **Lock.** `$H serve-second --same-addr` and `$H serve-second` print `exit: 1`, stderr `reef: open store: data folder ... is in use by another reef`, `ok: refused`. `$H api GET /projects` still answers with the one project.
- **Stop and restart.** `$H serve-stop` prints `exit=0` and `elapsed_s` under 1. `$H api GET /health` now fails with `no running server`. `$H serve-start --data-name server`, then `$H api GET /projects` still lists `shop`.
- **Newer database.** `$H sql "pragma user_version"`, note it. `$H serve-stop`, `$H sql --write "pragma user_version=99"`, then `$H serve-start --data-name server` exits 1 and prints `database is newer than this reef (version 99, this reef knows 1)`. Put the noted version back with `$H sql --write "pragma user_version=1"` and `$H serve-start --data-name server` is ready.
- **Odd names.** `$H serve-stop`, then `$H serve-start --data-name "odd dir 'x' é"` and `$H api GET /health` give 200. `$H serve-stop`, `$H serve-start --data-name "nested/two words"` and `$H api GET /health` give 200 too. `$H sql "pragma user_version"` reads the file in the new folder (1).

## Gotchas

- A refused second server never gets to the data: the lock fails before the database opens. If `serve-second` reports it was still running, that is a real failure of the lock, not of the helper.
- `serve-stop` removes the recorded addr, so `api` fails until the next `serve-start`. The port changes on every start; the data folder does not.
- `sql` is `query_only`. A `delete` through it fails with `attempt to write a readonly database`. That is the helper, not the server.
- `pragma user_version=99` is the only supported way to make a "newer" database; put it back before going on, or later calls fail.

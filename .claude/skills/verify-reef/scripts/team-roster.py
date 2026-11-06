"""team-roster.py FILE ADDR API_LOG ROSTER_OUT

Creates or updates the Bots FILE describes through the running server, then writes the
roster of Bot ids (what `reef team apply` takes) to ROSTER_OUT. A role with a profile (any
field beyond name and lead) creates its Bot, or updates the Bot of that name; a role with
only name and lead names an existing Bot.
"""

import json
import sys
import urllib.error
import urllib.request

_PROFILE = ("description", "skills", "backend", "model", "provider", "face", "colour", "tools", "connections", "write", "permissionMode")

spec_path, addr, log, roster = sys.argv[1:5]


def call(method, path, body=None):
    data = None if body is None else json.dumps(body).encode()
    req = urllib.request.Request(
        f"http://{addr}{path}", data=data, method=method, headers={"Content-Type": "application/json"}
    )
    try:
        with urllib.request.urlopen(req, timeout=10) as r:
            status, text = r.status, r.read().decode()
    except urllib.error.HTTPError as e:
        status, text = e.code, e.read().decode()
    with open(log, "a") as f:
        f.write(f"> {method} {path}{'' if body is None else ' ' + json.dumps(body)}\n< {status} {text}\n")
    if status >= 300:
        sys.exit(f"team-apply: {method} {path}: {status} {text}")
    return json.loads(text) if text.strip() else None


spec = json.load(open(spec_path))
bots = {b["name"]: b for b in call("GET", "/bots")}
roles = []
for r in spec["roles"]:
    profile = {k: r[k] for k in _PROFILE if k in r}
    if profile:
        body = {"name": r["name"], **profile}
        if r["name"] in bots:
            bots[r["name"]] = call("PUT", f"/bots/{bots[r['name']]['id']}", body)
        else:
            bots[r["name"]] = call("POST", "/bots", body)
    elif r["name"] not in bots:
        sys.exit(f"team-apply: no Bot named {r['name']}")
    roles.append({"botId": bots[r["name"]]["id"], "lead": bool(r.get("lead"))})
json.dump({"name": spec["name"], "responsibility": spec.get("responsibility", ""), "roles": roles}, open(roster, "w"))

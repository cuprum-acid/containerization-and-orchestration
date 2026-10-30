#!/bin/sh
# n8n -> repo. The raw export carries instance metadata: the owner project
# (with the account's name and email), timestamps and version counters that
# change on every save. Only the workflow itself is kept, so the diff shows
# real changes and nothing personal is committed.
set -eu
cd "$(dirname "$0")"
docker compose exec -T n8n n8n export:workflow --backup --output=/workflows/ >/dev/null
for f in workflows/*.json; do
  python3 - "$f" <<'PY'
import json, sys
path = sys.argv[1]
wf = json.load(open(path))
keep = ("id", "name", "active", "nodes", "connections", "settings", "pinData", "tags")
wf = {k: wf[k] for k in keep if k in wf}
wf["active"] = False
open(path, "w").write(json.dumps(wf, indent=2, ensure_ascii=False) + "\n")
PY
done
echo "exported: $(ls workflows)"

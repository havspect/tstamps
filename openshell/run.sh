#!/usr/bin/env bash
# Create the agents sandbox (opencode + agy) and forward its dev ports to host
# loopback. Ports match the host: 4096 opencode serve, 5173 web, 5174 docs,
# 8080 API, 8098/8099 e2e (web/e2e/global-setup.ts).
set -euo pipefail

name=${SANDBOX_NAME:-tstamps-agents}
image=${SANDBOX_IMAGE:-tstamps-agents:latest}
repo=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
ports=(4096 5173 5174 8080 8098 8099)

if openshell sandbox get "$name" >/dev/null 2>&1; then
  echo "sandbox $name already exists: openshell sandbox delete $name, then rerun" >&2
  exit 1
fi

docker image inspect "$image" >/dev/null 2>&1 || docker build -t "$image" "$repo/openshell"

# The canonical main process has to outlive the terminal, so it idles; get a
# shell with `openshell sandbox exec --name $name --tty -- bash -l`.
openshell sandbox create --name "$name" --from "$image" \
  --policy "$repo/openshell/policy.yaml" --detach -- sleep infinity

for port in "${ports[@]}"; do
  # -d leaves the daemon holding the inherited pipes, so redirect or this hangs.
  openshell forward start "127.0.0.1:$port" "$name" -d >/dev/null 2>&1 ||
    echo "port $port: forward failed (already bound on the host?)" >&2
done

openshell forward list
echo "shell: openshell sandbox exec --name $name --tty -- bash -l"

#!/usr/bin/env bash

set -euo pipefail
cd "$(dirname "$0")"

NAME=${NAME:-mydocker}
MEM=${MEM:-100M}
CPU=${CPU:-50000 100000}
PIDS=${PIDS:-64}
[ $# -gt 0 ] || set -- ./bin/api

CG=/sys/fs/cgroup/$NAME-$$
sudo mkdir "$CG"
trap 'sudo rmdir "$CG" 2>/dev/null || true' EXIT
echo "$MEM"  | sudo tee "$CG/memory.max"      >/dev/null
echo 0       | sudo tee "$CG/memory.swap.max" >/dev/null
echo "$CPU"  | sudo tee "$CG/cpu.max"         >/dev/null
echo "$PIDS" | sudo tee "$CG/pids.max"        >/dev/null
echo "mydocker: cgroup $CG memory.max=$MEM cpu.max=\"$CPU\" pids.max=$PIDS" >&2


(
  me=$BASHPID
  echo "$me" | sudo tee "$CG/cgroup.procs" >/dev/null
  [ "$(cat /proc/self/cgroup)" = "0::/${CG#/sys/fs/cgroup/}" ] ||
    { echo "mydocker: not in $CG: $(cat /proc/self/cgroup)" >&2; exit 1; }

  exec unshare --user --map-root-user --pid --fork --mount-proc \
               --net --uts --ipc --cgroup --kill-child \
    sh -c '
      # Setup that still needs privileges inside the namespaces.
      hostname "$0"
      ip link set lo up
      # Then take them away, right before exec - the order runtimes use:
      # empty bounding set -> no capabilities after exec; no_new_privs ->
      # setuid bits are ignored; seccomp last, and it is inherited by the command.
      exec setpriv --bounding-set -all --no-new-privs \
           ./seccomp/seccomp-exec.py seccomp/profile.json "$@"
    ' "$NAME" "$@"
)

#!/usr/bin/env sh
set -eu
workdir=${RUN_DIR:-$(pwd)}
if [ "$(cat "$workdir/state.txt")" != "step1" ]; then
  echo "missing initial state" >&2
  exit 1
fi
echo step2 >> "$workdir/state.txt"
cat "$workdir/state.txt"

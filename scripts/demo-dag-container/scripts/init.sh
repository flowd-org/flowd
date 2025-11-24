#!/usr/bin/env sh
set -eu
workdir=${RUN_DIR:-$(pwd)}
echo step1 > "$workdir/state.txt"

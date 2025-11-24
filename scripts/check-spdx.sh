#!/usr/bin/env bash
set -euo pipefail

LICENSE_RE='SPDX-License-Identifier: AGPL-3.0-or-later '

EXCLUDES='^(docs/agent-context/|\.specify/memory/|vendor/|third_party/|.git/|\.github/cla\.json$)'
BINARY_EXT='(png|jpg|jpeg|gif|svg|ico|pdf|zip|tar|gz|xz|bz2)$'
GENERATED_RE='(\._generated\.go$|\.pb\.go$|/gen/|/generated/|^docs/agent-context/SOT/openapi_.*\.ya?ml$)'

fail=0

is_text() { file -b --mime-type "$1" | grep -q '^text/'; }

while IFS= read -r f; do
  if [[ $f =~ $EXCLUDES ]]; then continue; fi
  if [[ $f =~ \.($BINARY_EXT)$ ]]; then continue; fi
  if [[ $f =~ $GENERATED_RE ]]; then continue; fi
  if [[ $f == *.json ]]; then continue; fi
  if ! is_text "$f"; then continue; fi

  if ! head -n 5 "$f" | grep -q "$LICENSE_RE"; then
    echo "::error file=$f,line=1::Missing SPDX license header: '$LICENSE_RE'"
    echo "MISSING: $f"
    fail=1
  fi
done < <(git ls-files)

exit $fail

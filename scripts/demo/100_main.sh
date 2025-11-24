#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-or-later 
set -Eeuo pipefail
IFS=$'\n\t'

if [ -z "${ARG_NAME:-}" ]; then
    echo "demo: ARG_NAME missing" >&2
    exit 1
fi

greeting="hello ${ARG_NAME}"
if [ "${ARG_LOUD:-false}" = "true" ]; then
    greeting=$(printf '%s' "$greeting" | tr '[:lower:]' '[:upper:]')
fi

echo "demo: $greeting"

if [ "${FLWD_ARGS_JSON:-}" != "" ]; then
    echo "demo: arguments captured (see events/artifacts)"
fi

echo "demo: completed"

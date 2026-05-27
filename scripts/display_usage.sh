#!/bin/bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
BINARY_PATH="$PROJECT_ROOT/codesome"

cd "$PROJECT_ROOT"

if [[ -x "$BINARY_PATH" ]]; then
    "$BINARY_PATH" "$@"
else
    go run . "$@"
fi

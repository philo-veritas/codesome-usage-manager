#!/bin/bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

LOCAL_DB="${LOCAL_DB:-$PROJECT_ROOT/codesome-manager.db}"
REMOTE_TARGET="${REMOTE_TARGET:-hl-mbp:/Users/hl/Projects/codesome-usage-manager/codesome-manager.db}"

if [[ ! -f "$LOCAL_DB" ]]; then
    echo "Local database not found: $LOCAL_DB" >&2
    exit 1
fi

echo "Copying local database:"
echo "  from: $LOCAL_DB"
echo "  to:   $REMOTE_TARGET"

scp -p "$LOCAL_DB" "$REMOTE_TARGET"

#!/bin/bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

REMOTE_HOST="${REMOTE_HOST:-hl-mbp}"
REMOTE_DB="${REMOTE_DB:-/Users/hl/Projects/codesome-usage-manager/codesome-manager.db}"
LOCAL_DB="${LOCAL_DB:-$PROJECT_ROOT/codesome-manager.db}"
COMPARE_DIR="${COMPARE_DIR:-$PROJECT_ROOT/.db-compare}"

usage() {
    cat <<EOF
Usage: $(basename "$0") [--remote-host HOST] [--remote-db PATH] [--local-db PATH]

Defaults:
  --remote-host $REMOTE_HOST
  --remote-db   $REMOTE_DB
  --local-db    $LOCAL_DB

The comparison exports table content while excluding columns named updated_at
or last_synced_at.
Environment variables with the same uppercase names are also supported.
EOF
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --remote-host)
            REMOTE_HOST="$2"
            shift 2
            ;;
        --remote-db)
            REMOTE_DB="$2"
            shift 2
            ;;
        --local-db)
            LOCAL_DB="$2"
            shift 2
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            echo "Unknown argument: $1" >&2
            usage >&2
            exit 2
            ;;
    esac
done

require_command() {
    if ! command -v "$1" >/dev/null 2>&1; then
        echo "Missing required command: $1" >&2
        exit 1
    fi
}

shell_quote() {
    printf "'%s'" "${1//\'/\'\\\'\'}"
}

sql_string() {
    printf "'%s'" "${1//\'/\'\'}"
}

sql_identifier() {
    printf '"%s"' "${1//\"/\"\"}"
}

append_csv_value() {
    local current="$1"
    local value="$2"

    if [[ -n "$current" ]]; then
        printf '%s, %s' "$current" "$value"
    else
        printf '%s' "$value"
    fi
}

append_concat_expr() {
    local current="$1"
    local value="$2"

    if [[ -n "$current" ]]; then
        printf '%s || char(9) || %s' "$current" "$value"
    else
        printf '%s' "$value"
    fi
}

export_comparable_db() {
    local database="$1"
    local output="$2"

    : >"$output"
    while IFS= read -r table; do
        local table_identifier
        local table_literal
        local columns=""
        local select_expr=""
        local order_expr=""

        table_identifier="$(sql_identifier "$table")"
        table_literal="$(sql_string "$table")"

        while IFS= read -r column; do
            local column_identifier
            column_identifier="$(sql_identifier "$column")"
            columns="$(append_csv_value "$columns" "$column")"
            select_expr="$(append_concat_expr "$select_expr" "quote($column_identifier)")"
            order_expr="$(append_csv_value "$order_expr" "$column_identifier")"
        done < <(sqlite3 "$database" "SELECT name FROM pragma_table_info($table_literal) WHERE name NOT IN ('updated_at', 'last_synced_at') ORDER BY cid")

        {
            echo "TABLE $table"
            echo "COLUMNS $columns"
        } >>"$output"

        if [[ -n "$select_expr" ]]; then
            sqlite3 "$database" "SELECT $select_expr FROM $table_identifier ORDER BY $order_expr" >>"$output"
        fi
    done < <(sqlite3 "$database" "SELECT name FROM sqlite_schema WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name")
}

require_command scp
require_command ssh
require_command sqlite3
require_command diff

if [[ ! -f "$LOCAL_DB" ]]; then
    echo "Local database not found: $LOCAL_DB" >&2
    exit 1
fi

mkdir -p "$COMPARE_DIR"

timestamp="$(date +%Y%m%d%H%M%S)"
remote_snapshot="/tmp/codesome-manager.${timestamp}.$$.db"
remote_copy="$COMPARE_DIR/hl-mbp-codesome-manager.$timestamp.db"
local_dump="$COMPARE_DIR/local.$timestamp.sql"
remote_dump="$COMPARE_DIR/hl-mbp.$timestamp.sql"
diff_file="$COMPARE_DIR/db-diff.$timestamp.diff"

cleanup_remote() {
    ssh "$REMOTE_HOST" "rm -f $(shell_quote "$remote_snapshot")" >/dev/null 2>&1 || true
}
trap cleanup_remote EXIT

echo "Creating remote SQLite snapshot on $REMOTE_HOST..."
ssh "$REMOTE_HOST" "sqlite3 $(shell_quote "$REMOTE_DB") \".backup $(shell_quote "$remote_snapshot")\""

echo "Copying remote snapshot to $remote_copy..."
scp -p "$REMOTE_HOST:$remote_snapshot" "$remote_copy"

echo "Exporting local database content without updated_at or last_synced_at..."
export_comparable_db "$LOCAL_DB" "$local_dump"

echo "Exporting remote database content without updated_at or last_synced_at..."
export_comparable_db "$remote_copy" "$remote_dump"

echo "Comparing dumps..."
if diff -u "$local_dump" "$remote_dump" >"$diff_file"; then
    echo "Databases match."
    echo "Remote copy: $remote_copy"
    echo "Dumps: $local_dump $remote_dump"
    echo "Diff file: $diff_file"
    exit 0
fi

echo "Databases differ."
echo "Remote copy: $remote_copy"
echo "Dumps: $local_dump $remote_dump"
echo "Diff file: $diff_file"
exit 1

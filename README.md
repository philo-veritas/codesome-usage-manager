# Codesome Usage Manager

Go CLI and HTTP API for SQLite-backed Codesome API Key management, usage sync, and reporting.

## Quick Start

Prerequisites:

- Go 1.25 or newer.
- A Codesome account that can log in to `https://v3.codesome.cn`.
- Docker Compose, only if you want to run the HTTP API in Docker.

Create a local config and initialize the database:

```bash
cp config.yaml.example config.yaml
$EDITOR config.yaml
go build -o codesome .
./codesome db init
```

Import existing Codesome API keys from the remote API into SQLite:

```bash
./codesome db import-remote-keys --dry-run
./codesome db import-remote-keys
```

Common first commands:

```bash
./codesome team add --code platform --name "Platform"
./codesome user import --file users.csv --dry-run
./codesome user import --file users.csv
./codesome user import-feishu --dry-run
./codesome sync users --dry-run
./codesome feishu send-keys --team platform --dry-run
./codesome sync usage --yesterday
./codesome report monthly --month 2026-05
```

## Build

```bash
make build-dev
make test
make check
```

Without Make:

```bash
go build -o codesome .
go test ./...
go build ./...
```

## Command Overview

| Area | Commands |
| --- | --- |
| Database | `db init`, `db migrate`, `db import-remote-keys`, `db import-config-keys` |
| Teams | `team add`, `team update`, `team list` |
| Users | `user add`, `user import`, `user import-feishu`, `user update`, `user delete`, `user list` |
| API keys | `sync users`, `key export`, `create-key`, `update-key` |
| Feishu | `feishu bitable explore`, `feishu send-keys` |
| Usage | `usage today`, `sync usage`, `usage-stats`, `daily-usage`, default `codesome` usage view |
| Reports | `report monthly` |
| Service | `serve`, `switch-on-exhausted`, `auto-switch` |

## SQLite Management

The SQLite-backed Codesome API Key management flow has been migrated into this repository. It uses `codesome-manager.db` as the source of truth for local teams, users, API keys, usage history, and monthly reports.

Initialize or migrate the local SQLite database:

```bash
codesome db init
codesome db migrate
codesome db init --path /tmp/codesome-manager-test.db
codesome db import-remote-keys --dry-run
codesome db import-remote-keys
codesome db import-config-keys --dry-run
codesome db import-config-keys --group-id 51
```

`db import-remote-keys` is the preferred bootstrap path. It reads the current Codesome API Key list through the Codesome API and creates virtual local users such as `codesome-key:6732` for keys that are not yet assigned to a real employee. You can later update those users and teams manually.

Manage local teams:

```bash
codesome team add --code platform --name "Platform"
codesome team update --code platform --status inactive
codesome team list
```

Manage local users:

```bash
codesome user add --employee-no E12345 --name "Alice" --team platform --group-id 51
codesome user add --employee-no E12346 --name "Bob" --feishu-open-id ou_xxx
codesome user import --file users.csv --dry-run
codesome user import --file users.csv
codesome user update --employee-no E12345 --status inactive
codesome user update --employee-no E12345 --feishu-open-id ou_xxx
codesome user update --employee-no E12345 --team infra
codesome user update --employee-no E12345 --clear-group-id
codesome user delete --employee-no E12345
codesome user list
```

CSV import expects `employee_no` and `name`; `team`, `group_id`, `status`, and `feishu_open_id` are optional. When `team` is set, it must match an existing team code. Save Excel sheets as CSV before importing:

```csv
employee_no,name,team,group_id,status,feishu_open_id
E12345,Alice,platform,51,active,ou_alice
E12346,Bob,infra,60,active,ou_bob
E12347,Carol,platform,,inactive,
```

Import users from a Feishu Bitable:

```bash
codesome feishu bitable explore
codesome user import-feishu --dry-run
codesome user import-feishu
```

Feishu Bitable user import uses a fixed table shape in code, so `config.yaml` only needs the Feishu app and Bitable location:

```yaml
feishu:
  app_id: "cli_xxx"
  app_secret: "your-feishu-app-secret"
  bitable:
    app_token: "bascn_xxx"
    users:
      table_id: "tblxxx"
```

The importer expects these Bitable fields: `人员`, `工号`, `团队`, and `状态`. `name` comes from `人员[0].name`, `open_id` comes from `人员[0].id`, `employee_no` comes from `工号.value[0].text`, `team` comes from `团队[0].text`, and `status` maps `生效` to `active` and `禁用` to `inactive`. The Bitable API is queried with `user_id_type=open_id`.

When a Feishu team does not exist locally, `user import-feishu` creates it automatically with `teams.code` and `teams.name` both set to the Feishu team text, for example `数字化中心`. Dry-run uses the same validation path but rolls back those temporary team creations.

Sync local users to Codesome API keys:

```bash
codesome sync users --dry-run
codesome sync users
codesome sync users --employee-no E12345
```

The default `sync users` mode is incremental. Use it for routine syncs; it creates missing active-user keys and updates keys only when local state changed or fields differ. `codesome sync users --full` intentionally calls the remote update API for every matched existing key, so reserve it for repairing remote drift after manual Codesome changes.

When `sync users` creates or updates a Codesome key for a user without a manual group override, it selects the active subscription group with the most remaining daily balance, matching the auto-switch selection rule. `codesome.default_group_id` remains a fallback when that live selection is unavailable.

Send locally stored raw API keys to Feishu users:

```bash
codesome feishu send-keys --employee-no E12345 --dry-run
codesome feishu send-keys --team platform
codesome feishu send-keys --all
```

This sends only API keys whose `raw_key` is still stored locally. Keys imported from remote lists usually do not have `raw_key`, because Codesome only returns the secret when a key is created.

Export local API keys:

```bash
codesome key export --employee-no E12345
codesome key export --team platform --output keys-platform.csv
codesome key export --all --output keys.csv
codesome key export --all --include-inactive --output keys-all.csv
```

Query today's usage for active API keys from the local database:

```bash
codesome usage today
codesome usage today --sort-by-today-cost
codesome usage today --include-inactive
```

Rows marked `remote_missing` exist in local SQLite but were not returned by the Codesome usage API, usually because the remote API key was deleted.

Sync local usage history:

```bash
codesome sync usage --date 2026-05-26
codesome sync usage --from 2026-05-01 --to 2026-05-26
codesome sync usage --yesterday
codesome sync usage --date 2026-05-28 --include-today
```

When `feishu.bitable.usage.table_id` is configured, `sync usage` also upserts the synced rows to the Feishu Bitable usage table. The usage table must contain these fields: `ID`, `日期`, `人员`, `总Tokens`, and `实际成本USD`. `ID` uses `YYYY-MM-DD#codesome_key_id`; the local `feishu_usage_records` table caches the matching Bitable `record_id` for faster updates.

Feishu usage sync only writes rows that have a local `feishu_open_id` and `total_tokens > 0`; rows without a matched Feishu person or without token usage are skipped.

For already stored historical dates, `sync usage` reuses local `usage_daily` rows and skips Codesome usage requests. Pass `--force-update` to refresh stored dates. Today is skipped by default; when explicitly included with `--include-today`, it is always refreshed because same-day usage can still change.

Generate monthly reports:

```bash
codesome report monthly --month 2026-05
codesome report monthly --month 2026-05 --team platform
codesome report monthly --month 2026-05 --output report-2026-05.csv
```

For the design, see [`docs/codesome-api-key-management-design.md`](docs/codesome-api-key-management-design.md).

## Legacy Usage Helper

The default command and alias-based key commands are retained as a legacy/simple usage helper. They can still read `api_key_ids` aliases from `config.yaml`; new management workflows should use SQLite commands above.

Run from the repository root:

```bash
go run .
go run . --force-update
go run . --debug
```

Query one API key's daily usage:

```bash
go run . daily-usage --key main
go run . daily-usage --key-id 6732
```

Query one API key's aggregate usage stats for an inclusive date range:

```bash
go run . usage-stats --key main --start-date 2026-05-26 --end-date 2026-05-26
go run . usage-stats --key-id 6732 --start-date 2026-05-26 --end-date 2026-05-27
```

Create or update a Codesome API key:

```bash
go run . create-key --name test --group-id 51
go run . update-key --key-id 9356 --status inactive
go run . update-key --key main --name main-2
go run . update-key --key-id 9356 --group-id 51
```

Reset one API key's quota:

```bash
go run . reset-quota --key main
go run . reset-quota --key-id 6732
```

Switch one API key to a target group:

```bash
go run . switch-group --key main --group-id 60
go run . switch-group --key-id 6732 --group-id 60
```

Switch one API key when its current subscription is below the remaining-budget threshold:

```bash
go run . switch-on-exhausted --key main
go run . switch-on-exhausted --key-id 6732
go run . switch-on-exhausted --key-id 6732 --min-remaining 10
```

Batch mode reads active API keys from the local SQLite database:

```bash
go run . switch-on-exhausted --all --min-remaining 10
go run . switch-on-exhausted --all --path /tmp/codesome-manager.db --min-remaining 10
```

## Auto Switch

Run the state-changing auto switcher for all Codesome API keys:

```bash
go run . auto-switch --min-remaining 10 --min-interval 2m --max-interval 2h
```

When all active subscription groups are exhausted, `switch-on-exhausted` and `auto-switch`
can fall back to the configured pay-as-you-go group. The fallback uses
`codesome.pay_as_you_go_group_id` (default `3`) and only runs when the active subscription
daily limit is at least `max(pay_as_you_go_min_subscription_daily_limit_usd, recent usage P80)`.
The default minimum is `$60/day`; recent usage P80 is calculated from local `usage_daily` over
`pay_as_you_go_history_days` (default `21`). This keeps pay-as-you-go for spikes instead of
masking a subscription capacity shortfall.

## HTTP API

Start the server:

```bash
go run . serve --port 8080
```

By default, the server binds to `127.0.0.1`. Use `--host` only when you intentionally want to expose it elsewhere.

Endpoints:

```bash
GET  /api/codesome/usage
GET  /api/codesome/usage?force_update=true
GET  /api/codesome/usage-stats?key=main&start_date=2026-05-26&end_date=2026-05-26
GET  /api/codesome/usage-stats?key_id=6732&start_date=2026-05-26&end_date=2026-05-27&force_update=true
POST /api/codesome/keys              {"name":"test","group_id":51}
PUT  /api/codesome/keys?key=main     {"status":"inactive"}
PUT  /api/codesome/keys?key_id=9356  {"name":"main-2","group_id":51}
GET  /api/codesome/daily-usage?key=main
GET  /api/codesome/daily-usage?key_id=6732
POST /api/codesome/reset-quota?key=main
POST /api/codesome/reset-quota?key_id=6732
POST /api/codesome/switch-group?key=main&group_id=60
POST /api/codesome/switch-group?key_id=6732&group_id=60
POST /api/codesome/switch-on-exhausted?key=main
POST /api/codesome/switch-on-exhausted?key_id=6732
```

## Docker

Run the API with Docker Compose:

```bash
cp config.yaml.example config.yaml
$EDITOR config.yaml
make compose-up
```

The compose files persist auth/cache files and `codesome-manager.db` on the host. The state-changing auto switcher is opt-in and operates on all Codesome API keys:

```bash
make compose-up-auto
make compose-logs-auto
```

If you call `docker compose` directly, create the bind-mounted state files first:

```bash
make ensure-state-files
docker compose up -d --build
```

The base compose file publishes the API on host port `8055`. The container command binds the app to `0.0.0.0` so Docker port publishing and nginx proxying work; the local `serve` command still defaults to `127.0.0.1`. Use `docker-compose.nginx.yml` only when you intentionally want an nginx front end:

```bash
make compose-up-nginx
```

## Config

`config.yaml` stores Codesome connection settings and the local SQLite database path:

```yaml
codesome:
  base_url: "https://v3.codesome.cn"
  login:
    email: "your-email@example.com"
    password: "your-password"
  default_group_id: 51
  pay_as_you_go_group_id: 3
  pay_as_you_go_min_subscription_daily_limit_usd: 60
  pay_as_you_go_history_days: 21

feishu:
  app_id: "cli_xxx"
  app_secret: "your-feishu-app-secret"
  bitable:
    app_token: "bascn_xxx"
    users:
      table_id: "tblxxx"

database:
  path: "./codesome-manager.db"
```

Runtime state is stored in `.codesome_auth.json` and `.usage_cache.json`. Do not commit those files.

`api_key_ids` is a legacy static key list. New SQLite-backed commands use the local database as their key source, and new setups should prefer `codesome db import-remote-keys`. Keep `api_key_ids` only when you still need legacy alias commands, or when the remote API is unavailable and you must import old config keys into SQLite:

```yaml
codesome:
  api_key_ids:
    - id: 6732
      name: "architecture-extra"
      key: "main"
```

## Test

```bash
make test
make check
```

`make check` runs both `go test ./...` and `go build ./...`.

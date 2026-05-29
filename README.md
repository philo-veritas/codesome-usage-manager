# Codesome Usage Manager

Go CLI and HTTP API for SQLite-backed Codesome API Key management, usage sync, and reporting.

## Build

```bash
go mod tidy
go build -o codesome .
```

## SQLite Management

The SQLite-backed Codesome API Key management flow has been migrated into this repository. It uses `codesome-manager.db` as the source of truth for local teams, users, API keys, usage history, and monthly reports.

Initialize or migrate the local SQLite database:

```bash
codesome db init
codesome db migrate
codesome db init --path /tmp/codesome-manager-test.db
codesome db import-config-keys --dry-run
codesome db import-config-keys --group-id 51
```

Manage local teams:

```bash
codesome team add --code platform --name "Platform"
codesome team update --code platform --status inactive
codesome team list
```

Manage local users:

```bash
codesome user add --employee-no E12345 --name "Alice" --team platform --group-id 51
codesome user update --employee-no E12345 --status inactive
codesome user update --employee-no E12345 --team infra
codesome user update --employee-no E12345 --clear-group-id
codesome user delete --employee-no E12345
codesome user list
```

Sync local users to Codesome API keys:

```bash
codesome sync users --dry-run
codesome sync users
codesome sync users --employee-no E12345
```

Export local API keys:

```bash
codesome key export --employee-no E12345
codesome key export --team platform --output keys-platform.csv
codesome key export --all --output keys.csv
codesome key export --all --include-inactive --output keys-all.csv
```

Sync local usage history:

```bash
codesome sync usage --date 2026-05-26
codesome sync usage --from 2026-05-01 --to 2026-05-26
codesome sync usage --yesterday
codesome sync usage --date 2026-05-28 --include-today
```

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

## Auto Switch

Run the state-changing auto switcher for all Codesome API keys:

```bash
go run . auto-switch --min-remaining 10 --min-interval 2m --max-interval 2h
```

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
docker compose up -d --build
```

The compose files persist auth/cache files and `codesome-manager.db` on the host. The state-changing auto switcher is opt-in and operates on all Codesome API keys:

```bash
docker compose --profile auto-switch up -d --build
docker compose logs -f usage-auto-switch
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

database:
  path: "./codesome-manager.db"
```

Runtime state is stored in `.codesome_auth.json` and `.usage_cache.json`. Do not commit those files.

`api_key_ids` is a legacy static key list. New SQLite-backed commands use the local database as their key source. Keep `api_key_ids` only when you still need legacy alias commands, or when importing old config keys into SQLite:

```yaml
codesome:
  api_key_ids:
    - id: 6732
      name: "architecture-extra"
      key: "main"
```

## Test

```bash
go test ./...
```

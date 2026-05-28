# Codesome Usage Manager

Go CLI and HTTP API for Codesome usage lookup, API Key management, quota reset, group switching, and auto switching.

## Build

```bash
go mod tidy
go build -o codesome .
```

## Usage

Run from the repository root:

```bash
go run .
go run . --force-update
go run . --debug
```

The CLI loads `config.yaml` from the current directory or the parent project directory.

## Codesome Commands

For the planned SQLite-backed Codesome API Key management flow, see
[`docs/codesome-api-key-management-design.md`](docs/codesome-api-key-management-design.md).

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

Switch automatically when the current subscription daily remaining quota is below a threshold. The target is the active subscription group with the most remaining daily quota. The default threshold is `0`, which preserves the old exhausted-only behavior:

```bash
go run . switch-on-exhausted --key main
go run . switch-on-exhausted --key-id 6732
go run . switch-on-exhausted --key main --min-remaining 10
go run . switch-on-exhausted --all --min-remaining 10
```

Run a long-lived auto switcher for all configured Codesome API keys. It aligns all keys to the active group with the most remaining daily quota on startup and at the start of each Beijing day, then checks dynamically based on remaining quota:

```bash
go run . auto-switch --all --min-remaining 10 --min-interval 2m --max-interval 2h
```

Run the API on a server with Docker Compose:

```bash
docker compose up -d --build
```

Run the API and opt in to the state-changing auto switcher:

```bash
docker compose --profile auto-switch up -d --build
docker compose logs -f usage-auto-switch
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
POST /api/codesome/reset-all-quotas
POST /api/codesome/switch-group?key=main&group_id=60
POST /api/codesome/switch-group?key_id=6732&group_id=60
POST /api/codesome/switch-on-exhausted?key=main
POST /api/codesome/switch-on-exhausted?key_id=6732
```

## Config

Codesome keys can be given aliases in `config.yaml`:

```yaml
codesome:
  base_url: "https://v3.codesome.cn"
  login:
    email: "your-email@example.com"
    password: "your-password"
  default_group_id: 51
  api_key_ids:
    - id: 6732
      name: "architecture-extra"
      key: "main"

database:
  path: "./codesome-manager.db"
```

Runtime state is stored in `.codesome_auth.json` and `.usage_cache.json`. Do not commit those files.

Initialize or migrate the local SQLite database:

```bash
codesome db init
codesome db migrate
codesome db init --path /tmp/codesome-manager-test.db
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

## Test

```bash
go test ./...
```

# Codesome 功能拆分迁移计划

## 目标

将当前仓库中的 Codesome 相关能力拆分到独立仓库，避免 `claude_code_usage_helper` 同时承载 usage helper、statusline、Claude Buddy 查询和 Codesome 管理系统。

新仓库建议命名：

```text
codesome-usage-manager
```

定位：

- Codesome API Key 管理。
- Codesome 用量查询和历史采集。
- Codesome quota reset。
- Codesome group 手动/自动切换。
- Codesome HTTP API 服务。
- 后续 SQLite-backed user/key/usage 管理。

## 拆分原则

1. 只迁移 Codesome 相关功能。
2. 不迁移 Claude Buddy、88Code、Python TUI、statusline、safe_rm 等当前仓库的非 Codesome 能力。
3. 新仓库以 Go 实现为主，避免继续保留 Python/Go 双实现。
4. 新仓库配置语义重新收敛，不沿用当前 `config.yaml` 中多 provider 的历史结构。
5. 不迁移真实凭据文件，只迁移文件格式和路径逻辑。
6. 保留当前仓库中的轻量说明，指向新仓库。

## 迁移时机

迁移不等待 SQLite 新功能完成。

推荐顺序是：

1. 先把当前已有 Codesome 能力迁到新仓库。
2. 在新仓库完成 Codesome-only 收敛，去掉 Claude Buddy、88Code、多 provider 配置和 `/api/cost`。
3. 在新仓库内继续开发 SQLite-backed 管理功能。

这样可以避免先在旧仓库实现新功能，再迁移时二次重构配置、命令名和目录结构。

当前仓库已经具备足够的迁移起点：

- Codesome auth 和 token refresh。
- Codesome usage、daily usage、usage stats。
- key create/update。
- quota reset。
- switch group、switch-on-exhausted、auto-switch。
- Codesome HTTP API。
- `scripts/display_usage_go.sh`。

## 迁移范围

### 必须迁移

Go 代码：

```text
go/main.go
go/go.mod
go/go.sum
go/Makefile
go/Dockerfile

go/cmd/daily_usage.go
go/cmd/key.go
go/cmd/usage_stats.go
go/cmd/reset.go
go/cmd/switch_group.go
go/cmd/auto_switch.go
go/cmd/serve.go

go/internal/auth/codesome.go
go/internal/provider/codesome.go
go/internal/cache/cache.go
go/internal/config/config.go
go/internal/server/codesome_handler.go

go/internal/provider/codesome_switch_test.go
go/internal/server/codesome_handler_test.go
go/cmd/switch_group_test.go
go/cmd/auto_switch_test.go
```

脚本和部署：

```text
scripts/display_usage_go.sh
go/docker-compose.yml
go/docker-compose.nginx.yml
go/nginx/default.conf
```

文档：

```text
go/README.md 中 Codesome 相关内容
docs/codesome-api-key-management-design.md
docs/codesome-repo-migration-plan.md
curl_examples.md 中 Codesome API 示例
config.yaml.example 中 Codesome 配置示例
```

### 部分迁移或重写

当前这些文件混有非 Codesome 逻辑，迁移时应重写或裁剪：

```text
go/cmd/root.go
go/cmd/sync.go
go/internal/config/config.go
go/internal/server/handler.go
go/internal/provider/claude_buddy.go
```

处理方式：

- `root.go`: 改为 Codesome-only CLI root。
- `sync.go`: 当前是 Claude Buddy 缓存同步，应不迁；新仓库后续实现 `sync users` / `sync usage`。
- `config.go`: 去掉多 provider 结构，改成 Codesome-only 配置。
- `handler.go`: 当前 `/api/cost` 依赖 Claude Buddy auth token，不迁。
- `claude_buddy.go`: 不迁。

### 不迁移

```text
main.py
monitor.py
config.py
src/providers/claude_buddy.py
src/providers/eightcode.py
src/providers/codesome.py
src/auth/codesome.py
src/models.py
statusline/
hooks/
examples/
monitor/
scripts/display_usage.sh
scripts/hl-cc-usage-monitor.sh
scripts/test_safe_rm.sh
README.md 中非 Codesome 内容
CLAUDE.md 中非 Codesome 内容
AGENTS.md 中非 Codesome 内容
uv.lock
pyproject.toml
```

## 新仓库建议结构

```text
codesome-usage-manager/
  README.md
  AGENTS.md
  go.mod
  go.sum
  Makefile
  Dockerfile
  docker-compose.yml
  config.yaml.example
  docs/
    api-key-management-design.md
    migration-from-claude-code-usage-helper.md
  scripts/
    display_usage.sh
  cmd/
    root.go
    usage.go
    usage_stats.go
    key.go
    reset.go
    switch_group.go
    auto_switch.go
    serve.go
  internal/
    auth/
      codesome.go
    cache/
      cache.go
    config/
      config.go
    provider/
      codesome.go
    server/
      codesome_handler.go
    db/
      db.go
      migrations.go
    sync/
      users.go
      usage.go
```

SQLite 管理功能可以在迁移完成后逐步加入：

```text
cmd/db.go
cmd/user.go
cmd/sync_users.go
cmd/sync_usage.go
internal/db/
internal/repository/
internal/sync/
```

## 推荐启动方式

不要从旧仓库直接移动文件。先复制到新仓库，保持旧仓库可用并便于对照。

示例：

```bash
mkdir -p /Users/philoveritas/Tools/codesome-usage-manager

rsync -a --exclude '.git' \
  /Users/philoveritas/Tools/claude_code_usage_helper/go/ \
  /Users/philoveritas/Tools/codesome-usage-manager/

mkdir -p /Users/philoveritas/Tools/codesome-usage-manager/docs

cp /Users/philoveritas/Tools/claude_code_usage_helper/docs/codesome-api-key-management-design.md \
   /Users/philoveritas/Tools/codesome-usage-manager/docs/api-key-management-design.md

cp /Users/philoveritas/Tools/claude_code_usage_helper/docs/codesome-repo-migration-plan.md \
   /Users/philoveritas/Tools/codesome-usage-manager/docs/migration-from-claude-code-usage-helper.md
```

初始化：

```bash
cd /Users/philoveritas/Tools/codesome-usage-manager
git init
go test ./...
go build ./...
```

首次复制后允许短暂保留旧结构，目标是先建立可编译基线，再逐步裁剪。

## 新配置格式

建议不要继续沿用当前多 provider 配置。新仓库使用 Codesome-only 配置：

```yaml
codesome:
  base_url: "https://v3.codesome.cn"
  login:
    email: "your-email@example.com"
    password: "your-password"
  default_group_id: 51

database:
  path: "./codesome-manager.db"

server:
  host: "127.0.0.1"
  port: 8080
```

旧 `providers` 配置格式已移除，不再作为运行时兼容格式。旧配置需要迁移为 top-level `codesome`：

```yaml
codesome:
  base_url: "https://v3.codesome.cn"
  login:
    email: "your-email@example.com"
    password: "your-password"
  default_group_id: 51
  api_key_ids:
    - id: 6732
      name: "team-a"
      key: "main"
```

最终策略：

- `providers` 已移除，只支持 top-level `codesome`。
- `login_credentials` 已迁移为 `codesome.login`。
- `api_key_ids` 迁移到 SQLite。
- `api_key_ids` 只作为 legacy 静态 key 清单，用于旧 alias 命令或 `db import-config-keys`。

## 命令设计

### 迁移后保留的现有能力

```bash
codesome usage
codesome usage --force-update
codesome daily-usage --key-id 6732
codesome usage-stats --key-id 6732 --start-date 2026-05-26 --end-date 2026-05-26

codesome create-key --name alice --group-id 51
codesome update-key --key-id 9356 --status inactive
codesome reset-quota --key-id 6732

codesome switch-group --key-id 6732 --group-id 60
codesome switch-on-exhausted --all --min-remaining 10
codesome auto-switch --min-remaining 10 --min-interval 2m --max-interval 2h

codesome serve --host 127.0.0.1 --port 8080
```

### 后续 SQLite 管理能力

```bash
codesome db init
codesome db migrate
codesome db import-config-keys

codesome team add --code platform --name "Platform"
codesome team update --code platform --status inactive
codesome team list

codesome user add --employee-no E12345 --name "Alice" --team platform --group-id 51
codesome user update --employee-no E12345 --status inactive
codesome user update --employee-no E12345 --team infra
codesome user delete --employee-no E12345
codesome user list

codesome key export --employee-no E12345
codesome key export --team platform --output keys-platform.csv
codesome key export --all --output keys.csv

codesome sync users
codesome sync users --dry-run
codesome sync usage --yesterday
codesome sync usage --from 2026-05-01 --to 2026-05-26

codesome report monthly --month 2026-05
codesome report monthly --month 2026-05 --team platform
codesome report monthly --month 2026-05 --output report-2026-05.csv
```

## HTTP API 迁移

迁移后保留现有 Codesome HTTP API：

```text
GET  /api/codesome/usage
GET  /api/codesome/usage-stats
POST /api/codesome/keys
PUT  /api/codesome/keys
GET  /api/codesome/daily-usage
POST /api/codesome/reset-quota
POST /api/codesome/switch-group
POST /api/codesome/switch-on-exhausted
```

不迁移当前 `/api/cost`，因为它是 Claude Buddy token-based 查询接口。

后续 SQLite HTTP API 另行添加：

```text
GET    /api/codesome/teams
POST   /api/codesome/teams
PATCH  /api/codesome/teams/{code}
GET    /api/codesome/users
POST   /api/codesome/users
PATCH  /api/codesome/users/{employee_no}
DELETE /api/codesome/users/{employee_no}
POST   /api/codesome/sync/users
POST   /api/codesome/sync/usage
GET    /api/codesome/usage/daily
GET    /api/codesome/usage/user
GET    /api/codesome/usage/team
GET    /api/codesome/reports/monthly
```

安全要求：

- 默认监听 `127.0.0.1`。
- 若监听非 localhost，必须有认证或部署在可信内网。
- 创建 key 返回的 `sk-...` 不写日志。
- 不提交 `.codesome_auth.json`、`.usage_cache.json`、SQLite DB 和真实配置。

## 状态文件迁移

不迁移真实状态文件：

```text
.codesome_auth.json
.usage_cache.json
codesome-manager.db
config.yaml
```

新仓库应提供 `.gitignore`：

```gitignore
config.yaml
.codesome_auth.json
.usage_cache.json
*.db
*.db-shm
*.db-wal
```

缓存和 auth 路径建议：

- 默认在当前工作目录。
- 支持通过配置或环境变量覆盖。
- Docker 部署时挂载 volume。

## 分阶段迁移步骤

### 阶段 0：复制并建立基线

1. 按“推荐启动方式”复制当前 `go/` 和 Codesome 文档。
2. 在新仓库执行 `go test ./...` 和 `go build ./...`。
3. 提交一个初始迁移 commit，记录“原样复制 Go 版 Codesome 起点”。

验收：

- 新仓库能独立运行 `go test ./...`。
- 没有真实 `config.yaml`、`.codesome_auth.json`、`.usage_cache.json`、SQLite DB 被提交。
- `docs/` 中包含迁移计划和 API Key 管理设计。

### 阶段 1：创建新仓库骨架

1. 创建 `codesome-usage-manager` 仓库。
2. 添加 `.gitignore`、`README.md`、`config.yaml.example`。
3. 添加基础 CI 或 Makefile。
4. 保留当前 Go module，先不急于改 module path；完成裁剪后再统一重命名。

验收：

- `go test ./...` 可运行。
- README 明确项目定位。

### 阶段 2：迁移 Codesome provider 和 CLI

1. 保留 Codesome auth/provider/cache/config。
2. 保留 usage、daily-usage、usage-stats、key、reset、switch、auto-switch 命令。
3. 删除 Claude Buddy/88Code 分支。
4. 重写 `cmd/root.go` 为 Codesome-only root。
5. 重命名二进制和 root command 为 `codesome`。
6. 更新帮助文案，不再出现 Claude Buddy、88Code、Codex Buddy provider resolution。

验收：

- `codesome usage` 可查询 Codesome usage。
- `codesome create-key` / `update-key` 编译通过。
- 现有 Codesome 单元测试通过。
- `rg "Claude Buddy|claude-buddy|88Code|eightcode|ANTHROPIC_"` 不应命中新仓库业务代码。

### 阶段 3：迁移 HTTP API 和部署

1. 迁移 Codesome HTTP handlers。
2. 移除 `/api/cost`。
3. 删除 `internal/server/handler.go` 或裁剪掉 Claude Buddy handler。
4. 迁移 Dockerfile 和 compose。
5. 默认监听 `127.0.0.1`。
6. Docker/compose 中只保留 Codesome 服务。

验收：

- `codesome serve --port 8080` 正常启动。
- HTTP handler tests 通过。
- Docker compose 可启动。
- `/api/cost` 不存在。

### 阶段 4：迁移脚本

1. 迁移 `scripts/display_usage_go.sh`。
2. 重命名为 `scripts/display_usage.sh` 或 `scripts/codesome_usage.sh`。
3. 修改脚本路径和二进制名。

验收：

- 脚本可从新仓库根目录运行。
- 不再引用旧仓库路径。

### 阶段 5：加入 SQLite 管理能力

1. 添加 SQLite DB 初始化和 migration。
2. 添加 `teams`、`users`、`api_keys`、`usage_daily`、`sync_runs`。
3. 实现 team/user CRUD。
4. 实现 `sync users`。
5. 实现 `key export`。
6. 实现 `sync usage`。
7. 实现 `report monthly`。
8. 实现 `db import-config-keys`。

验收：

- 新功能不依赖 `config.yaml.api_key_ids`。
- 可从旧配置导入 legacy keys。
- usage 同步可幂等 upsert。
- inactive team 下不能新增 active user，也不能把已有 user 更新为 active。
- 月报可按所有团队或指定 team 输出。

### 阶段 6：当前仓库收尾

1. 当前仓库 README 标记 Codesome 管理能力已迁移。已完成。
2. 保留 Go Codesome 代码，定位为 Codesome-only 管理工具，并将旧 alias 命令说明为 legacy/simple usage helper。已完成。
3. 删除或停用重复部署脚本。已删除 daily reset cron，Docker/compose 只保留 Codesome 服务和 opt-in auto-switch。
4. 移除旧 `providers` 配置 fallback。已完成，只支持 top-level `codesome`。

验收：

- 当前仓库定位回到 usage helper。
- 新仓库承接 Codesome 管理职责。
- 没有两个仓库都在维护同一套 Codesome 管理逻辑。
- 业务代码不再包含 Claude Buddy / 88Code / 多 provider 配置模型。

阶段 6 已完成最终验收：

- `go test ./...` 通过。
- `go build ./...` 通过。
- `rg "Claude Buddy|claude-buddy|Codex Buddy|88Code|eightcode|ANTHROPIC_" .` 只命中迁移文档中的历史说明。
- `providers` 词本身仍会命中 legacy 配置忽略测试和 Codesome provider 包命名，不代表运行时仍保留多 provider 配置模型。

## 裁剪清单

新仓库完成 Codesome-only 收敛后，应删除或重写下列内容：

```text
internal/provider/claude_buddy.go
internal/server/handler.go
cmd/sync.go
cmd/root.go 中的 provider auto-detect
internal/config/config.go 中的 Claude Buddy / 88Code / providers 配置模型
README 中的 Claude Buddy / Codex Buddy 说明
Docker/compose 中的非 Codesome 路由或服务
```

需要保留但改名或重写：

```text
cmd/root.go -> Codesome-only root
cmd/serve.go -> 只注册 Codesome routes
internal/config/config.go -> Codesome-only config
scripts/display_usage_go.sh -> scripts/display_usage.sh 或 scripts/codesome_usage.sh
go/README.md -> 新仓库 README.md
```

完成裁剪后建议运行：

```bash
rg "Claude Buddy|claude-buddy|Codex Buddy|88Code|eightcode|ANTHROPIC_" .
go test ./...
go build ./...
```

允许 docs 中出现“从旧仓库迁移”这类历史说明；业务代码和用户-facing README 不应再以旧 provider 为主。

## 首个 Codex 任务提示词

在新仓库打开 Codex 后，可直接使用：

```text
请按照 docs/migration-from-claude-code-usage-helper.md 执行阶段 1-3：

1. 将当前复制过来的 Go 项目收敛为 Codesome-only CLI。
2. 删除 Claude Buddy、88Code、多 provider resolution 和 /api/cost。
3. 保留 Codesome usage、daily-usage、usage-stats、create/update key、reset-quota、switch-group、switch-on-exhausted、auto-switch、serve。
4. 将 root command 和帮助文案改成 codesome-usage-manager / codesome。
5. 更新 config.yaml.example 为 Codesome-only 配置。
6. 更新 README，说明迁移后的命令和安全边界。
7. 确保 gofmt、go test ./...、go build ./... 通过。

不要开始实现 SQLite 管理功能；阶段 5 以后再做。
```

## 风险和注意事项

- Codesome key 创建接口会返回完整 `sk-...`，必须避免日志泄露。
- 迁移时不要提交真实 `.codesome_auth.json` 或 `config.yaml`。
- `auto-switch` 是状态变更能力，已改为 opt-in，并只处理 active Codesome API Key。
- 新仓库如果提供 HTTP 管理 API，必须明确部署边界。
- SQLite 迁移后，`api_key_ids` 不应继续作为新功能的数据源。
- 不要在迁移初期同时改业务逻辑和做大规模架构重写；先保证现有 Codesome 能力在新仓库可运行。
- 新仓库初始复制后，可以用多个小 commit 分阶段裁剪，避免一次性 diff 过大。

## 与当前设计文档的关系

SQLite-backed user/key/usage 设计见：

```text
docs/codesome-api-key-management-design.md
```

本文件只描述从当前仓库拆分到新仓库的迁移计划。

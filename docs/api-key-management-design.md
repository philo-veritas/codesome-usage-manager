# Codesome API Key 管理设计

## 背景

当前 Codesome key 主要通过 `config.yaml` 的 `api_key_ids` 静态维护，适合少量团队级 key 的查询、切换和重置。

仓库拆分迁移计划见 [migration-from-claude-code-usage-helper.md](migration-from-claude-code-usage-helper.md)。

后续目标是用本项目管理 Codesome 上的 API Key：

- 为研发个人创建并分发 API Key。
- 支持把研发个人归属到团队，用团队口径做用量汇总。
- 本地维护人员状态，统一同步到 Codesome key 状态。
- 按天采集每个 key 的历史用量。
- 从“每个小团队一个共享 key”平滑演进到“每个研发一个 key”。

新功能应以本地 SQLite DB 作为 key 清单和人员状态的来源，不再依赖 `config.yaml` 中的 `api_key_ids`。`config.yaml` 只保留 Codesome 连接和认证配置。

## 设计原则

1. 本地 DB 是管理状态的来源。
2. Codesome 是远端执行系统，`sync` 命令负责让远端状态收敛到本地期望状态。
3. `teams` 管理团队归属，`users` 管理研发个人，`api_keys` 管理远端 key 资源映射。
4. usage 按天保存历史快照，只采集已结束日期的数据。
5. 保留现有 `api_key_ids` 作为 legacy 静态配置能力，新增 SQLite 流程默认不依赖它。
6. 创建 key 返回的 `sk-...` 默认不长期明文保存；如需分发，应显式选择存储策略。

## 配置边界

`config.yaml` 继续保存 Codesome 登录信息：

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

`default_group_id` 是新管理流程创建 key 时使用的全局默认 Codesome group。`users.codesome_group_id` 只作为个人级覆盖值；大多数用户不需要单独配置。

`api_key_ids` 不作为新功能的数据源。它只用于现有命令兼容，例如：

- `daily-usage --key main`
- `switch-group --key main`
- `switch-on-exhausted --all`

## SQLite 表结构

### teams

`teams` 表保存团队信息，用于归属和用量汇总。团队默认不作为 key owner，也不直接分发 key。

```sql
CREATE TABLE teams (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  code TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('active', 'inactive')),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
```

字段说明：

- `code`: 团队稳定标识，例如 `platform`、`infra`。
- `name`: 团队展示名称。
- `status`: 团队是否仍参与统计和新增人员分配。

### users

`users` 表保存研发个人。人员通过 `team_id` 归属到团队。

```sql
CREATE TABLE users (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  employee_no TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  team_id INTEGER REFERENCES teams(id),
  status TEXT NOT NULL CHECK (status IN ('active', 'inactive', 'deleted')),
  codesome_group_id INTEGER,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  deleted_at TEXT
);

CREATE INDEX idx_users_team_id ON users(team_id);
CREATE INDEX idx_users_status ON users(status);
```

字段说明：

- `employee_no`: 人员稳定标识，通常是工号。
- `name`: 展示名称，也可用于生成 Codesome key name。
- `team_id`: 所属团队。为空表示暂未归属团队。
- `status`: 本地期望状态。
- `codesome_group_id`: 个人级 Codesome group 覆盖值。为空时使用全局 `default_group_id`。
- `deleted_at`: 软删除时间。删除用户不删除历史 usage。

### api_keys

`api_keys` 表保存本地 user 与 Codesome API Key 的映射。

```sql
CREATE TABLE api_keys (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id INTEGER NOT NULL REFERENCES users(id),
  codesome_key_id INTEGER NOT NULL UNIQUE,
  name TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('active', 'inactive')),
  group_id INTEGER NOT NULL,
  raw_key TEXT,
  raw_key_stored_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  last_synced_at TEXT
);

CREATE INDEX idx_api_keys_user_id ON api_keys(user_id);
CREATE INDEX idx_api_keys_status ON api_keys(status);
```

为什么 key 独立成表：

- 一个人未来可能有多个 key。
- key 可能轮换、禁用或迁移 group。
- 远端 key 的生命周期和人员生命周期不同。
- usage 应该关联 key，而不是只关联 user。

团队用量通过 `usage_daily -> api_keys -> users -> teams` 聚合。团队不直接绑定 key；历史团队共享 key 可迁移为一个 legacy/virtual user，并挂到对应 team 下。

`raw_key` 策略：

- 可以明文保存，因为 Codesome 网站本身可以持续查看 key。
- 不应写入日志或错误信息。
- 分发动作做成独立命令，例如导出 CSV 或按 user 输出。
- 后续如有更高安全要求，再考虑加密保存或定期清理。

### usage_daily

`usage_daily` 表按 key 和日期保存历史用量。

```sql
CREATE TABLE usage_daily (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  api_key_id INTEGER NOT NULL REFERENCES api_keys(id),
  usage_date TEXT NOT NULL,
  total_requests INTEGER NOT NULL DEFAULT 0,
  total_input_tokens INTEGER NOT NULL DEFAULT 0,
  total_output_tokens INTEGER NOT NULL DEFAULT 0,
  total_cache_tokens INTEGER NOT NULL DEFAULT 0,
  total_tokens INTEGER NOT NULL DEFAULT 0,
  total_cost REAL NOT NULL DEFAULT 0,
  total_actual_cost REAL NOT NULL DEFAULT 0,
  average_duration_ms REAL NOT NULL DEFAULT 0,
  fetched_at TEXT NOT NULL,
  UNIQUE(api_key_id, usage_date)
);

CREATE INDEX idx_usage_daily_date ON usage_daily(usage_date);
CREATE INDEX idx_usage_daily_api_key_date ON usage_daily(api_key_id, usage_date);
```

采集规则：

- `usage_date` 使用 `YYYY-MM-DD`。
- 时区固定 `Asia/Shanghai`。
- `start_date` 和 `end_date` 在 Codesome API 中左右包含。
- 默认只采集今天之前的日期，避免当天数据未稳定。
- 重复采集同一天时使用 upsert。
- 预计管理 30-50 个 key，按天记录的数据量很小，默认长期保留，不做归档或压缩。
- 如果未来出现合规或性能需求，再增加按年份导出和清理能力。

### sync_runs

`sync_runs` 记录同步任务执行情况，便于排查和审计。

```sql
CREATE TABLE sync_runs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  kind TEXT NOT NULL CHECK (kind IN ('users', 'usage', 'import')),
  started_at TEXT NOT NULL,
  finished_at TEXT,
  status TEXT NOT NULL CHECK (status IN ('running', 'success', 'failed')),
  message TEXT
);
```

## 状态模型

`users.status` 是本地期望状态：

- `active`: 应该存在可用 Codesome key。
- `inactive`: key 应该存在但远端状态应为 `inactive`。
- `deleted`: 软删除。保留历史记录，远端 key 应禁用。

`api_keys.status` 是最后一次同步后记录的远端状态：

- `active`
- `inactive`

`sync users` 负责把远端状态同步为本地期望：

| 本地 user 状态 | 本地 key 状态 | 同步动作 |
| --- | --- | --- |
| active | 不存在 | 创建 Codesome key 并写入 `api_keys` |
| active | inactive | 更新远端 key 为 `active` |
| inactive | active | 更新远端 key 为 `inactive` |
| deleted | active | 更新远端 key 为 `inactive` |
| active | group 不一致 | 更新远端 key 的 `group_id` |
| active | name 不一致 | 可选更新远端 key 的 `name` |

团队状态不会直接触发 key 创建或禁用。inactive team 下禁止新增 active user，也禁止把已有 user 更新为 active。

## 命令设计

### DB 初始化

```bash
codesome db init
codesome db migrate
```

### 用户管理

```bash
codesome team add --code platform --name "Platform"
codesome team update --code platform --status inactive
codesome team list

codesome user add --employee-no E12345 --name "Alice" --team platform --group-id 51
codesome user update --employee-no E12345 --status inactive
codesome user update --employee-no E12345 --team infra
codesome user delete --employee-no E12345
codesome user list
```

说明：

- team 只用于归属和统计，默认不创建 Codesome key。
- user 是 key 分发的默认对象。
- inactive team 下不能新增 active user。
- 如果 user 所属 team 是 inactive，不能把该 user 状态更新为 active。
- `delete` 默认软删除。
- 软删除后下一次 `sync users` 会禁用远端 key。
- 如确实需要物理删除，应单独提供危险命令，并默认禁止删除已有 usage 的 user。

### 同步人员与 Key

```bash
codesome sync users
codesome sync users --dry-run
codesome sync users --employee-no E12345
codesome sync users --full
```

同步行为：

1. 读取本地 `users`。
2. 查找对应 `api_keys`。
3. 对缺失 key 的 active user 创建 Codesome key。
4. 对状态、group、name 不一致的 key 调用 Codesome 更新接口。
5. 写入 `api_keys.last_synced_at`。

默认只同步本地变更：

- active user 缺失本地 key 时创建远端 key。
- 本地 key 状态、group、name 与 user 期望不一致时更新远端 key。
- `api_keys.last_synced_at` 为空时补同步一次。
- `users.updated_at` 晚于 `api_keys.last_synced_at` 时重新应用期望状态。
- 其他已匹配 user 输出 `noop`，不调用 Codesome。

对于未设置个人 `codesome_group_id` 的 active user，期望 group 仍按运行时可用余额最多的 group 计算；默认模式会读取 Codesome subscription 来判断 group 是否需要更新，但只对有差异的 key 调用更新接口。

`--full` 会全量收敛匹配到的本地 user，重新应用所有现有 key 的期望状态。它用于修正远端被人工修改但本地未变化的漂移。

`--dry-run` 只输出计划，不创建或更新 Codesome key；为准确预览运行时 group 选择，它可能读取 Codesome subscription。

### Key 分发与导出

```bash
codesome key export --employee-no E12345
codesome key export --team platform --output keys-platform.csv
codesome key export --all --output keys.csv
codesome key export --all --include-inactive --output keys-all.csv
```

导出规则：

- 默认只导出 active user 关联的 active key。
- `--employee-no`、`--team`、`--all` 三者互斥。
- 输出字段建议为 `employee_no,name,team,key_name,codesome_key_id,raw_key,status`。
- 导出命令可以把 `raw_key` 写入目标文件，但不应把完整 key 打到普通日志。
- 如果 `raw_key` 为空，仍导出元信息，并标记该行无法本地分发。Codesome 网站仍可人工查看 key。
- 后续如接入内部系统，可以在导出命令基础上增加专门的分发 adapter。

### 同步历史用量

```bash
codesome sync usage --date 2026-05-26
codesome sync usage --from 2026-05-01 --to 2026-05-26
codesome sync usage --yesterday
```

同步行为：

1. 读取本地 `api_keys`。
2. 默认只处理非 deleted user 关联的 key。
3. 对每个 key 调用 Codesome `/api/v1/usage/stats`。
4. 按 `(api_key_id, usage_date)` upsert 到 `usage_daily`。
5. 默认不采集当天，除非显式传 `--include-today`。

### 查询

```bash
codesome usage daily --date 2026-05-26
codesome usage user --employee-no E12345 --from 2026-05-01 --to 2026-05-26
codesome usage team --team platform --from 2026-05-01 --to 2026-05-26
codesome usage top --from 2026-05-01 --to 2026-05-26 --by actual-cost
codesome report monthly --month 2026-05
codesome report monthly --month 2026-05 --team platform
codesome report monthly --month 2026-05 --output report-2026-05.csv
```

月报规则：

- `--month` 使用 `YYYY-MM`。
- 月报按自然月和 `Asia/Shanghai` 日期统计。
- 默认输出所有 team 和 user 的汇总。
- 支持按 team 过滤。
- CSV 字段建议包含 `month,team,user,employee_no,total_requests,total_tokens,total_actual_cost`。

## HTTP API 设计

当前使用场景只需要 CLI，不优先实现新的 SQLite 管理 HTTP API。如果未来需要给内部系统调用，HTTP API 可以围绕本地 DB 暴露，而不是直接围绕 `config.yaml`。

建议接口：

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

GET    /api/codesome/usage/daily?date=2026-05-26
GET    /api/codesome/usage/user?employee_no=E12345&from=2026-05-01&to=2026-05-26
GET    /api/codesome/usage/team?team=platform&from=2026-05-01&to=2026-05-26
GET    /api/codesome/reports/monthly?month=2026-05
```

安全要求：

- 默认只监听 `127.0.0.1`。
- 涉及创建、更新、删除、同步的 HTTP API 如未来对外提供，必须要求认证或仅暴露在可信内网。
- 创建 key 返回的 `sk-...` 只能返回给创建请求，不应写入日志。

## 从远程 API 或 config.yaml 迁移

首选一次性导入命令：

```bash
codesome db import-remote-keys
```

远程导入策略：

1. 通过 Codesome API 读取当前 API Key 列表。
2. 为每个尚未入库的 key 创建一个 virtual user，例如 `employee_no = codesome-key:<id>`。
3. 写入 `api_keys.codesome_key_id`、`name`、`status`、`group_id`。
4. 不填 `raw_key`。
5. 后续人工把 virtual user 归并到真实研发个人和团队，并保留历史 usage 可追溯。

`config.yaml` 导入只作为 legacy 兜底：

```bash
codesome db import-config-keys
```

迁移策略：

1. 读取 `config.yaml` 中的 `api_key_ids`。
2. 为每个 key 创建一个 legacy/virtual user，例如 `employee_no = legacy:<key>`。
3. 如果能判断团队归属，则把 legacy user 挂到对应 team；否则挂到 `legacy` team 或保持 `team_id` 为空。
4. 写入 `api_keys.codesome_key_id`、`name`、`group_id`。
5. 不填 `raw_key`。
6. 后续人工把 legacy user 归并到真实研发个人，并保留历史 usage 可追溯。

## 实施阶段

### 阶段 1：本地 DB 基础

- 增加 SQLite 初始化和迁移。
- 增加 `teams`、`users`、`api_keys`、`usage_daily`、`sync_runs` 表。
- 增加 team/user CRUD CLI。

### 阶段 2：Key 同步

- 实现 `sync users`。
- 对接已有 Codesome create/update provider。
- 支持 `--dry-run`。
- 创建 key 后输出或短期保存 `raw_key`。

### 阶段 3：Usage 同步

- 实现 `sync usage --date/--from/--to/--yesterday`。
- 使用已有日期范围 usage stats provider。
- 使用 upsert 保证幂等。

### 阶段 4：查询与报表

- 增加按 user、日期、成本排序的 CLI 查询。
- 增加月报命令，支持按 team 过滤和 CSV 导出。

### 阶段 5：迁移与废弃静态 key 清单

- 实现 `db import-remote-keys` 作为默认 bootstrap 路径。
- 实现 `db import-config-keys`。
- 文档标记 `api_key_ids` 为 legacy。
- 新功能默认不读取 `api_key_ids`。

## 待确认问题

暂无。

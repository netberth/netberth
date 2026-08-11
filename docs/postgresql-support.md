# PostgreSQL 支持 — 方案评审（2026-08-11）

## 目标

- 新增 `NB_DB_DRIVER=postgres` + `NB_DB_DSN=postgres://...` 支持；
- SQLite 保持默认、零配置，行为完全不变；
- 支持多副本部署场景（同一 Postgres 供多实例连接）。

## 架构决策

### 1. 占位符方言化：驱动包装层（关键决策）

全项目数百个查询都使用 `?` 占位符（Go 标准 `database/sql` 风格），Postgres 需要 `$N`。
**不改任何调用点**，而是在 `sql.Open` 时包一层自定义 `driver.Driver`：
对 `Query`/`Exec`/`Prepare` 的 SQL 做 `?` → `$1..$N` 重写后转交 pgx。
这样 handler / service / engine 全部保持 `*sql.DB` 接口不变，改动面收敛到 `internal/db`。

### 2. SQLite 专有语法收敛（可移植写法优先，翻译兜底）

- `INSERT OR REPLACE INTO settings ...`（license Activate）改为
  `INSERT ... ON CONFLICT(key) DO UPDATE SET value=excluded.value`（SQLite ≥3.24 与 PG 均支持）；
- `INSERT OR IGNORE INTO tenants ...`（SeedAdminUser）改为 `ON CONFLICT(id) DO NOTHING`；
- 其余 SQL 本身可移植（TEXT/INTEGER/`?`）。

### 3. 迁移按方言提供

- SQLite：沿用现有 `migrations.go` + `ensureColumn`（PRAGMA）；
- Postgres：新增 `postgres_migrations.go`（`TIMESTAMP`、`BIGSERIAL`/`GENERATED ALWAYS AS IDENTITY`、`information_schema` 列检查）；
- 统一入口 `runMigrations(db, dialect)`。

### 4. 连接管理

- `db.OpenDatabase(driver, dsn)`：sqlite 走现有路径；postgres 用 pgx stdlib，
  `SetMaxOpenConns` 按 CPU 配置、`conn_max_lifetime` 与 sqlite 一致。

## 涉及文件

| 文件 | 改动 |
|---|---|
| `internal/db/dialect.go` | Dialect 常量 + Rebind（`?`→`$N`） |
| `internal/db/driver_rebind.go` | 驱动包装层（M2） |
| `internal/db/postgres_migrations.go` | Postgres DDL（M2） |
| `internal/db/db.go` | `OpenDatabase` + 方言分派（M2） |
| `internal/config/config.go` | `NB_DB_DRIVER` / `NB_DB_DSN`（M1 已做） |
| `cmd/netberth/main.go` | 按配置调 `OpenDatabase`（M3） |
| `internal/api/handler/license_handler.go` | `INSERT OR REPLACE` → `ON CONFLICT`（M2） |
| `go.mod` / `vendor` | 新增 `github.com/jackc/pgx/v5`（M2） |

## 风险点

1. **驱动包装层与 prepared statement 交互**：pgx stdlib 已实现
   `QueryerContext`/`ExecerContext`，包装层必须同时处理 `Prepare` 的 SQL 重写；
   用 `database/sql` 全路径测试（QueryRow/Exec/Prepare）兜底。
2. **双份迁移漂移**：SQLite 与 Postgres DDL 需保持列一致，用“schema 对照测试”约束
   （比较两方言迁移后的列集合，M2）。
3. **类型差异**：`bool`/`time.Time`/`NULL` 扫描差异，用可选集成测试覆盖。
4. **CI 无 Postgres 实例**：真实连接测试用 `NB_TEST_POSTGRES_DSN` 环境变量门控，
   未设置则 skip；CI 不强制跑。

## 回滚路径

- 默认 `NB_DB_DRIVER` 为空 → sqlite，与现状完全一致；不设置新环境变量即零影响；
- 所有改动收敛在 `internal/db` 与配置层，失败可整体 revert；
- pgx 依赖若出问题：`git revert` go.mod/vendor 即可回到纯 SQLite 构建。

## 里程碑状态（2026-08-11）

- **M1 ✅**：设计文档 + `Rebind` 方言层（带状态扫描器：字符串/标识符/注释）+ 配置字段与测试。
- **M2 ✅**：pgx v5.7.1 依赖 + `pgx-rebind` 驱动包装层（Query/Exec/Prepare 重写 `?`→`$N`）
  + `OpenDatabase` 方言分派 + Postgres 迁移 + SQLite/Postgres schema 对照测试。
  注：pgx stdlib 原生只支持 `$N`，驱动包装层为必需。
- **M3 ✅**：main 接线（`NB_DB_DRIVER`/`NB_DB_DSN` + 数据目录回退 `./data`）
  + `INSERT OR REPLACE`/`INSERT OR IGNORE` 收敛为 `ON CONFLICT` + CI 升级 Go 1.25
  + 可选集成测试（`NB_TEST_POSTGRES_DSN` 门控，CI 不强制）。
- **真实库验证 ✅（2026-08-11）**：临时 postgres:16-alpine 容器 + 独立库，
  `NB_TEST_POSTGRES_DSN` 集成测试通过（连接/`?` 重写/迁移/SeedAdminUser），
  建出 19 张表与 SQLite 对照一致；容器已清理。
- **待办**：完成 v1.1.0 发布流程（tag/zig 构建/strings 检查/sha256/Draft Release），
  并把 PostgreSQL 与 CI Go 1.25 变更同步到公开仓库。

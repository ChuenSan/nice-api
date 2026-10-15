# 01: 原子全局优先级保存与并发保护

Type: task
Status: resolved
Blocked by: None

## What to build

依据 [spec](../spec.md)，提供稳定分页、排序快照和仅表达页内移动的全局重排 API，保存后渠道及路由能力优先级一致。

## Acceptance criteria

- 列表采用 priority DESC, id DESC，返回与集合及优先级一致的排序快照。
- 服务端验证页内移动与期望快照，全局编号 N 到 1，只更新优先级。
- 所有渠道新增、删除、优先级编辑及能力重建参与数据库级协调，支持多进程，避免比较与写入窗口及旧能力覆盖。
- HTTP 409 拒绝过期快照；事务整体回滚；使用既有渠道写权限；缓存与实际选路反映已提交结果。
- TDD：真实 API/数据库入口验证全局顺序、无效操作、冲突、失败回滚及实际选路，最多一个新增 Go 测试文件。

## Verification seam

采用规格 Testing Decisions 中的控制器/API、持久化数据及公开选路入口；不扩大到认证重构。

## Answer

后端实现与验收完成（2026-10-15，分支 `feat/channel-priority-api`；代码 HEAD `625521542`，已并入最新集成分支 `d8bda4d04`）。

### 协调器 fixture 修复

`4d30b63a2`：`middleware/distributor_test.go` 的纯缓存 fixture 让 `model.DB` 指向未迁移的 SQLite；选路在无匹配渠道时也会读取已提交排序版本，触发缺表 panic。fixture 迁移 `Option/Channel/Ability` 并在清理时还原全局句柄。修复前红见 `tools/backend-final-tests.log`。

### 命令与结果（workdir 为 worktree 根；`GOWORK=off CGO_ENABLED=0 GOMAXPROCS=2`，Go 1.25.1）

- 模块 fixture：`go test -p 1 ./middleware -run 'TestDistributeHidesTaskPluginDetailsButLogsDiagnostics' -count=1 -v` → exit 0，`--- PASS`（`tools/resume-middleware-fixture.log`）。
- 三引擎完整重排矩阵：`TEST_MYSQL_DSN='root@tcp(127.0.0.1:13379)/channel_priority_test?parseTime=true' TEST_POSTGRES_DSN='postgresql://postgres@127.0.0.1:15439/postgres?sslmode=disable' go test -p 1 ./controller -run 'TestChannelPriority' -count=1 -v` → exit 0，7 个顶层测试 / 29 个子测试通过，真实 SQLite 3.50.4、MySQL 8.0.32、PostgreSQL 15.19（`tools/resume-priority-matrix.log`）。
- 宽后端套件：`... go test -p 1 ./middleware ./model ./router ./service ./controller -count=1` → exit 0，`ok` 覆盖全部五个包（`tools/resume-backend-broad.log`）。
- 根构建：`go build -p 1 ./...`（使用实际 `web/dist`）→ exit 0，空输出（`tools/resume-root-build.log`）。`gofmt -l middleware/distributor_test.go` 无输出。

### 验收对照

列表/快照、页内移动校验与 N..1 全局编号、所有渠道写入路径数据库级协调与多进程、过期快照 409、注入失败整体回滚、渠道写权限、缓存与实际选路即时生效，均由上述矩阵及更早的 `tools/list-matrix.log`、`conflict-rollback-matrix.log`、`ability-slice-matrix.log`、`priority-complete-matrix.log` 覆盖；仅新增一个 Go 测试文件 `controller/channel_priority_test.go`。

### 限制

既有 `TestSecurityAccountDeletionConcurrentRequestsHaveOneWinner` 的 SQLite busy 抖动与本特性无关，本轮全套件未复现，历史单测复跑通过（`tools/account-delete-isolated.log`）；未改动认证运行时。

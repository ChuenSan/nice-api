# 03: 三数据库验收与集成审查

Type: task
Status: claimed
Blocked by: 01, 02

## What to build

完整验证 [spec](../spec.md) 的全局重排、并发、事务、选路和用户交互，交付可审查的单一集成分支。

## Acceptance criteria

- 真实 SQLite、MySQL、PostgreSQL 记录精确版本、命令和结果；若新增迁移还需新建、升级、重复启动和数据保留。
- 相关 Go 测试、前端测试、typecheck、lint、格式、翻译同步/校验及生产构建通过。
- 按 code-review 技能并行 Standards / Spec 审查并修复所有有效问题。
- 工单记录验收证据；有阻塞保持未解决，不将跳过当通过；清理临时实施 worktree。

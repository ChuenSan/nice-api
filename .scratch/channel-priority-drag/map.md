# Channel priority drag

Spec: [spec.md](spec.md)
Integration branch: integration/channel-priority-drag
Base commit: 51205165eda31ca49cc1962f8344d6a54ba57f48

## Notes

本次 implement-spec 请求授权实施。规格 Out of Scope 的“本轮实现”描述原规格编写轮次，不限制本次实施。
本地 tracker 已配置；原规格无实施工单，本次按规格拆分。
原工作区未提交 GLOSSARY.md、.pi/ 及规格保持原样。实施在隔离检出中进行，最终将集成分支带回原仓库。临时目录曾被环境重启清理，现已恢复到持久隔离目录 /home/user/0github/nice-api-channel-priority-recovered。后续环境中断导致部分新 Git 对象为空，已从有效集成分支与源码备份恢复；Git 完整性检查通过。新提交启用 core.fsync=all（仅命令参数）并同步落盘。

## Decisions-so-far

测试入口采用规格 Testing Decisions：真实控制器/API、数据库与选路；真实表格及 API 边界；浏览器指针验证。

- `73cb87b906` 合入 `9231b44ef`：稳定列表及原子排序快照。
- 首个已执行矩阵：`GOMAXPROCS=2 TEST_MYSQL_DSN='root@tcp(127.0.0.1:13379)/' TEST_POSTGRES_DSN='postgresql://postgres@127.0.0.1:15439/postgres?sslmode=disable' go test -p 1 ./controller -run '^TestChannelPriorityList$' -count=1 -v`；SQLite 3.50.4、MySQL 8.0.32、PostgreSQL 15.19 全通过（2026-10-15）。此结果仅覆盖列表快照切片，尚未验证完整重排。初始化 helper 在建表前尝试读取 options 产生既有日志，功能测试通过。

- 集成分支已通过原子重排、旧能力重建、冲突、注入失败回滚、多进程重排/编辑与缓存即时刷新验收，完整 SQLite 3.50.4、MySQL 8.0.32、PostgreSQL 15.19 均真实执行；最终命令、真实退出码、审查处置与限制已记录于 [03](issues/03-verify-and-review.md#answer)。
- 一致性代价：开启内存缓存时每次选路读取 options 中的提交版本；数据库读取失败会中止选路，避免继续使用过期优先级。列表快照与缓存重建取得数据库协调锁，SQLite 使用写锁保证读取一致。这些是本实现满足全局快照及多进程即时生效的约束，增加了数据库往返与竞争；未做性能测量。
- ticket02 前端完成：`8cc3fce46` 让自定义 `renderRow` 经 `helpers.className` 保留 `getRowClassName` 解析的行样式（修复移动端禁用行变体），回归测试覆盖默认/自定义路径；渠道+data-table+keys 36 文件 407 测试、typecheck、build、i18n 校验通过；复用缺口、命令与浏览器证据见 [02](issues/02-accessible-page-reorder.md#answer)。

- 最终代码集成点 `4b4d9b1e`：01/02/03 全部 resolved，规格实现闭合。最终前端 36 文件 / 408 测试、类型/改动文件静态检查/七语言/生产构建通过，根构建使用真实 dist 并明确 exit 0；Go 五包及三库矩阵通过，最终 Go 源与验收点逐字节相同。Standards / Spec 各 0 未解决有效问题并批准。已移除三个实施 worktree，保留集成检出，原用户改动校验和不变。全局既有 lint 165 / format 20 不在改动范围，未声称全仓检查通过；精确证据、成本与其他限制见 [03](issues/03-verify-and-review.md#answer)。

接口契约：完整列表提供 data.order_snapshot；重排 POST 接收期望快照与页内 moved_id/target_id。

## Tickets

- [01 原子全局优先级 API](issues/01-atomic-priority-api.md) — resolved
- [02 可访问页内拖动](issues/02-accessible-page-reorder.md) — resolved
- [03 验证与审查](issues/03-verify-and-review.md) — resolved

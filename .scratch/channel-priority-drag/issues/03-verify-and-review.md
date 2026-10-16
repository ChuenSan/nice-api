# 03: 三数据库验收与集成审查

Type: task
Status: resolved
Blocked by: 01, 02

## What to build

完整验证 [spec](../spec.md) 的全局重排、并发、事务、选路和用户交互，交付可审查的单一集成分支。

## Acceptance criteria

- 真实 SQLite、MySQL、PostgreSQL 记录精确版本、命令和结果；若新增迁移还需新建、升级、重复启动和数据保留。
- 相关 Go 测试、前端测试、typecheck、lint、格式、翻译同步/校验及生产构建通过。
- 按 code-review 技能并行 Standards / Spec 审查并修复所有有效问题。
- 工单记录验收证据；有阻塞保持未解决，不将跳过当通过；清理临时实施 worktree。

## Answer

2026-10-15 完成集成验收。代码基线 `51205165eda31ca49cc1962f8344d6a54ba57f48`，最终代码审查与前端构建点 `4b4d9b1e07124c24ac7253f2ea96a3efc4224fa8`，分支 `integration/channel-priority-drag`。01、02 已 resolved；本提交仅补齐 tracker，不改变已验证代码。

### 后端与真实三数据库

Go 1.25.1 linux/arm64；工作目录为隔离集成检出。公共环境：

```sh
export GOWORK=off CGO_ENABLED=0 GOMAXPROCS=2
export TEST_MYSQL_DSN='root@tcp(127.0.0.1:13379)/channel_priority_test?parseTime=true'
export TEST_POSTGRES_DSN='postgresql://postgres@127.0.0.1:15439/postgres?sslmode=disable'
go test -p 1 ./middleware ./model ./router ./service ./controller -count=1
go test -p 1 ./controller -run TestChannelPriority -count=1 -v
```

- 两条命令均 exit 0，五个包均 `ok`；功能矩阵 8 个顶层测试、29 个子测试通过，真实 SQLite **3.50.4**、MySQL **8.0.32**、PostgreSQL **15.19** 均有版本与执行记录。
- 覆盖稳定分页/快照、全局 N..1（含停用渠道及页外相对顺序）、参数/空操作、旧能力重建、重排/单渠道/标签编辑/新增删除冲突、注入失败整体回滚、多进程写入协调与即时缓存选路、权限及 nil DB fail-closed。
- 唯一 `SKIP` 是独立启动的 `TestChannelPriorityProcess` 子进程 helper；`TestChannelPriorityMultiProcess` 已在三库实际执行并通过，不把 helper 跳过当数据库验收。
- 上述 Go 检查固定于 `3abaac2eb`；其后只有两处 TS 文件调整，`git diff 3abaac2eb..4b4d9b1e -- '*.go'` 为空，最终双轴审查亦核实 Go 源逐字节相同。所有改动 Go 文件 `gofmt -l` 无输出。
- 未新增 schema、迁移或 ORM/driver 依赖；协调行复用原有 `options` 主键并事务内按需创建，既有主键迁移在基线已存在。因此未声称执行新迁移的发布版升级/重复启动验收；未独立运行最低 MySQL 5.7.8 / PostgreSQL 9.6 实例。
- 日志根目录 `/home/user/0github/channel-priority-tools/`：`resume-final-backend-results.log`、`resume-final-backend-broad.log`、`resume-final-priority-matrix.log`、`resume-final-backend-gofmt.log`。

### 前端与完整嵌入构建

在最终代码点 `4b4d9b1e` 的 `web/` 执行：

```sh
bun x vitest run src/features/channels src/components/data-table src/features/keys --maxWorkers=2
bun run typecheck
bun x oxlint <本分支全部 13 个改动 TS/TSX 文件>
bun x oxfmt --check <全部改动 TS/TSX 与七语言 JSON>
bun run i18n:sync
PRIORITY_WEB_ROOT="$PWD" node /home/user/0github/channel-priority-tools/verify-frontend-i18n.mjs
bun run lint
bun run format:check
bun run build
```

- **36 文件 / 408 测试通过**；typecheck、全部改动文件 lint/format、同步、七语言校验、生产构建均 exit 0。精确展开的文件参数和逐命令退出码在 `resume-final-frontend-results.log`，未用占位符命令冒充实际执行。
- 七语言共检查 33 个源键与 22 条特性文案，翻译及占位符完整；这不是全站所有历史翻译质量已验证的声明。
- 全局 `bun run lint` exit 1：**165** 条既有错误；全局 `format:check` exit 1：**20** 个既有文件。错误路径与本分支改动文件交集为空；未修改无关文件以掩盖全局失败，也未声称全仓静态检查通过。
- 组件复用缺口与浏览器证据见 [02](02-accessible-page-reorder.md#answer)：真实指针乐观保存、取消/原位/越界不提交、滚动/焦点及 tr/td 语义已观察。浏览器采用本地模拟 API；后端真实 API/DB 分开验收。移动端禁用行样式由组件回归覆盖，未声称新增真实移动设备浏览器验证。
- 使用本次 `bun run build` 实际生成的 `web/dist/index.html`，在根目录执行 `GOWORK=off CGO_ENABLED=0 GOMAXPROCS=2 go build -p 1 ./...`，**明确捕获 exit 0**（`resume-final-root-build-result.log`）；不是空日志推断成功。
- 其余日志：`resume-final-frontend-affected.log`、`resume-final-frontend-{typecheck,lint-changed,format-changed,i18n-sync,i18n-verify,lint-global,format-global,build}.log`。

### Standards

独立只读 Review，最终集成点 `4b4d9b1e`，**未解决有效发现 0，Approve**。已修 nil DB fail-closed、移动端行样式、Enter-only 保存；最后消除快照重复断言并移除本地 handle export。上轮 upstream settings helper 的“唯一调用点”被实际三处调用反驳，审查者撤回；机械单行更新/失效及文件体量的启发式建议按根规范不作推测性抽象。

元数据/定价锁疑虑已核对外层事务：定价行 → 元数据协调行 → 渠道协调行 → 渠道/能力，无反向取得造成的具体环。缓存锁在定价失效前释放，不作无证据锁重写。

### Spec

独立只读 Review，与 Standards 并行，最终集成点 **未解决有效代码发现 0，Approve**。原 JSON codec 范围疑虑撤回：改动 Go 使用 `common.Marshal` 是根规范强制要求，配置仍为原 map→JSON 字符串语义，无配置字段/类型重设计。本 Answer 完成原 F2 验收工件缺口。

两轴均实际以 **DeepSeek V4.1 Flash**（显式 provider/model 与会话元数据确认）运行，未嵌套代理。完整原始报告存于 `resume-final-review-{standards,spec}-3-events.jsonl`，摘要与逐项处置为 `resume-final-review-approved.md` / `resume-final-review-fix-disposition.md`。审查是静态证据，不声明已证明正确性。

### 权限控制与 OWASP 参考

新增全局入口复用现有渠道权限，服务端同时要求完整读取及写入权限，并验证浏览器会话、PAT scope+permission、旧令牌允许/拒绝路径；未重设计登录、凭据存储或会话协议。复核参考 [OWASP Authentication](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html)、[Session Management](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html)、[Authorization](https://cheatsheetseries.owasp.org/cheatsheets/Authorization_Cheat_Sheet.html)（2026-10-15 已读取的稳定指南）。未据此声称整个项目达到 ASVS 合规。

### 清理与已知限制

- 三个实施 worktree 均 clean、提交为集成分支祖先并备份后移除；保留 `/home/user/0github/nice-api-channel-priority-recovered` 供审阅。`git fsck --full` exit 0，仅可有历史 dangling 对象。
- 原始工作区仍 `main@51205165`；`GLOSSARY.md` 差异哈希与 `.pi/`、原 `.scratch/channel-priority-drag/` 全部文件校验和保持不变，不将集成 tracker 覆盖回用户未提交目录。
- 仅启动后停止两个一次性测试数据库容器；线上 `new-api*` 容器、数据库未改动。不推送、不创建 PR、不部署；最终分支通过本地 bundle/fetch 带回原仓库，不切换其工作区。
- 选路每次读取 DB 修订号、快照/重建取协调锁是即时多进程一致性的代价，DB 失败时 fail-closed；未作性能测量，不声明无延迟/争用成本。
- 历史无关 SQLite 账号删除并发测试曾 busy；隔离重跑与本轮两个正确 DSN 的完整后端回归均通过，不改认证运行时代码。环境中断后的首次最终运行因测试 DB 停止失败，日志保留为 `resume-final-missing-db-*`，已被就绪三库的完整成功重跑取代。
- 早期 `-p1` 误运行根包无测试、空/中断日志均不作成功证据；本 Answer 命令全部使用 `-p 1` 并记录真实退出码。

# 02: 渠道表格页内拖动与保存反馈

Type: task
Status: resolved
Blocked by: None

## What to build

依据 [spec](../spec.md)，在渠道 ID 左侧提供鼠标及键盘拖动，在完整优先级视图中乐观保存当前页顺序。

## Acceptance criteria

- 组合或兼容扩展既有 sortable/data-table 能力，不破坏表格、行选择及优先级编辑。
- 搜索、筛选、聚合、非优先级降序、空/单行及保存中禁用，提供可访问说明。
- 有效移动立即展示并保存；原位、取消及无效落点不提交；与行内编辑调度协调。
- 普通失败恢复，409 与不确定结果读取最新服务器状态且不自动重放写入。
- 明示全局编号拆开同级分流；全部七种语言翻译、操作通知及无障碍名称完整。
- TDD：真实渠道表格、API 边界与键盘交互覆盖成功、禁用、失败、409、取消/空操作和保存期禁拖。

## Coordination

接口契约已选定，前后端可按 API 边界并行实施，由 03 集成验收。完整列表返回 data.order_snapshot；POST /api/channel/priority/reorder 携带 expected_snapshot、page、page_size、moved_id、target_id，冲突为 HTTP 409。

## Verification seam

采用规格 Testing Decisions 的真实表格与 API 网络边界；实际指针、滚动、焦点和布局通过已有浏览器设施验证。

## Answer

前端实现与验证完成（2026-10-15，分支 `feat/channel-priority-table`）。

### 最终修复：自定义 renderRow 丢失行样式

Standards 复查发现 `DataTableView` 的 `renderRow` 路径不经过 `getRowClassName`，渠道表为此在 renderRow 硬编码 `DISABLED_ROW_DESKTOP`，于是 `getRowClassName` 按 `isMobile` 选择的 `DISABLED_ROW_MOBILE` 永远到不了行。

- `8cc3fce46`：`DataTableRenderRowHelpers` 增加 `className`（`tableBodyRowClassName` 与 `getRowClassName` 的合成值），`data-table-view.tsx` 让默认行与自定义 renderRow 共用同一解析；`ChannelPriorityRow` 改用 `helpers.className`，删掉硬编码。
- 回归测试 `web/src/components/data-table/core/__tests__/row-rendering.test.tsx`：默认 renderer 与自定义 renderer 两条路径都保留 `min-h-12`、disabled 背景与 `data-state=selected`。修复前自定义路径红（tools/frontend-row-style-red.log），修复后绿（tools/frontend-row-style-green.log）。
- 桌面输出保持不变（`DISABLED_ROW_DESKTOP` 等于 `getRowClassName(row,{isMobile:false})`），移动端恢复由该回调选择的 `DISABLED_ROW_MOBILE`。

### 复用缺口（记录）

现有 Auto group 拖动为列表专用：`web/src/components/auto-group-order-item.tsx` 的 `Reorder.Item` 固定渲染列表卡片，props 仅 `group/index/onMove/onRemove`，不能输出表格 `tr`/`td` 或保留渠道表单元格；`auto-group-order-editor.tsx`、`group-ratio-visual-editor.tsx` 同为列表布局。因此本特性组合 Motion 公共原语 `Reorder.Group`/`Reorder.Item`/`useDragControls` 与共享 data-table 的 `renderRow`/`DataTableRow.render` 插槽，保留既有单元格、行选择与行内编辑，未新建通用拖动框架。CodeGraph（原索引 `/home/user/0github/nice-api`）确认 `renderTableBodyContent → renderRow`，且 core `getRowClassName` 仅被 `data-table-view.tsx` 调用，印证该绕过路径。

### 命令与结果（workdir `web/`；日志在 `tools/`）

- `bun x vitest run src/features/channels src/components/data-table src/features/keys --maxWorkers=2` → 36 files / 407 tests 通过（`frontend-affected-postfix.log`，退出码 0）。
- `bun x vitest run src/components/data-table/core/__tests__/row-rendering.test.tsx` → 2 passed（`frontend-row-style-green.log`）。
- `bun run typecheck`（tsgo -b）→ exit 0（`frontend-typecheck-postfix.log`）。
- `bun run build`（rsbuild 2.1.6）→ exit 0（`frontend-build-postfix.log`）。
- `node tools/verify-frontend-i18n.mjs` → 7 locales PASS，各 33 源键、22 条特性串、占位符完整、非 en 均已翻译（`frontend-i18n-verify-postfix.log`）。
- `bun run i18n:sync` → 各 locale missing 0（`frontend-i18n-sync-postfix.log`）。
- `bun run lint`（oxlint）→ exit 1，165 条改动前既有错误，均不在本次改动文件；本次改动文件无 error（`frontend-lint-postfix.log`）。
- `bun run format:check`（oxfmt 0.57.0）→ exit 1，20 个改动前既有文件；本次改动文件已不在其中（`frontend-format-postfix2.log`）。

关键版本：vitest 4.1.10、oxfmt 0.57.0、React 19.2.7、Rsbuild 2.1.6、Node v22。

### 浏览器证据

`tools/browser-evidence-final.log`（本次行样式修复前采集）：指针乐观保存、焦点保持、每行 `tr`/每格 `td`、computed table/table-row；原位、越界落点、Escape 均无 POST；滚动后保留焦点。该证据未覆盖移动端禁用行样式；桌面输出在修复前后一致，移动端样式由上述组件回归测试覆盖，故按要求未重复浏览器运行。

### 真实状态

ticket02 前端验收项已满足，由组件/集成测试、类型检查、构建、i18n 校验与既有浏览器证据支撑。三数据库完整矩阵与合并后集成审查属 ticket03；本工单不声称已通过三项数据库矩阵或最终集成审查。

# 02: 渠道表格页内拖动与保存反馈

Type: task
Status: claimed
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

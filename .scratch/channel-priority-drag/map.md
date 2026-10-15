# Channel priority drag

Spec: [spec.md](spec.md)
Integration branch: integration/channel-priority-drag
Base commit: 51205165eda31ca49cc1962f8344d6a54ba57f48

## Notes

本次 implement-spec 请求授权实施。规格 Out of Scope 的“本轮实现”描述原规格编写轮次，不限制本次实施。
本地 tracker 已配置；原规格无实施工单，本次按规格拆分。
原工作区未提交 GLOSSARY.md、.pi/ 及规格保持原样。实施在隔离检出中进行，最终将集成分支带回原仓库。临时目录曾被环境重启清理，现已恢复到持久隔离目录 /home/user/0github/nice-api-channel-priority。

## Decisions-so-far

测试入口采用规格 Testing Decisions：真实控制器/API、数据库与选路；真实表格及 API 边界；浏览器指针验证。

## Fog

需要真实三引擎验收，未验证不能关闭最终工单。接口契约已选定：完整列表提供 data.order_snapshot；重排 POST 接收期望快照与页内 moved_id/target_id；前后端可按 API 边界并行实施。

## Tickets

- [01 原子全局优先级 API](issues/01-atomic-priority-api.md)
- [02 可访问页内拖动](issues/02-accessible-page-reorder.md), independent after API contract selection
- [03 验证与审查](issues/03-verify-and-review.md), blocked by 01, 02

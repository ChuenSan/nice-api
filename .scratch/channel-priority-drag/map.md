# Channel priority drag

Spec: [spec.md](spec.md)
Integration branch: integration/channel-priority-drag
Base commit: 51205165eda31ca49cc1962f8344d6a54ba57f48

## Notes

本次 implement-spec 请求授权实施。规格 Out of Scope 的“本轮实现”描述原规格编写轮次，不限制本次实施。
本地 tracker 已配置；原规格无实施工单，本次按规格拆分。
原工作区未提交 GLOSSARY.md、.pi/ 及规格保持原样。实施在隔离检出中进行，最终将集成分支带回原仓库。

## Decisions-so-far

测试入口采用规格 Testing Decisions：真实控制器/API、数据库与选路；真实表格及 API 边界；浏览器指针验证。

## Fog

数据库实例、并发协调与缓存一致性方案待研究。需要真实三引擎验收，未验证不能关闭最终工单。

## Tickets

- [01 原子全局优先级 API](issues/01-atomic-priority-api.md)
- [02 可访问页内拖动](issues/02-accessible-page-reorder.md), blocked by 01
- [03 验证与审查](issues/03-verify-and-review.md), blocked by 01, 02

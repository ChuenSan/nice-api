# 01: 原子全局优先级保存与并发保护

Type: task
Status: claimed
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

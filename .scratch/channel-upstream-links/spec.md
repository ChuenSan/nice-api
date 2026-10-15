# 渠道名称链接到上游地址

状态：实现完成，自动化验证通过；待真实浏览器验收

## 用户原始需求

> 我需要给所有渠道的前端展示加一个超链接。比如前端展示的名称是“123”，我希望这个名称可以通过点击的方式，直接到达上游链接。这个链接就是“123”这个渠道里配置的请求地址。

用户在逐项讨论后确认：仅覆盖渠道管理列表；Q7–Q12 全部采用推荐方案。Q3 最终确认需要隐藏后台管理地址。

## 术语

- 渠道：管理后台配置的一个上游连接条目。
- 上游地址（Upstream Base URL）：渠道配置的 `base_url`，即请求地址的基础部分；不是拼接后的模型请求端点。

## 已确认的行为

1. 范围为管理员渠道管理列表中的每条渠道，包括桌面表格与移动端卡片。
2. 名称本身作为链接文字，例如“123”。链接目标原样使用该渠道配置的 `base_url`，不拼接 `/v1/chat/completions` 等请求路径。
3. 配置为空时，使用该渠道类型的默认上游地址；没有默认地址时显示普通文字。默认地址加载失败时也保留名称展示，已配置的地址仍可点击。
4. 仅有效的 `http://` 和 `https://` 地址可以生成链接。非网址关键字、带占位符的地址、格式错误和其他协议保持普通文字。明确无效的已配置值不会改用类型默认地址。
5. 点击在当前标签页打开。通过 `rel="noreferrer"` 隐藏来源地址，不能向上游发送后台地址作为 Referer。
6. 链接只用于现有管理员渠道列表，不向普通用户新增上游地址数据或入口。
7. 隐藏敏感信息时，名称显示为 `••••`，同时移除链接，鼠标悬停不能通过链接看到上游地址。
8. 标签聚合行保持展开/折叠行为，不添加链接；展开后的各条渠道按上述规则处理。
9. 使用日志、任务日志、仪表盘、渠道测试/余额查询/获取模型等弹窗不在本次范围。
10. 默认保留现有名称样式，悬停时出现下划线和手型光标，不新增图标。键盘可以聚焦链接，并保留清晰的焦点提示。
11. 保留已有长名称截断与名称提示能力。

## 实现约束与代码依据

- `web/src/features/channels/components/channels-columns.tsx`：渠道名称列及标签聚合分支。
- `web/src/features/channels/components/channel-card.tsx`：卡片复用名称列渲染，因此共享链接行为。
- `web/src/features/channels/components/channels-table.tsx`：列表统一加载默认地址，传入列配置；避免单独渲染其他列时发起额外请求。
- `web/src/features/channels/api.ts` 的 `getChannelDefaultBaseURLs` 及现有查询键：复用 `/api/channel/default_base_urls`，与编辑面板共享缓存。
- `TruncatedText` 和 `TruncatedCell`：继续组合现有截断与提示能力。`LearnMore` 用于说明弹层，不能承载名称导航；本次使用功能内组件组合原生语义链接，不新建公共链接组件。
- 现有渠道列表接口已有 `base_url`，本次不改接口、数据库、计费或认证逻辑。

## 验收条件

- 配置有效 HTTP(S) 地址：名称有正确链接，配置优先于默认值；当前页打开且禁止发送 Referer。
- 未配置地址：有类型默认值时链接到默认值；无默认值或查询失败时为普通文字。
- 非 HTTP(S)、关键字、占位符及格式错误：保持普通文字，不生成危险跳转。
- 敏感信息从显示切换为隐藏后，名称及链接目标均不再出现在该名称展示中。
- 标签聚合行无链接，桌面与移动端名称使用相同规则。
- 长名称仍截断并可查看名称提示；可通过键盘聚焦和激活链接。
- 相关组件与列表回归测试、TypeScript 检查、涉及文件 lint、格式检查及生产构建通过；记录实际执行结果与未验证项。

## 验证记录

以下命令均从 `web/` 执行（本次 Bun 可执行文件位于 `/tmp/nice-api-tools/node_modules/.bin/bun`）：

- `bun run test src/features/channels/components/__tests__/channel-name-link.test.tsx src/features/channels/components/__tests__/channels-columns-status-tooltip-content.test.tsx src/features/channels/components/__tests__/table-refresh.test.tsx`：3 个测试文件、18 条测试全部通过。
- `bun run typecheck`：通过，无 TypeScript 错误。
- `bun x oxlint -c .oxlintrc.json src/features/channels/components/channels-columns.tsx src/features/channels/components/channels-table.tsx src/features/channels/components/channel-name-cell.tsx src/features/channels/components/__tests__/channel-name-link.test.tsx src/features/channels/components/__tests__/table-refresh.test.tsx`：通过，无输出。
- 对上述 5 个 TSX 文件使用 `oxfmt -c .oxfmtrc.json --stdin-filepath <文件>` 校验正文格式，并原样保留版权头：通过。
- `bun run build`：通过，Rsbuild 成功生成 `dist/`。
- 仓库根目录执行 `git -c safe.directory=/home/user/0github/nice-api diff --check`：通过。

新增组件测试覆盖配置地址优先、HTTP/HTTPS、默认地址降级、非法或特殊地址、无地址、隐藏后移除链接、键盘聚焦与无 Referer 属性；列表集成测试覆盖默认地址接入及隐藏/显示切换。

曾因把默认地址请求放进列定义 Hook 而影响原有状态提示框测试；请求已移至列表组件，原测试与新测试最终全部通过。

未执行真实浏览器和真实上游的访问验收：移动端实际视觉效果、浏览器实际跳转以及真实请求头仍需在运行中的管理后台验收。移动端卡片复用同一名称列，未单独新增移动端浏览器测试。

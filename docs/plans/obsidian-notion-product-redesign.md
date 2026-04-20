# Obsidian + Notion 风格协同文档产品重定位计划

> **给 AI 接力者：** 后续不要直接跳到实现。先按本计划逐步拆任务、确认边界，再改代码。当前产品主线是“协同文档工作台”，待办只是轻量辅助模块。

**目标：** 将现有 ToDoList 重定位为类 Notion + Obsidian 的文档工作台，支持空间化文档、Obsidian 风格个人日记、Notion 风格协作会议纪要和轻量待办模块。

**架构：** 短期复用现有 `projects/tasks/content_md` 作为兼容底座，但产品语义改为“空间 / 文档 / 文档正文”。协同文档和会议纪要继续使用 Yjs；日记默认使用普通 Markdown 保存，不启用多人协同；待办保留为轻量模块。

**技术栈：** Go、Gin、MySQL、Redis、WebSocket、Yjs、React、Vite。

---

## 1. 产品重新定位

### 1.1 一句话定位

一个面向个人和小团队的协同文档工作台：像 Notion 一样组织空间、文档和会议纪要，像 Obsidian 一样沉淀个人 Markdown 日记，并保留轻量待办来承接行动项。

### 1.2 主卖点

- 协同文档：多人实时编辑 Markdown 文档。
- 会议纪要：Notion 风格会议模板，支持多人协作，后续扩展参会人、议题、行动项。
- 日记：Obsidian 风格日记，按日期自动创建 `.md` 标题的个人日记，默认非协作。
- 空间：以空间组织文档、会议和日记。

### 1.3 非主卖点

- 待办模块只是轻量辅助能力。
- “今日 / 未来 7 天 / 日历”作为待办模块的次级视图保留。
- 不把产品包装成复杂任务管理器。
- 不优先做甘特图、复杂工作流、多级任务依赖。

## 2. 信息架构

### 2.1 顶级导航

建议主导航顺序：

1. `空间`
2. `日记`
3. `会议`
4. `待办`
5. `搜索`

其中 `待办` 不作为视觉中心，只作为轻量模块入口。

### 2.2 空间与文档

- `Project` 在用户界面中逐步改名为“空间”。
- `Task` 在用户界面中逐步改名为“文档”。
- `TaskDetailPanel` 的产品语义改为“文档编辑面板”。
- `ProjectDetailPage` 的产品语义改为“空间详情页”。

短期不要求立即改文件名，先改用户可见文案和 plan。

### 2.3 文档类型

- `doc`
  - 普通协同文档。
  - 默认启用 Yjs 协作。

- `diary`
  - Obsidian 风格日记。
  - 标题格式 `YYYY-MM-DD.md`。
  - 默认归属 “日记” 空间。
  - 默认非协作。

- `meeting`
  - Notion 风格会议纪要。
  - 默认启用 Yjs 协作。
  - 默认使用会议模板。

- `todo`
  - 轻量待办。
  - 可由会议行动项生成。
  - 不是主产品对象。

## 3. 视觉设计方向

### 3.1 总体风格

方向：Obsidian 的深色知识库壳层 + Notion 的清爽文档画布。

- 左侧导航接近 Obsidian：深色、紧凑、知识库结构感。
- 主内容区接近 Notion：浅色或纸张感画布、大留白、文档卡片、轻边框。
- 编辑器区域强调 Markdown 文档，不强调任务状态。
- 待办模块视觉上更轻，不抢主导航。

### 3.2 颜色

建议 CSS token：

- 侧边栏背景：`#191820`, `#211f2a`
- 画布背景：`#f7f4ec`
- 纸张表面：`#fffdf7`
- 主文本：`#26221c`
- 辅助文本：`#7c7468`
- 琥珀强调色：`#d7a85b`
- 蓝灰强调色：`#7d8ca3`
- 边框：`rgba(38, 34, 28, 0.12)`

避免继续使用“温暖待办看板”的大面积米黄色卡片。

### 3.3 字体

建议使用更像知识工作台的字体组合：

- 界面字体：`IBM Plex Sans`
- Markdown 正文字体：`Literata` 或 `Noto Serif SC`
- 代码字体：`JetBrains Mono`

如果外部字体加载不稳定，保留本地 fallback，但视觉 token 要先定。

### 3.4 交互氛围

- 页面切换轻微 fade/slide。
- 文档卡片 hover 时仅轻微抬升，不做强按钮化。
- 编辑器 Markdown 区域要像纸张或文档页；协同正文编辑器推荐使用 CodeMirror 6 + Yjs 绑定，不继续用原生 textarea 做整篇文本替换。
- 协同状态使用小型 pill，不打断阅读。

## 4. 功能规划

### 4.1 日记

目标：Obsidian 风格日记。

用户流程：

1. 点击 `日记`。
2. 点击 `新建日记`。
3. 系统查找或创建 “日记” 空间。
4. 系统按当前日期查找 `YYYY-MM-DD.md`。
5. 已存在则打开，不存在则创建。
6. 打开后进入非协作文档编辑。

实现原则：

- 日记是系统内文档记录，不是磁盘真实文件。
- 标题像文件名：`2026-04-19.md`。
- 日记正文是 Markdown。
- 日记不需要多人协同。
- 后续可以做日历、标签、双链、反向链接。

### 4.2 会议纪要

目标：Notion 风格会议纪要。

用户流程：

1. 在空间内点击 `新建会议纪要`。
2. 系统创建会议纪要文档。
3. 默认标题：`会议纪要 YYYY-MM-DD`，后续可用户输入。
4. 默认正文模板：

```markdown
# 会议纪要

## 参会人

## 议题

## 结论

## 行动项
```

实现原则：

- 会议可以协作。
- 会议正文走现有 Yjs。
- 后续扩展结构化字段：参会人、会议时间、议题、行动项。
- 行动项后续可以生成轻量待办。

### 4.3 待办

目标：轻量待办模块。

用户流程：

1. 用户进入 `待办`。
2. 创建待办事项。
3. 设置状态、截止时间、负责人。
4. 可以从会议行动项生成待办。

实现原则：

- 不作为主卖点。
- 不优先复杂任务管理。
- 现有“今日 / 未来 7 天 / 日历”可以归到“待办”下。
- 保留基础 CRUD 即可。

### 4.4 协同正文编辑器推荐方案

目标：把当前正文编辑区从“原生 textarea + 整篇文本替换”升级为“CodeMirror 6 + Yjs 绑定”的 Markdown 协同编辑器，解决两个人同时编辑同一块区域时可能出现的整篇重复、异常覆盖或快照抖动问题。

推荐方案：

- 使用 CodeMirror 6 承载 Markdown 编辑体验。
- 使用 Yjs 绑定把 CodeMirror 的本地输入映射成细粒度 Y.Text 操作。
- 保留现有正文 WebSocket 协议、`task_content_updates` update log、Redis Pub/Sub fan-out 和断线补偿链路。
- 将 `YjsTaskContentProvider` 收敛为“Y.Doc + WebSocket 同步 provider”，不要再由它直接执行整篇 `delete + insert`。
- 新增 `DocumentMarkdownEditor` 组件，由编辑器组件负责把 CodeMirror 文档和 Y.Text 绑定起来。
- `content_snapshot` 只作为渲染缓存或列表预览缓存，不作为正文最终权威；正文权威仍然是 Yjs update log 或由服务端/后台任务应用 update 后生成的快照。

并发编辑语义：

- 两个人同时编辑同一段文字时，Yjs 负责最终收敛，不由后端自研 CRDT/OT。
- 两个人在同一位置同时插入内容时，两段内容都应保留，顺序由 CRDT 规则确定。
- 一个人删除、另一个人在附近插入时，系统应保留可合并的插入内容，但用户可能仍需要人工整理语义。
- 系统目标是“不丢 update、不静默覆盖、最终一致”，不是自动判断人的真实编辑意图。

后续增强：

- 增加 Yjs awareness，用于显示文档内光标、选区和正在编辑位置。
- 在同一段落或同一标题块多人编辑时显示轻提示，降低同一区域编辑冲突。
- 日记可以复用 CodeMirror 作为本地 Markdown 编辑器，但默认不接入 Yjs 绑定，不进入多人正文协同。

### 4.5 评论功能

当前状态：当前代码中没有业务级评论模型、评论 API 或评论 UI。后续需要把评论作为文档协作能力补齐，但不要给日记板块开启评论。

目标：为普通协同文档、会议纪要和轻量待办增加评论能力，让成员可以围绕文档内容、会议结论和待办行动项进行异步讨论；日记默认是个人 Markdown 记录，不显示评论入口，也不允许创建评论。

产品范围：

- 普通文档支持评论。
- 会议纪要支持评论。
- 待办详情支持评论。
- 日记不支持评论，避免把个人日记变成协作讨论区。

第一阶段建议先做文档级评论：

- 评论挂在整篇文档或待办详情下。
- 支持创建评论、查看评论列表、删除自己的评论。
- 支持基础回复线程，或者先用 `parent_id` 预留回复能力。
- 支持 `resolved` 状态，方便会议纪要和文档评审关闭讨论。
- 评论内容使用普通 Markdown 文本，不进入 Yjs 正文协同。

第二阶段再做行内评论：

- 允许评论绑定到正文选区、标题块或段落锚点。
- 行内评论需要依赖稳定的文档锚点策略，不能只保存临时字符 offset。
- 行内评论与 Yjs 正文协同要解耦：正文 update 继续走 Yjs，评论 CRUD 走 HTTP + 事件通知。

建议数据模型：

- 短期可以新增 `task_comments`，沿用当前 `tasks` 作为文档底座。
- 中期迁移后可重命名为 `document_comments`。
- 字段建议包括：
  - `id`
  - `project_id`
  - `task_id`
  - `parent_id`
  - `user_id`
  - `content_md`
  - `anchor_type`
  - `anchor_payload`
  - `resolved`
  - `resolved_by`
  - `resolved_at`
  - `created_at`
  - `updated_at`
  - `deleted_at`

建议 API：

- `GET /api/v1/documents/:id/comments`
  - 获取文档评论列表。

- `POST /api/v1/documents/:id/comments`
  - 创建文档评论。

- `PATCH /api/v1/comments/:id`
  - 修改评论正文或切换 resolved 状态。

- `DELETE /api/v1/comments/:id`
  - 删除评论，优先软删除。

权限规则：

- 普通文档和会议纪要评论沿用空间成员权限。
- 待办评论沿用待办可见性和成员权限。
- 日记评论接口必须拒绝，前端也不展示评论入口。
- 评论作者可以删除或编辑自己的评论；空间 owner/editor 可以管理评论状态。

实时与异步规则：

- 评论创建、更新、删除可以通过项目级 WebSocket 广播轻量事件，例如 `COMMENT_CREATED`、`COMMENT_UPDATED`、`COMMENT_DELETED`。
- 评论不进入 Yjs 正文 update log。
- Kafka 可用于评论通知、邮件提醒、未读聚合等可靠异步副作用，但不作为评论实时广播链路。

### 4.6 回收站模块

目标：把“删除文档 / 删除会议纪要 / 删除待办 / 删除空间”从不可逆硬删除改成可恢复的软删除流程，降低误删风险，并为协同工作台提供更符合文档产品的删除体验。

产品范围：

- 普通文档进入回收站。
- 会议纪要进入回收站。
- 待办进入回收站。
- 空间删除可以先进入回收站，空间内文档随空间一起隐藏。
- 日记是否进入回收站需要单独控制：默认可以进入个人回收站，但不进入团队协作回收站，也不展示评论恢复链路。

第一阶段建议：

- 删除操作改为软删除，写入 `deleted_at`、`deleted_by`。
- 默认列表、搜索、同步接口不返回已删除对象。
- 新增回收站页面，按类型展示已删除内容。
- 支持恢复。
- 支持彻底删除。
- 彻底删除需要二次确认。

建议数据模型：

- 在 `tasks` 上新增：
  - `deleted_at`
  - `deleted_by`
  - `delete_reason`

- 在 `projects` 上新增：
  - `deleted_at`
  - `deleted_by`

- 评论如果启用软删除，也应保留 `deleted_at`。

建议 API：

- `GET /api/v1/trash`
  - 获取当前用户可见的回收站条目。

- `POST /api/v1/trash/:type/:id/restore`
  - 恢复空间、文档、会议纪要或待办。

- `DELETE /api/v1/trash/:type/:id`
  - 彻底删除回收站条目。

权限规则：

- 用户只能看到自己有权限恢复或彻底删除的条目。
- 空间 owner 可以恢复或彻底删除空间内文档。
- 普通 editor 可以删除文档，但是否允许彻底删除需要由空间权限策略决定。
- 日记回收站条目默认只有 owner 可见和恢复。

实时与事件规则：

- 删除、恢复、彻底删除都应写事件日志，便于 Sync API 补偿。
- 项目级 WebSocket 应广播删除和恢复事件，让列表实时隐藏或恢复条目。
- Kafka 可用于彻底删除后的异步资源清理，例如附件、头像或后续对象存储文件，不参与实时删除广播。

## 5. 数据与 API 规划

### 5.1 短期复用

继续复用：

- `projects` -> spaces
- `tasks` -> documents/todos
- `task_events` -> document events
- `task_content_updates` -> document content updates
- 后续新增 `task_comments` -> document comments
- 后续新增 `deleted_at` / `deleted_by` -> trash state

### 5.2 推荐新增字段

在 `tasks` 上新增：

- `doc_type`
  - `doc`
  - `diary`
  - `meeting`
  - `todo`

- `collaboration_mode`
  - `yjs`
  - `plain`

- `template_key`
  - `blank`
  - `diary`
  - `meeting_minutes`
  - `todo`

删除与回收站字段：

- `deleted_at`
- `deleted_by`
- `delete_reason`

最小实现可以先只加 `doc_type`；如果先做回收站，则至少需要补 `deleted_at` 和 `deleted_by`。

### 5.3 API 规划

建议新增产品语义 API：

- `POST /api/v1/documents`
  - 创建普通文档。

- `POST /api/v1/diary/today`
  - 幂等创建或打开今天日记。

- `POST /api/v1/meetings`
  - 创建会议纪要。

- `POST /api/v1/todos`
  - 创建轻量待办。

- `PATCH /api/v1/documents/:id/content`
  - 保存非协作文档正文，主要用于 diary。

- `GET /api/v1/documents/:id/comments`
  - 获取文档评论，日记类型应拒绝。

- `POST /api/v1/documents/:id/comments`
  - 创建文档评论，日记类型应拒绝。

- `GET /api/v1/trash`
  - 获取回收站条目。

- `POST /api/v1/trash/:type/:id/restore`
  - 恢复回收站条目。

短期可以保留旧 task API，并逐步加别名。

## 6. 分阶段实施

### 阶段 1：只沉淀产品定位和视觉计划

范围：

- 更新 `docs/plans/finallist.md`。
- 新增本计划文档。
- 同步 `AGENTS.md`、`README.md`、`docs/TODO.md`。

不做：

- 不新增接口。
- 不新增数据库字段。
- 不大改前端。

验收：

- 文档明确主线是协同文档，不是到期任务。
- 日记、会议、待办三者定位清晰。

### 阶段 2：前端信息架构和视觉改造

范围：

- 更新全局 CSS token。
- 重做 `AppLayout` 侧边栏。
- 将用户可见的“Lists”改为“空间”。
- 将项目页主文案改为文档空间。
- 将任务详情面板文案改为文档编辑器。
- 将“今日 / 未来 7 天 / 日历”收进“待办”语义下。
- 将正文编辑区从原生 textarea 替换为 CodeMirror 6 + Yjs 绑定，避免协同输入继续走整篇文本替换。

文件：

- `web/src/index.css`
- `web/src/layouts/AppLayout.jsx`
- `web/src/layouts/AppLayout.css`
- `web/src/pages/ProjectListPage.jsx`
- `web/src/pages/ProjectListPage.css`
- `web/src/pages/ProjectDetailPage.jsx`
- `web/src/pages/ProjectDetailPage.css`
- `web/src/components/TaskDetailPanel.jsx`
- `web/src/components/TaskDetailPanel.css`
- `web/src/components/DocumentMarkdownEditor.jsx`
- `web/src/components/DocumentMarkdownEditor.css`
- `web/src/realtime/yjsTaskContentProvider.js`
- `web/src/components/TaskSection.jsx`
- `web/src/components/TaskSection.css`

验收：

- 页面视觉接近 Obsidian + Notion。
- 用户看到的是“空间 / 文档 / 日记 / 会议 / 待办”。
- 待办入口是次级模块。
- 两个用户同时编辑同一文档同一段落时，不应出现整篇正文重复插入或整篇快照互相覆盖。
- 普通文档和会议纪要正文继续走现有正文 WebSocket、Yjs update log 和 Redis Pub/Sub fan-out。

### 阶段 3：日记最小功能

范围：

- 后端实现 `POST /api/v1/diary/today`。
- 自动查找/创建 “日记” 空间。
- 同日幂等打开 `YYYY-MM-DD.md`。
- 日记正文使用 plain Markdown 保存。

验收：

- 同一天不会重复创建日记。
- 日记不启用多人协同 UI。
- 日记可保存 Markdown。

### 阶段 4：会议纪要最小功能

范围：

- 后端实现会议纪要创建接口，或前端先复用文档创建接口带模板。
- 默认启用 Yjs 协作。
- 默认插入会议模板。

验收：

- 可以创建会议纪要。
- 多人可以协作编辑。
- 有基础模板。

### 阶段 5：待办轻量模块整理

范围：

- 将现有待办视图收敛到“待办”。
- 保留创建、完成、删除、截止时间。
- 暂不做复杂任务管理。

验收：

- 用户可以创建待办事项。
- 待办模块不抢占文档主路径。

### 阶段 6：评论功能最小实现

范围：

- 新增评论数据模型。
- 新增文档评论 API。
- 前端在普通文档、会议纪要和待办详情中展示评论面板。
- 日记不展示评论入口，后端也拒绝日记评论创建。
- 评论暂时做文档级评论，不做行内评论。

验收：

- 普通文档可以创建、查看、删除评论。
- 会议纪要可以创建、查看、删除评论。
- 待办详情可以创建、查看、删除评论。
- 日记没有评论入口，也不能通过 API 创建评论。
- 评论不影响 Yjs 正文协同。

### 阶段 7：回收站模块

范围：

- 将文档、会议纪要、待办和空间删除改为软删除。
- 新增回收站页面。
- 新增恢复接口。
- 新增彻底删除接口。
- 删除、恢复和彻底删除写入事件日志，用于同步补偿。

验收：

- 删除后的文档不会出现在默认空间列表。
- 回收站可以看到有权限的已删除条目。
- 用户可以恢复误删文档。
- 用户可以彻底删除条目，且彻底删除需要二次确认。
- 恢复操作可以通过项目事件同步到其他在线客户端。

## 7. 当前注意事项

- 不要直接大规模重命名数据库表。
- 不要把 diary 接入多人 Yjs 协同。
- 不要给 diary 开启评论功能。
- meeting 可以协作。
- todo 是小模块，不是主卖点。
- 前端 redesign 要先做视觉和信息架构，不要先新增复杂功能。
- Swagger 仍需在新增 API 后统一重生成。

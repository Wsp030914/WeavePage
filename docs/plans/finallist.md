# 协同文档工作台产品与实施总计划

> **给 AI 接力者：** 后续实现必须按本文档的产品定位和优先级执行。除非用户明确要求恢复待办系统方向，否则不要再把“到期任务”当作主线。

**产品定位：** 将当前 ToDoList 重定位为类 Notion 的协同文档工作台。用户以项目/空间组织文档，可以创建普通协同文档、Obsidian 风格日记、Notion 风格会议纪要，也可以在轻量待办模块里创建待办事项。普通文档和会议纪要以多人实时协同编辑为核心；日记是个人 Markdown 知识沉淀，默认不参与多人协同；待办只是辅助模块和会议行动项承接能力，不作为主要卖点。

**核心价值：** 不是单一“管理到期任务”，而是“围绕文档、日记、会议和待办形成个人/团队知识与行动工作台，并保证协同文档的实时编辑、断线恢复、权限和多节点一致性”。

**当前兼容实现：** 短期继续复用现有 `projects` 和 `tasks` 表：

- `projects` 在产品语义上逐步改称“空间 / 文档集”。
- `tasks` 在产品语义上逐步改称“文档 / 页面”。
- `tasks.title` 是文档标题，例如 `2026-04-19.md`。
- `tasks.content_md` 是 Markdown 文档渲染缓存；普通文档和会议纪要的权威正文来自 Yjs update log / snapshot。
- `status`、`priority`、`due_at` 暂作为兼容字段保留，不作为新产品主线能力。

**目标架构：** HTTP 继续作为文档元数据的权威写入通道；文档正文走 Yjs CRDT update，通过 WebSocket 传输，通过 Redis Pub/Sub 做多节点 fan-out，通过 MySQL update log / snapshot 做断线恢复。文档元数据继续使用 MySQL 版本号 CAS、事件日志和 Sync API。Kafka 保留给通知、邮件、提醒等可靠异步副作用，不作为实时广播主链路。

**技术栈：** Go、Gin、MySQL、Redis、Redis Pub/Sub、Kafka、React、Vite、WebSocket、Yjs。

---

## 1. 新产品信息架构

### 1.1 一级概念

- **空间 / 文档集**
  - 当前底层实体：`projects`
  - 示例：工作区、团队项目、日记、会议
  - 负责组织文档、成员、权限和实时 room

- **文档 / 页面**
  - 当前底层实体：`tasks`
  - 示例：产品方案、接口说明、会议纪要、日报、个人日记
  - 元数据走 HTTP + CAS
  - 正文按文档类型决定是否启用协同

- **协同正文**
  - 当前底层实体：`task_content_updates`
  - 普通文档和会议纪要默认启用
  - 日记默认不启用多人协同，可以先用普通 Markdown 保存路径，后续需要时再升级

### 1.2 文档类型

短期建议新增文档类型语义，底层可以先通过字段、标题约定或 project 固定归属实现：

- `doc`
  - 普通协同文档
  - 默认启用 Yjs 协同
  - 归属用户选择的空间

- `diary`
  - Obsidian 风格日记
  - 点击“新建日记”时自动创建当天标题，例如 `2026-04-19.md`
  - 自动归属到名为“日记”的空间；如果不存在则自动创建
  - 默认不启用多人协同
  - 同一天重复点击应打开当天已有日记，而不是重复创建
  - Markdown-first，强调个人记录、知识沉淀、日期导航
  - 后续可扩展标签、双链、反向链接、全文搜索

- `meeting`
  - Notion 风格会议纪要
  - 点击“新建会议纪要”创建会议文档
  - 默认启用协同，方便多人共同记录
  - 初期只创建基础 Markdown 文档和模板；后续再扩展参会人、议题、行动项、会议时间、关联文档等结构化能力

- `todo`
  - 轻量待办事项
  - 用户可以手动创建待办
  - 可被会议纪要行动项引用或生成
  - 保留状态、截止时间、负责人等基础能力
  - 不作为产品主要卖点，不优先做复杂任务管理

### 1.3 旧待办能力的定位

- 待办模块保留，但只是轻量辅助能力。
- 到期提醒、今日任务、未来 7 天、日历不再是主线产品卖点。
- 这些页面可以保留为待办模块的辅助视图，但不要优先投入复杂任务管理能力。
- `due_at` 可以用于待办截止时间，也可以作为会议时间、文档提醒时间等可选元数据。
- 会议纪要里的行动项可以逐步与待办模块打通。

## 2. 当前已完成能力

### 2.1 实时协同底座

- `tasks.version + expected_version + CAS` 已落地，用于元数据并发保护。
- `task_events + Sync API` 已落地，用于项目/空间级断线补偿。
- `GET /api/v1/projects/:id/ws` 已落地，用于空间级事件广播、presence、metadata 锁。
- `GET /api/v1/tasks/:id/content/ws` 已落地，用于文档正文 Yjs update 同步。
- `task_content_updates` 已落地，用于正文 update log / snapshot。
- Redis Pub/Sub fan-out 已接入项目事件和正文 update。
- 前端 `ProjectDetailPage` 已接入空间级 WebSocket 事件流和本地 patch。
- 前端 `TaskDetailPanel` 已接入 Yjs provider，正文输入会实时保存。
- metadata 协同锁已落地，编辑标题、优先级、截止时间等元数据前会申请 `metadata` 锁。

### 2.2 需要重新命名但可复用的能力

- “项目详情页”应逐步改为“空间文档列表页”。
- “任务详情面板”应逐步改为“文档编辑面板”。
- `content_md` 应逐步在 UI 中称为“协同文档正文”。
- `TaskSection` 仍可作为文档列表分组组件复用，但 UI 文案应避免强调任务到期。

## 3. P0：产品重定位必须完成的功能

### 3.1 文档化 UI 改造

目标：让用户感知这是一个文档工作台，而不是任务列表。

需要改动：

- `web/src/pages/ProjectDetailPage.jsx`
  - 新建入口文案改为“新建文档”
  - 列表分组文案改为“文档”语义
  - 页面标题区域可以继续显示空间名称

- `web/src/components/TaskDetailPanel.jsx`
  - “Description”改为“Collaborative document”
  - “Body sync”改为“Document sync”
  - 保存提示改成“正文实时保存，Save 只保存元数据”

- `web/src/components/TaskSection.jsx`
  - 通用按钮仍可保留，但文案后续应从 `Task` 转为 `Doc`

验收：

- 主路径 UI 不再让用户以为核心功能是到期任务。
- 文档正文区域是页面主要编辑区域。

### 3.2 新建日记：Obsidian 风格日记

目标：一键创建当天个人 Markdown 日记，作为个人知识沉淀入口，而不是协同文档入口。

产品行为：

- 用户点击“新建日记”。
- 系统查找或创建名为“日记”的空间。
- 系统查找当天是否已有日记文档。
- 如果已有，直接打开该文档。
- 如果没有，创建标题为当天日期的 Markdown 文档，例如 `2026-04-19.md`。
- 日记归属“日记”空间。
- 日记默认不启用多人协同。
  - 日记体验接近 Obsidian 风格日记：
  - 标题/文件名使用日期
  - 默认 Markdown 编辑
  - 打开后直接聚焦正文
  - 后续支持日历导航、标签、双链和搜索

短期实现建议：

- 前端新增入口：
  - 侧边栏或首页按钮：`新建日记`
  - 点击后调用封装 API：`createOrOpenTodayDiary()`

- 后端新增接口：
  - `POST /api/v1/diary/today`
  - 行为：幂等创建/打开当天日记
  - 返回文档 snapshot 和所属空间

- 数据层策略：
  - 短期复用 `projects` 创建“日记”空间。
  - 短期复用 `tasks` 创建日记文档。
  - 推荐新增字段或兼容标记：
    - `tasks.doc_type = 'diary'`
    - 或在无法立即迁移时先用标题 + 专用 project 约束。

- 正文保存策略：
  - 日记不需要协同。
  - 第一阶段可以走普通 Markdown 保存接口，或继续复用现有内容字段的非协同保存路径。
  - 如果当前代码已停止普通 `content_md` 整段保存，需要为 diary 提供明确的非协同保存接口，例如：
    - `PATCH /api/v1/documents/:id/content`
    - 仅允许 owner 保存
    - 不写 Yjs update log

验收：

- 同一天多次点击“新建日记”只打开同一篇日记。
- 不同日期生成不同标题。
- 日记默认只对创建者可编辑。
- 日记正文保存不依赖多人 Yjs 协同。
- 日记入口不展示在线人数、协同锁等多人协同暗示。

### 3.3 新建会议纪要：Notion 风格会议纪要

目标：先提供最小会议纪要文档入口，默认支持多人协作，后续逐步扩展为 Notion 风格的结构化会议记录。

产品行为：

- 用户点击“新建会议纪要”。
- 系统创建一篇会议文档。
- 标题可以先使用：
  - `会议纪要 2026-04-19`
  - 或允许用户输入标题
- 默认归属当前空间；如果没有当前空间，可以归属名为“会议”的空间。
- 默认启用协同正文，方便多人共同记录。
  - 会议纪要体验接近 Notion 风格会议纪要：
  - 默认带会议模板
  - 支持多人协作
  - 后续支持结构化属性
  - 后续支持行动项和待办模块打通

短期实现建议：

- 前端新增入口：
  - 当前空间内：`新建会议纪要`
  - 全局快捷入口：可后续再做

- 后端可先复用现有创建文档接口：
  - 创建 `doc_type = 'meeting'`
  - 初始化 Markdown 模板：
    - `# 会议纪要`
    - `## 参会人`
    - `## 议题`
    - `## 结论`
    - `## 行动项`

- 会议正文使用现有 Yjs 协同通道。
- metadata 仍使用 HTTP + CAS + metadata 锁保护。

验收：

- 能创建会议纪要文档。
- 打开后可多人实时编辑。
- 后续可平滑扩展参会人、议题、行动项、会议时间等结构化字段。
- 会议纪要入口应明显区别于普通空白文档，默认带会议模板。

### 3.4 轻量待办模块

目标：保留用户可创建待办事项的能力，但不把待办作为产品主卖点。

产品行为：

- 用户可以创建待办事项。
- 待办可以设置标题、状态、截止时间、负责人。
- 待办可以从会议纪要行动项生成或关联。
- 待办列表、今日、未来 7 天、日历等视图作为辅助入口保留。

短期实现建议：

- 继续复用现有任务 CRUD 能力。
- UI 上将待办模块收敛到次级导航。
- 新产品主导航优先展示：
  - 文档
  - 日记
  - 会议
  - 待办

验收：

- 用户可以创建、完成、删除待办。
- 待办模块不影响协同文档主路径。
- 会议纪要后续可把行动项转为待办。

### 3.5 文档类型与权限语义

目标：补齐日记、会议、普通文档的差异。

建议策略：

- 新增文档类型字段：
  - `doc`
  - `diary`
  - `meeting`
  - `todo`

- 新增协同策略：
  - `doc`: enable collaboration
  - `meeting`: enable collaboration
  - `diary`: disable collaboration by default
  - `todo`: no document collaboration by default

- 权限：
  - 普通文档：沿用项目成员权限
  - 会议纪要：沿用项目成员权限
  - 日记：默认 owner 私有，除非后续显式分享
  - 待办：沿用个人或空间权限，作为轻量操作对象

验收：

- 前端可以根据文档类型选择协同 provider 或普通保存方式。
- 后端可以根据文档类型拒绝 diary 的多人正文 WebSocket 写入，或仅允许 owner 使用非协同保存接口。

## 4. P1：协同文档体验增强

### 4.1 文档列表与空间导航

- 将项目列表改造成空间列表。
- 空间内展示文档列表，而不是任务看板。
- 支持按文档类型过滤：
  - 全部
  - 普通文档
  - 日记
  - 会议纪要

### 4.2 协同正文编辑器推荐方案

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

### 4.3 文档内 presence

- 当前 presence 只是空间在线人数。
- 后续需要补：
  - 当前正在查看哪篇文档
  - 谁正在编辑正文
  - 光标/选区位置
  - 正在编辑元数据字段

### 4.4 会议纪要扩展

后续可扩展：

- 会议时间
- 参会人
- 议题列表
- 行动项
- 行动项负责人
- 行动项截止时间
- 从会议纪要生成轻量待办或提醒
- 会议纪要与相关文档互链

注意：这些是会议文档的增强，不应让系统重新变成“复杂任务管理系统”。

### 4.5 日记体验增强

后续可扩展：

- 日历式日记入口
- 上一篇/下一篇
- 日记模板
- Obsidian 风格双链
- 反向链接
- 标签
- 心情/标签
- 全文搜索

日记仍默认个人私有，不启用多人协同。

### 4.6 待办模块增强

待办模块只做轻量增强：

- 基础列表
- 今日/未来 7 天
- 简单日历视图
- 从会议行动项生成待办
- 文档内引用待办

不做复杂任务管理：

- 不做甘特图
- 不做复杂工作流
- 不做多级任务依赖
- 不把待办作为首页主卖点

### 4.7 评论功能

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

权限规则：

- 普通文档和会议纪要评论沿用空间成员权限。
- 待办评论沿用待办可见性和成员权限。
- 日记评论接口必须拒绝，前端也不展示评论入口。
- 评论作者可以删除或编辑自己的评论；空间 owner/editor 可以管理评论状态。

实时与异步规则：

- 评论创建、更新、删除可以通过项目级 WebSocket 广播轻量事件，例如 `COMMENT_CREATED`、`COMMENT_UPDATED`、`COMMENT_DELETED`。
- 评论不进入 Yjs 正文 update log。
- Kafka 可用于评论通知、邮件提醒、未读聚合等可靠异步副作用，但不作为评论实时广播链路。

### 4.8 回收站模块

目标：把“删除文档 / 删除会议纪要 / 删除待办 / 删除空间”从不可逆硬删除改成可恢复的软删除流程，降低误删风险，并为协同工作台提供更符合文档产品的删除体验。

产品范围：

- 普通文档进入回收站。
- 会议纪要进入回收站。
- 待办进入回收站。
- 空间删除可以先进入回收站，空间内文档随空间一起隐藏。
- 日记默认可以进入个人回收站，但不进入团队协作回收站，也不展示评论恢复链路。

第一阶段建议：

- 删除操作改为软删除，写入 `deleted_at`、`deleted_by`。
- 默认列表、搜索、同步接口不返回已删除对象。
- 新增回收站页面，按类型展示已删除内容。
- 支持恢复。
- 支持彻底删除。
- 彻底删除需要二次确认。

权限规则：

- 用户只能看到自己有权限恢复或彻底删除的条目。
- 空间 owner 可以恢复或彻底删除空间内文档。
- 普通 editor 可以删除文档，但是否允许彻底删除需要由空间权限策略决定。
- 日记回收站条目默认只有 owner 可见和恢复。

实时与事件规则：

- 删除、恢复、彻底删除都应写事件日志，便于 Sync API 补偿。
- 项目级 WebSocket 应广播删除和恢复事件，让列表实时隐藏或恢复条目。
- Kafka 可用于彻底删除后的异步资源清理，例如附件、头像或后续对象存储文件，不参与实时删除广播。

## 5. P2：分布式与工程能力

### 5.1 Redis 分布式限流

- 替换 `server/middlewares/ratelimit.go` 当前单机实现。
- 增加 HTTP、WebSocket 连接、WebSocket 消息级限流。
- key 建议：
  - `rl:http:{route}:ip:{ip}`
  - `rl:http:{route}:uid:{uid}`
  - `rl:ws:connect:{uid}`
  - `rl:ws:msg:{uid}:{type}`

### 5.2 分布式缓存击穿保护

- 将单机 `singleflight` 主逻辑替换为 Redis 锁 + 共享缓存等待。
- 重点检查：
  - `server/service/user_service.go`
  - `server/service/project_service.go`
  - `server/service/task_service.go`

### 5.3 可观测性

- 增加实时链路指标：
  - WebSocket 连接数
  - 文档房间数
  - Yjs update 入库耗时
  - Pub/Sub fan-out 延迟
  - 重连次数
  - Sync API 补偿事件数
  - 锁冲突次数
  - 限流命中次数

### 5.4 操作历史

- 复用 `task_events` 提供文档操作历史。
- 产品上叫“文档活动记录”。
- 可展示：
  - 创建文档
  - 修改标题
  - 删除文档
  - 会议纪要创建
  - 日记创建

正文逐字编辑历史不从 `task_events` 展示，避免噪声过大。

## 6. 数据模型演进建议

### 6.1 短期兼容

继续使用：

- `projects`
- `tasks`
- `task_events`
- `task_content_updates`
- 后续新增 `task_comments`

并通过 UI 和 service 层把它们解释成：

- `projects` -> spaces
- `tasks` -> documents
- `task_comments` -> document comments

### 6.2 中期重命名或新增模型

如果继续迭代，建议新增或迁移到更准确的模型：

- `spaces`
  - 替代 `projects`
- `documents`
  - 替代 `tasks`
- `document_events`
  - 替代 `task_events`
- `document_content_updates`
  - 替代 `task_content_updates`
- `document_comments`
  - 替代 `task_comments`

迁移不应阻塞当前功能实现。短期最重要的是产品语义和用户路径先对齐。

### 6.3 推荐新增字段

在当前 `tasks` 上推荐补：

- `doc_type`
  - `doc`
  - `diary`
  - `meeting`

- `collaboration_mode`
  - `yjs`
  - `plain`

- `template_key`
  - `blank`
  - `diary`
  - `meeting_minutes`
  - `todo`

- `deleted_at`
  - 回收站软删除时间

- `deleted_by`
  - 执行删除的用户

- `delete_reason`
  - 可选删除原因

如果不想一次性迁移，也可以先用最小字段：

- `doc_type`

如果先做回收站，则至少需要补：

- `deleted_at`
- `deleted_by`

### 6.4 推荐新增评论模型

短期可以新增 `task_comments`，沿用当前 `tasks` 作为文档底座。

字段建议包括：

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

## 7. API 规划

### 7.1 文档创建

短期可以继续复用：

- `POST /api/v1/tasks`

但建议新增产品语义接口：

- `POST /api/v1/documents`
  - 创建普通文档

- `POST /api/v1/diary/today`
  - 幂等创建或打开今天的日记

- `POST /api/v1/meetings`
  - 创建会议纪要

- `POST /api/v1/todos`
  - 创建轻量待办事项

### 7.2 文档正文

普通文档和会议纪要：

- `GET /api/v1/tasks/:id/content/ws`
  - 短期继续使用
  - 后续可新增别名：`GET /api/v1/documents/:id/content/ws`

日记：

- 不走多人协同。
- 建议新增：
  - `GET /api/v1/documents/:id/content`
  - `PATCH /api/v1/documents/:id/content`

### 7.3 同步接口

空间事件同步：

- 当前：`GET /api/v1/projects/:id/sync`
- 后续别名：`GET /api/v1/spaces/:id/sync`

### 7.4 评论接口

评论不进入 Yjs 正文协同，走普通 HTTP CRUD，并可通过项目级 WebSocket 发送轻量事件做实时提示。

- `GET /api/v1/documents/:id/comments`
  - 获取文档评论列表
  - 日记类型应拒绝或返回空能力声明

- `POST /api/v1/documents/:id/comments`
  - 创建文档评论
  - 日记类型应拒绝

- `PATCH /api/v1/comments/:id`
  - 修改评论正文或切换 resolved 状态

- `DELETE /api/v1/comments/:id`
  - 删除评论，优先软删除

### 7.5 回收站接口

- `GET /api/v1/trash`
  - 获取当前用户可见的回收站条目

- `POST /api/v1/trash/:type/:id/restore`
  - 恢复空间、文档、会议纪要或待办

- `DELETE /api/v1/trash/:type/:id`
  - 彻底删除回收站条目

## 8. 前端实施优先级

1. 完成 UI 语义切换：任务 -> 文档，项目 -> 空间。
2. 新增“新建日记”入口。
3. 实现 `createOrOpenTodayDiary()` 客户端封装。
4. 新增“新建会议纪要”入口。
5. 将 `TaskDetailPanel` 抽象成 `DocumentEditorPanel` 或至少完成用户可见文案切换。
6. 根据 `doc_type` 决定正文保存模式：
   - `doc` / `meeting`: Yjs 协同
   - `diary`: plain Markdown 保存
7. 将正文编辑区从原生 textarea 替换为 CodeMirror 6 + Yjs 绑定，避免协同输入继续走整篇文本替换。
8. 将 Today / Next 7 Days / Calendar 收敛到待办模块的次级视图。
9. 新增轻量待办入口，但不要抢占文档、日记、会议的主导航权重。
10. 新增评论面板，但日记详情不展示评论入口。
11. 新增回收站入口，支持查看、恢复和彻底删除有权限的条目。

## 9. 后端实施优先级

1. 增加文档类型字段或等价兼容方案。
2. 增加“日记”空间的查找/自动创建逻辑。
3. 增加 `POST /api/v1/diary/today`。
4. 增加 diary 非协同正文保存接口。
5. 增加会议纪要创建逻辑和默认模板。
6. 增加轻量待办创建接口或产品语义 API 别名。
7. 增加产品语义 API 别名：
   - documents
   - spaces
   - meetings
   - todos
8. 增加评论模型和评论接口；日记类型必须拒绝评论创建。
9. 增加回收站软删除、恢复和彻底删除接口。
10. 保持旧 task/project API 兼容，避免一次性破坏前端。

## 10. 验收场景

### 10.1 普通协同文档

- 用户在空间内新建文档。
- 两个浏览器同时打开同一文档。
- 两边编辑正文，内容自动合并。
- 断线后重连能恢复漏掉的正文 update。

### 10.2 日记

- 用户点击“新建日记”。
- 系统自动创建或打开 `YYYY-MM-DD.md`。
- 日记出现在“日记”空间。
- 同一天重复点击不会重复创建。
- 日记正文保存不启动多人协同。

### 10.3 会议纪要

- 用户点击“新建会议纪要”。
- 系统创建带基础模板的会议文档。
- 多人可同时编辑会议正文。
- 后续可以扩展参会人和行动项，而不破坏现有正文协同。
- 会议行动项后续可以生成轻量待办。

### 10.4 待办模块

- 用户可以创建待办事项。
- 用户可以完成/恢复/删除待办。
- 待办可以设置截止时间。
- 待办模块作为辅助功能存在，不影响协同文档主路径。

### 10.5 权限与一致性

- 未授权用户不能进入空间或文档。
- diary 默认只有 owner 可编辑。
- 普通文档和会议纪要遵循空间成员权限。
- 待办遵循个人或空间权限。
- metadata 冲突被锁或 CAS 拦截。

### 10.6 评论

- 普通文档可以创建、查看、删除评论。
- 会议纪要可以创建、查看、删除评论。
- 待办详情可以创建、查看、删除评论。
- 日记没有评论入口，也不能通过 API 创建评论。
- 评论不影响 Yjs 正文协同。

### 10.7 回收站

- 删除后的文档不会出现在默认空间列表。
- 回收站可以看到有权限的已删除条目。
- 用户可以恢复误删文档。
- 用户可以彻底删除条目，且彻底删除需要二次确认。
- 恢复操作可以通过项目事件同步到其他在线客户端。

## 11. 非目标

- 不把系统继续定位成任务管理器。
- 不把待办模块作为主要卖点。
- 不把日记默认做成多人协同。
- 不给日记开启评论功能。
- 不在第一阶段做完整 Notion block editor。
- 不自研 CRDT/OT 算法，继续使用 Yjs。
- 不把 Redis Pub/Sub 描述成可靠队列。
- 不为了重命名而立刻大规模迁移数据库表名。

## 12. 简历表述候选

- 将传统待办 CRUD 项目重定位并改造为类 Notion 的协同文档工作台，支持空间化文档管理、多人实时正文协同、Obsidian 风格个人日记、Notion 风格会议纪要和轻量待办模块。
- 基于 Yjs CRDT 实现 Markdown 文档多人在线协同编辑，服务端负责 WebSocket 房间转发、Redis Pub/Sub 多节点广播、update log / snapshot 持久化和断线恢复。
- 设计分层一致性方案：文档元数据使用 HTTP + Redis 锁 + MySQL CAS，正文使用 Yjs update，实时广播使用 Redis Pub/Sub，可靠补偿使用事件日志和 Sync API。
- 设计日记、会议纪要和待办的差异化模型：日记默认个人非协同，会议纪要默认多人协同，待办作为轻量辅助模块承接行动项。

---

# Obsidian + Notion 风格协同文档产品重定位计划（完整并入）

> 本章节由 `docs/plans/obsidian-notion-product-redesign.md` 完整并入。为保留完整语言和实施边界，以下内容不做压缩、不做摘要；仅将说明标签、导航名、阶段名和用户可见语义统一为中文表达。

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

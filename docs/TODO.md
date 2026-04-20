# TODO

## 进行中

- [x] 将当前单机 CRUD 待办系统重定位为 Obsidian + Notion 风格的协同文档工作台（MVP 口径已收口）
- [x] 补齐项目主文档：根目录 `AGENTS.md`、`README.md`、模块级 `AGENTS.md`
- [x] 将 `web/README.md` 从 Vite 模板替换为项目实际说明

## 已完成

- [x] 梳理实时协同系统总范围，并写入 `docs/plans/finallist.md`
- [x] 新增产品重定位计划：`docs/plans/obsidian-notion-product-redesign.md`
- [x] 明确实时协同的总体技术方向：HTTP + WebSocket + Redis Pub/Sub + Sync API
- [x] 明确 Kafka 继续用于到期提醒和可靠异步副作用
- [x] 建立本地 `sync-docs` skill，并加入当前项目文档映射
- [x] 初始化 Git 仓库，配置 GitHub SSH 远程仓库，并补充 `.gitignore` 保护本地配置、密钥和运行产物
- [x] 新增 plan 收口记录：`docs/plans/2026-04-20-plan-closure.md`

## 收口状态

- [x] P0 核心协同 MVP 已基本满足
- [x] P1 协同体验与分布式基础设施主项已基本满足
- [ ] P2 性能验证未做：按用户要求跳过多节点压测和性能测试
- [ ] 产品增强仍未完全收口：后端全文索引、Yjs 光标/选区 awareness、完整行内评论、会议结构化字段、日记标签/双链等高级能力

## 待办

### P0 核心协同

- [x] 前端整体信息架构和视觉风格改为 Obsidian + Notion：Spaces / Docs / Daily Notes / Meetings / Todos / Search
- [x] 新增 Obsidian Daily Notes 风格日记入口：创建或打开当天 `YYYY-MM-DD.md`，归属“日记”空间，默认非协同
- [x] 新增 Notion Meeting Notes 风格会议纪要入口：默认模板，支持多人协作
- [x] 整理轻量待办模块：可创建待办事项，但不作为主要卖点
- [x] 将正文编辑器从原生 textarea 升级为 CodeMirror 6 + Yjs 绑定，避免多人同区编辑时整篇替换
- [x] 为 `doc_type=diary` 补齐 plain Markdown 保存接口，替换当前 owner-only 私人正文通道
- [x] 给 `tasks` 增加 `version`
- [x] 新增 `task_events`
- [x] 新增 `task_content_updates` 正文协同 update 表与仓储
- [x] 接入基于 Yjs CRDT 的 `content_md` 后端 update 网关
- [x] 接入前端 Yjs provider 和任务详情正文编辑区
- [x] 新增正文 WebSocket 网关
- [x] 新增 project room 和权限校验
- [x] 新增 task 正文 room 和权限校验
- [x] 新增项目级 presence 快照
- [x] 新增项目级 metadata 协同锁状态展示和前端申请/释放链路
- [x] 新增 Sync API
- [x] 新增正文 update `message_id` 幂等去重
- [x] 新增 Markdown 文档分片导入接口，支持图片资源上传并改写为文档引用
- [x] 给 `tasks` 增加 `doc_type` / `collaboration_mode`，前端空间页按协作文档块和私人文档块组织入口
- [x] 重构前端任务状态流为增量 patch（`ProjectDetailPage`、`MyTasksPage`、`Next7DaysPage`、`CalendarPage` 已接入；聚合页暂未订阅跨项目实时事件）

### P1 协同体验与分布式基础设施

- [x] 将单机限流替换为 Redis 分布式限流
- [x] 将单机 `singleflight` 主逻辑替换为分布式缓存击穿保护
- [x] 增加锁续期和断线释放
- [x] 增加实时链路指标
- [x] 增加轻量操作历史接口
- [x] 增加评论功能：普通文档、会议纪要和待办支持评论，日记不开放评论
- [x] 增加回收站模块：删除改为软删除，支持恢复和彻底删除

### P2 可选增强

- [ ] 补充多节点压测
- [x] 接通项目摘要缓存异步回填
- [x] 新增后端搜索接口 `GET /api/v1/search`，替换 Search 页文档扫描式查询；全文索引仍是后续性能增强
- [x] 补空间级软删除 / 恢复 / 回收站，避免空间删除不可恢复
- [x] 补轻量文档查看 presence：详情页会向项目 WebSocket 上报正在查看的文档，并展示当前文档查看者
- [x] 补评论锚点预留：评论支持保存 selection anchor 文本，后续可升级为正文位置锚点
- [x] 补会议行动项转 todo 的最小入口
- [x] 补日记上一篇 / 下一篇导航
- [ ] 补后端全文索引、Yjs 光标/选区 awareness、完整行内评论、会议结构化字段、日记标签/双链/反向链接等高级能力
- [ ] 接入基于 `eino` 的 AI 写作与会议工作流：支持 Draft / Continue / Meeting Preview，第一阶段仅生成预览并由用户手动应用；设计与实施计划见 `docs/plans/2026-04-20-ai-writing-meeting-design.md` 和 `docs/plans/2026-04-20-ai-writing-meeting-implementation.md`

## 需要注意

- [x] 后端接口变更时检查 `docs/swagger.yaml`、`docs/swagger.json`、`docs/docs.go`
- [x] 如果开始落实实时协同，补充对应设计记录到 `docs/`
## 本轮已完成
- [x] `tasks.version + expected_version + CAS` 更新链路
- [x] 基于 Redis 的分布式锁 Watchdog 自动续期
- [x] `task_events` 事件表与事件仓储
- [x] `GET /api/v1/projects/:id/sync` 增量同步接口
- [x] `task_content_updates` 正文协同 update 表、仓储和 `tasks.content_md` 快照缓存更新方法
- [x] `GET /api/v1/tasks/:id/content/ws` 正文 WebSocket 网关、本机 room、Redis Pub/Sub fan-out、Yjs update 入库和 ACK/INIT/UPDATE 消息
- [x] `GET /api/v1/projects/:id/ws` 项目级任务事件 WebSocket room、本机 room、Redis Pub/Sub fan-out 和 PROJECT_INIT/TASK_* 消息
- [x] 项目级 `PRESENCE_SNAPSHOT` 在线态快照，支持本机 room 更新、Redis Pub/Sub 跨节点 fan-out 和前端在线人数展示
- [x] 项目级 metadata 协同锁，支持 `LOCK_REQUEST` / `LOCK_RELEASE`、断线释放、`TASK_LOCKED` / `TASK_UNLOCKED` 广播和前端锁状态展示
- [x] 前端 `ProjectDetailPage` 接入项目级 WebSocket 事件流和本地增量 patch，减少任务写操作后的整页 reload
- [x] 前端 `MyTasksPage`、`Next7DaysPage`、`CalendarPage` 写操作后改为本地 patch，失败或缺少 task snapshot 时再回退重新拉取
- [x] 前端 `TaskDetailPanel` 正文编辑区接入 Yjs，正文通过 WebSocket 实时保存；任务 Save 只提交元数据
- [x] 新增 `POST /api/v1/documents/imports`、`PUT /api/v1/documents/imports/:upload_id/parts/:part_no`、`POST /api/v1/documents/imports/:upload_id/assets`、`POST /api/v1/documents/imports/:upload_id/complete`、`DELETE /api/v1/documents/imports/:upload_id`
- [x] `ProjectDetailPage` 新增 Notion 风格协作文档块 / 私人文档块，支持新建文档、分片导入 Markdown 和上传图片引用
- [x] 新增 `POST /api/v1/diary/today`，前端侧边栏“日记”入口可幂等创建/打开当天 `YYYY-MM-DD.md`
- [x] 新增 `PATCH /api/v1/documents/:id/content`，日记正文改用 owner-only plain Markdown 保存，不再接入 Yjs 正文通道
- [x] 新增 `POST /api/v1/meetings`，支持在指定空间或默认“会议”空间创建协作会议纪要并注入模板
- [x] `ProjectDetailPage` 支持通过 `?task=<id>` 自动打开文档详情，供日记入口和后续全局入口复用
- [x] 前端侧边栏“会议”入口已接入，点击后创建会议纪要并自动打开
- [x] 重新生成 Swagger 产物，纳入本轮接口变更
- [x] 新增文档评论接口与详情面板评论区：普通文档 / 会议纪要 / 待办可评论，日记禁用评论
- [x] HTTP 限流改为 Redis 分布式 token bucket，protected 路由在鉴权后按 `uid` 限流，Redis 异常时本机限流兜底
- [x] 新增 `loadWithCacheProtection`，为用户资料、项目详情、项目列表 ID 重建、任务详情、任务列表 hydrate/fallback 等热点 cache miss 路径补齐 Redis 分布式缓存击穿保护
- [x] 空间详情页拆分文档块与轻量待办块，文档列表不再混入 todo，并支持在空间内快速创建轻量待办
- [x] `MyTasksPage`、`Next7DaysPage`、`CalendarPage` 收敛为 todo 次级模块，只展示 `doc_type=todo`，并按 reminder/due 时间组织视图
- [x] `AppLayout`、`ProjectListPage`、`TrashPage` 收敛到 Spaces / Daily Notes / Meetings / Todos 主次结构，并补齐空间首页与回收站文案
- [x] 新增顶层 `SearchPage`，侧边栏 Search 入口可搜索空间，并通过后端搜索文档 / 会议 / 待办标题、正文、类型和空间名，结果复用 `/projects/:id?task=:taskId` 打开详情
- [x] `SearchPage` 文档结果改为调用后端 `GET /api/v1/search`，不再逐空间拉取任务列表做前端扫描
- [x] 新增空间级回收站：`DELETE /api/v1/projects/:id` 软删除空间，`GET /api/v1/trash/spaces`、`POST /api/v1/trash/spaces/:id/restore`、`DELETE /api/v1/trash/spaces/:id` 支持列表、恢复和彻底删除
- [x] 项目 WebSocket 新增轻量文档查看 presence，`TaskDetailPanel` 展示当前文档查看者
- [x] 评论创建支持 `anchor_type` / `anchor_text`，前端评论区预留选区锚点输入与展示
- [x] 会议详情支持创建行动项 todo，复用当前空间创建轻量待办
- [x] 日记详情支持前一天 / 后一天导航，调用 `POST /api/v1/diary/:date` 幂等打开指定日期日记
- [x] `TaskSection`、`TaskDetailPanel`、`TrashPage` 状态与按钮文案按 document/todo 语义分流，不再直接暴露底层 `todo/done` 或 task 默认措辞
- [x] 新增 `GET /api/v1/projects/:id/activities`，基于 `task_events` 提供项目级/文档级文档活动记录分页接口，并同步 Swagger
- [x] `ProjectDetailPage` 接入 Recent Space Activity 面板，活动记录改为 recent-first 分页，并支持打开仍可访问的文档
- [x] 新增 `GET /api/v1/realtime/metrics`，返回当前节点 project/content WebSocket hub 的房间数、连接数、在线用户数、广播、Pub/Sub、丢弃连接、正文 update 和锁错误等指标，并同步 Swagger
- [x] 接通项目摘要缓存异步回填：项目列表 / 搜索命中摘要缓存，DB 查询成功后发布 `PutProjectsSummaryCache` 异步写入摘要并预热详情缓存，项目创建 / 更新 / 删除会 bump 用户级摘要版本让旧摘要失效

## 暂缓

- [ ] 多节点压测和性能测试：当前按用户要求跳过，先继续补产品功能

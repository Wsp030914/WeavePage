# server 接力说明

## 模块职责

`server/` 是主后端服务，负责：

- 鉴权
- 用户资料
- 项目管理
- 任务管理
- 任务成员权限
- Redis 缓存
- Kafka 异步事件
- 调度器回调入口

## 关键入口

- `main.go`
  - 初始化配置、MySQL、Redis、Kafka、Service、Router
- `router.go`
  - 注册全部 HTTP 路由和中间件
- `config/config.go`
  - 配置加载与 `TODO_*` 环境变量绑定

## 当前路由结构

- 公共接口：`/api/v1/login`、`/api/v1/register`
- 自鉴权 WebSocket：
  - `/api/v1/projects/:id/ws`
  - `/api/v1/tasks/:id/content/ws`
  - 这些路由不挂普通 `AuthMiddleware`，由 handler 支持 `Authorization: Bearer <token>` 或 `?token=<jwt>`，以兼容浏览器原生 WebSocket
- 内部接口：`/api/internal/scheduler/task-due`
- 鉴权后接口：
  - `/api/v1/users/me`
  - `/api/v1/projects`
  - `/api/v1/tasks`
  - `/api/v1/tasks/me`
  - `/api/v1/projects/:id/tasks/:task_id/members`
  - `/api/v1/diary/today`
  - `/api/v1/documents/:id/content`
  - `/api/v1/documents/:id/comments`
  - `/api/v1/comments/:id`
  - `/api/v1/meetings`
  - `/api/v1/meetings/:id/actions`
  - `/api/v1/search`
  - `/api/v1/realtime/metrics`
  - `/api/v1/trash/tasks`
  - `/api/v1/trash/spaces`
  - `/api/v1/documents/imports` 及其分片、图片资源、完成和取消导入子路由

## 当前架构事实

- Kafka 是当前唯一异步事件总线实现
- Redis 同时承担缓存和分布式锁
- 到期提醒依赖独立调度器，不是进程内 cron
- `task_service.go` 已经是业务最重的文件之一
- `document_import.go` 负责 Markdown 本地导入会话、分片对象、图片引用资源和最终文档创建
- `diary.go` 负责 Obsidian Daily Notes 语义入口：查找/创建“日记”空间与当天 `YYYY-MM-DD.md`
- `project_service.go`、`task_service.go`、`user_service.go` 的主要 cache miss 回源路径已通过 `loadWithCacheProtection` 组合本机 `singleflight` 与 Redis 分布式锁

## 当前非显而易见约束

- 当前 HTTP 限流已改为 Redis 分布式 token bucket
  症状：旧实现挂在全局中间件上，鉴权前拿不到 `uid`，多实例也无法共享配额
  原因：`uid` 只有 `AuthMiddleware` 后才写入 context，本机 `rate.Limiter` 也只在单进程内生效
  结论：现在 public/internal/自鉴权 WebSocket 路由按 `IP + method + route` 限流，protected 路由在鉴权后按 `uid + method + route` 限流；Redis 异常时会记录 warning 并降级到本机 limiter

- 主要缓存击穿路径已补 Redis 分布式保护
  症状：单独使用 `singleflight.Group` 时，单机压测 DB 压力可控，但多实例部署后无法跨节点合并 cache miss
  原因：`singleflight` 只在当前 Go 进程内共享，Redis 才能承担跨实例互斥
  结论：用户资料、项目详情、项目列表 ID 重建、任务详情、任务列表 hydrate/fallback 现在先用本机 `singleflight` 合并，再用 Redis 锁控制跨实例回源；未拿到锁的请求会短暂等待缓存回填，超时后为可用性降级回源

- 项目摘要缓存已接入读写链路
  症状：项目搜索和列表 DB fallback 仍会重复组装同一页结果
  原因：之前只有 `PutProjectsSummaryCache` 异步 handler，service 读路径没有读取摘要缓存，也没有发布摘要写入事件
  结论：`ProjectService.List` 现在会按用户级 summary version 读取摘要缓存；DB 查询成功后发布 `PutProjectsSummaryCache`，由异步 handler 写摘要并预热详情缓存；项目创建、更新、删除会 bump 用户级 summary version，让旧摘要自然失效

- 空间删除已接入回收站
  症状：文档 / 会议 / 待办已经是 task 级软删除，空间删除也需要可恢复，否则产品语义不一致
  原因：`projects` 已补 `deleted_at` / `deleted_by` / `deleted_name`，默认 GORM 查询会排除软删除空间
  结论：`DELETE /api/v1/projects/:id` 只移动到空间回收站；恢复走 `POST /api/v1/trash/spaces/:id/restore`；彻底删除只能走 `DELETE /api/v1/trash/spaces/:id`，并会硬删除空间及其任务

- Kafka 当前不负责实时协同广播
  症状：已有 Kafka，容易自然联想到直接给 WebSocket 用
  原因：它现在主要用于可靠异步副作用
  结论：实时协同主链路应走 Redis Pub/Sub + Sync API

- 实时链路指标是本节点快照，不是集群聚合
  症状：多实例部署后，`/api/v1/realtime/metrics` 只会显示命中当前 API 实例的 WebSocket room 和累计计数
  原因：连接、房间和发送缓冲区都在各 Go 进程内，Redis Pub/Sub 只负责 fan-out，不保存全局连接表
  结论：排查单节点连接泄漏、广播丢弃和 Pub/Sub 错误时直接用该接口；如果需要全局看板，应由 Prometheus/日志系统跨实例聚合，不要在 hub 内维护跨节点状态

- 正文协同不等同于普通任务事件同步
  症状：`task_events + Sync API` 可以补偿任务元数据变更，看起来也能同步正文
  原因：多人同时编辑长文本需要合并插入/删除操作，CAS 只能发现冲突，不能自动合并
  结论：`content_md` 正文主线使用 Yjs CRDT update；服务端只做鉴权、持久化和广播，不自研 CRDT/OT

- Markdown 大文件导入不应走单次大 multipart
  症状：本地 `.md` 文件过大时，普通上传容易遇到 API 超时、请求体限制和代理限制
  原因：正文最终仍要写入 `tasks.content_md`，但上传链路需要可恢复、可分片
  结论：使用 `DocumentImportService` 在 Redis 保存会话，分片临时写入 COS，完成时组装、校验、改写图片引用，再复用 `TaskService.Create`

- 私人文档不能添加协作者
  症状：协作文档和私人文档在 UI 上是两个 Notion 风格块
  原因：底层仍复用 `tasks`，需要字段表达访问语义
  结论：`tasks.collaboration_mode=private` 时，成员添加和非 owner 正文访问会被拒绝

- 今日日记入口必须保持幂等
  症状：用户会反复点击侧边栏“日记”
  原因：Daily Notes 语义要求同一天只打开同一篇 `YYYY-MM-DD.md`
  结论：`TaskService.OpenTodayDiary` 先按用户和标题查找，缺失才创建；并发重复创建要回读已存在记录

- Swagger 文档是生成产物
  症状：改了 handler 或注释后，文档可能还是旧的
  原因：`docs/swagger.yaml`、`docs/swagger.json`、`docs/docs.go` 不会自动更新
  结论：接口变化时要明确同步或标记需重生成

## 重点文件

- `service/task_service.go`
  - 任务 CRUD、成员、到期提醒、缓存回填、锁
- `service/project_service.go`
  - 项目列表缓存、详情缓存、项目摘要缓存异步回填，以及空间软删除 / 恢复 / 彻底删除
- `service/user_service.go`
  - 注册、登录、资料更新、头像上传
- `service/auth_service.go`
  - token 版本校验和黑名单检查
- `middlewares/ratelimit.go`
  - Redis 分布式 token bucket 限流实现，内置本机 limiter 作为 Redis 故障降级
- `cache/lock.go`
  - 当前 Redis 分布式锁
- `async/`
  - Kafka producer、consumer、handler
- `realtime/content_hub.go`
  - 任务正文 WebSocket room、本机广播、Redis Pub/Sub fan-out
- `realtime/hub.go`
  - 项目级任务事件 WebSocket room、本机广播、Redis Pub/Sub fan-out、presence 快照和 metadata 协同锁消息
- `realtime/metrics.go`
  - project/content hub 共享的本节点实时链路指标结构和累计计数器
- `realtime/lock_manager.go`
  - 项目级任务 metadata 协同锁；复用 Redis 分布式锁和 Watchdog，连接断开时释放当前连接持有的锁
- `handler/content_ws.go`
  - 正文协同 WebSocket 握手鉴权和任务权限校验
- `handler/ws.go`
  - 项目事件 WebSocket 握手鉴权和项目权限校验
- `handler/realtime_metrics.go`
  - `GET /api/v1/realtime/metrics`，返回当前节点 project/content hub 的房间、连接、广播、Pub/Sub、丢弃连接、正文 update 和锁错误指标
- `handler/document_import.go`
  - Markdown 导入会话、分片上传、图片资源上传、完成导入和取消导入
- `service/document_import.go`
  - Redis 会话状态、COS 分片对象、图片引用改写和导入完成创建文档
- `handler/diary.go`
  - `POST /api/v1/diary/today` 和 `POST /api/v1/diary/:date`，返回“日记”空间和指定日期 diary 文档
- `handler/task.go`
  - `PATCH /api/v1/documents/:id/content`，仅 diary owner 可保存 plain Markdown 正文，不写 Yjs update log
- `service/diary.go`
  - 复用 `projects + tasks`，创建 `doc_type=diary`、`collaboration_mode=private` 的当天 Markdown 文档
- `handler/meeting.go`
  - `POST /api/v1/meetings` 创建会议纪要；`POST /api/v1/meetings/:id/actions` 把会议行动项转为同空间 todo
- `service/meeting.go`
  - 复用 `projects + tasks`，创建 `doc_type=meeting`、`collaboration_mode=collaborative` 的会议模板文档，并提供行动项转 todo 最小入口
- `handler/comment.go`
  - `GET/POST /api/v1/documents/:id/comments`、`PATCH/DELETE /api/v1/comments/:id`，文档级评论接口
- `service/task_comment.go`
  - 评论权限、创建、列出、解决/重开和删除；支持 `anchor_type` / `anchor_text` 预留评论锚点；复用任务正文权限判断，明确拒绝 diary 评论
- `handler/search.go`
  - `GET /api/v1/search`，聚合当前用户空间和可访问文档 / 会议 / 待办结果

## 后续改造主线

如果开始做实时协同，优先顺序按 `docs/plans/finallist.md`：

1. `tasks.version`
2. `task_events`
3. Sync API
4. `task_content_updates` 正文 update log
5. WebSocket gateway 和 task 正文 room
6. Redis Pub/Sub
7. 前端 Yjs provider
8. 协同锁和在线态（项目级 presence 与 metadata 锁已落地）
9. 分布式限流
10. 分布式缓存击穿保护

## 改动时必须联动检查

- 改接口：检查 Swagger
- 改到期提醒：检查 `scheduler/`
- 改缓存策略：检查 `singleflight` 和 Redis 锁是否仍一致
- 改鉴权：检查前端 token 流和 `AuthMiddleware`
## 最近新增
- HTTP 限流已从全局单机 `rate.Limiter` 改为 Redis 分布式 token bucket：
  - public/internal/自鉴权 WebSocket 路由仍按 IP 限流，避免未鉴权入口无限放大。
  - protected 路由的限流中间件挂在 `AuthMiddleware` 之后，按 `uid` 限流。
  - Redis 调用使用 50ms 超时；Redis 异常时记录 `rate_limit_fallback` warning，并使用本机 limiter 兜底。
- 已新增 `service/cache_protection.go`，将热点 cache miss 回源路径统一收敛为“本机 singleflight + Redis 分布式锁 + 等待缓存回填 + 超时降级回源”。
- 已接通项目摘要缓存异步回填：
  - `ProjectCache` 负责 summary version、summary key、摘要读写。
  - `ProjectService.List` 会优先读取搜索摘要缓存，并在普通列表 DB fallback 前尝试摘要缓存。
  - DB 查询成功后发布 `PutProjectsSummaryCache`，Kafka consumer 由 `handlers.PutProjectsSummary` 写摘要并预热项目详情缓存。
  - 创建、更新、删除空间后会 bump 用户级 summary version，避免旧摘要被再次命中。
- 已落地 `task_events` 事件日志表，作为 Sync API 的可靠补偿数据源。
- 已新增 `GET /api/v1/projects/:id/sync`，按事件自增游标返回项目级任务增量事件。
- 已新增 `GET /api/v1/projects/:id/ws` 项目级任务事件 WebSocket：
  - 连接时先校验 JWT 和项目访问权限。
  - 建连后发送 `PROJECT_INIT` 补齐 `cursor` / `last_event_id` 之后的 `task_events`。
  - 任务 create/update/delete 事务成功后，会向本机 project room 和 Redis Pub/Sub 广播 `TASK_CREATED`、`TASK_UPDATED`、`TASK_DELETED`。
  - 连接加入、离开和周期心跳会推送 `PRESENCE_SNAPSHOT`；快照按节点发布，前端需要按 `server_node_id` 合并。
  - 客户端可发送 `LOCK_REQUEST` / `LOCK_RELEASE` 获取或释放任务 metadata 锁；服务端广播 `TASK_LOCKED` / `TASK_UNLOCKED` / `LOCK_ERROR`，连接断开时会释放该连接持有的锁。
- 已落地 `task_content_updates` 正文协同 update 表和仓储，支持按 task cursor 查询 Yjs update、按 `message_id` 幂等查询、查询最新正文快照。
- 已新增 `GET /api/v1/tasks/:id/content/ws` 正文 WebSocket 网关：
  - 连接时先校验 JWT 和任务访问权限。
  - 建连后发送 `CONTENT_INIT` 补齐 `last_update_id` 之后的 update。
  - 收到 `CONTENT_UPDATE` 后写入 `task_content_updates`，刷新可选 `tasks.content_md` 快照，并向本机 room 和 Redis Pub/Sub 广播。
  - 重复 `message_id` 只返回 ACK，不重复广播。
- 已新增 `GET /api/v1/realtime/metrics`：
  - 需要普通 Bearer 鉴权。
  - 返回当前节点 project/content WebSocket hub 的房间数、连接数、在线用户数和每个 room 的连接/用户数。
  - 返回进程启动以来的累计计数：连接接入/关闭、广播、Pub/Sub 发布/接收/失败、丢弃连接、正文 update、重复 update、锁请求和锁错误。
- `TaskRepository` 已新增 `UpdateContentSnapshot`，用于后续 CRDT 链路刷新 `tasks.content_md` 只读缓存；该方法不提升任务元数据 `version`。
- `TaskService` 的 create/update/delete 现在会在同一事务里同时写任务数据和事件日志，避免补偿层漏记。
- `cache/lock.go` 已升级为 Redis Watchdog 续期锁；续期通过 Lua 先校验 owner 再 `PEXPIRE`，不要改回盲续期。
- `realtime/lock_manager.go` 已复用 Watchdog 锁做项目级 metadata 协同锁；不要为 WebSocket 锁再写一套独立 Redis owner/TTL 逻辑。
- Swagger 产物已随本轮接口变化重新生成；如果继续改接口，记得统一补一次。
- `tasks` 已新增 `doc_type` / `collaboration_mode`，MySQL 初始化会补列；后续 diary / meeting 不要另起平行表，优先复用这两个字段表达产品语义。
- Markdown 导入接口已纳入 Swagger；后续修改导入参数或响应时要重新生成 `docs/swagger.*` 和 `docs/docs.go`。
- `POST /api/v1/diary/today` 已纳入 Swagger；diary 正文保存已改走 `PATCH /api/v1/documents/:id/content`，不要再让 diary 接入任务正文 Yjs WebSocket。
- `POST /api/v1/diary/:date` 已纳入 Swagger；日期格式为 `YYYY-MM-DD`，用于日记前一天 / 后一天导航。
- `PATCH /api/v1/documents/:id/content` 已纳入 Swagger；该接口只允许 diary owner 带 `expected_version` 保存 `content_md`，不写 `task_content_updates`。
- `POST /api/v1/meetings` 已纳入 Swagger；默认走“会议”空间 + 协作模式，如果要改默认模板请同步前端文案和产品文档。
- `POST /api/v1/meetings/:id/actions` 已纳入 Swagger；只接受 meeting 文档，把行动项标题转成同空间 `doc_type=todo`。
- 已新增 `task_comments` 表和评论服务：
  - `GET /api/v1/documents/:id/comments` 返回文档级评论列表。
  - `POST /api/v1/documents/:id/comments` 创建评论，可选 `anchor_type` / `anchor_text` 预留选区锚点。
  - `PATCH /api/v1/comments/:id` 切换评论 `resolved` 状态。
  - `DELETE /api/v1/comments/:id` 删除评论。
  - diary 文档评论会被拒绝；作者和文档 owner/editor 可以解决或删除评论。
- 已新增 task 级回收站能力：
  - `tasks` 已补 `deleted_at` / `deleted_by` / `deleted_title`；默认查询会排除软删除记录。
  - `DELETE /api/v1/tasks/:id` 改为软删除，并继续广播 `TASK_DELETED` 让活动列表立即移除。
  - `GET /api/v1/trash/tasks` 返回当前用户回收站列表。
  - `POST /api/v1/trash/tasks/:id/restore` 恢复文档；若同空间已有同名活跃任务会返回冲突。
  - `DELETE /api/v1/trash/tasks/:id` 才是真正硬删除。
- 已新增 space 级回收站能力：
  - `projects` 已补 `deleted_at` / `deleted_by` / `deleted_name`；默认查询会排除软删除空间。
  - `DELETE /api/v1/projects/:id` 改为软删除空间，并 bump 项目摘要缓存版本。
  - `GET /api/v1/trash/spaces` 返回当前用户空间回收站列表。
  - `POST /api/v1/trash/spaces/:id/restore` 恢复空间；若同用户已有同名活跃空间会返回冲突。
  - `DELETE /api/v1/trash/spaces/:id` 才是真正硬删除，并会硬删除该空间下任务。
- 已新增 `GET /api/v1/search`：
  - `q` 为空时返回空集合，避免顶层搜索页触发全量扫描。
  - 当前是 DB LIKE 聚合搜索，不是全文索引；如果后续文档量变大，应迁移到全文索引或搜索服务。
- 项目 WebSocket 已新增 `VIEW_DOCUMENT`：
  - 客户端打开文档时上报 task ID，关闭或切换时上报 0。
  - `PRESENCE_SNAPSHOT` 会包含 `viewing_task_ids`，用于前端展示轻量文档查看 presence；还不包含正文光标或选区。

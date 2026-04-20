# 项目接力说明

## 项目概览

这是一个前后端分离的协同文档工作台仓库，正在从原待办系统重定位为 Obsidian + Notion 风格产品。当前由 3 个主要运行单元组成：

- `server`
  - Go + Gin 后端 API
  - 负责用户、空间/项目、文档/任务、成员权限、缓存、实时协同、异步事件和轻量待办能力
- `web`
  - React + Vite 前端
  - 前端已收敛到 Spaces / Docs / Daily Notes / Meetings / Todos 信息架构，并新增 Search 顶层入口；`ProjectDetailPage` 已接入项目级 WebSocket 事件流和本地增量 patch
- `scheduler`
  - 独立的调度服务
  - 基于 Redis 存储定时任务，按时间回调后端内部接口

## 当前技术架构

- 数据库：MySQL
- 缓存与锁：Redis
- 异步事件总线：Kafka
- API 文档：Swagger 生成产物位于 `docs/`
- 生产部署：`docker-compose.prod.yml`

当前后端已经具备：

- JWT 鉴权
- 用户注册、登录、资料更新、头像上传
- 项目 CRUD
- 任务 CRUD
- 任务成员管理
- 任务到期提醒
- Redis 缓存
- Redis 分布式锁
- Kafka 异步消费、重试、DLQ

## 当前重要事实

- 实时协同正文通道已部分落地：`task_content_updates`、正文 WebSocket 网关、本机 room、Redis Pub/Sub fan-out 和前端 Yjs provider 已实现
- 项目级任务事件 WebSocket room 已落地：`GET /api/v1/projects/:id/ws` 支持 JWT 自鉴权、项目权限校验、`PROJECT_INIT` 补偿和任务事件实时广播
- 项目级 presence 已落地：项目 WebSocket 会推送 `PRESENCE_SNAPSHOT`，前端 `ProjectDetailPage` 会显示在线人数；详情页会发送 `VIEW_DOCUMENT` 上报当前查看文档，`TaskDetailPanel` 会展示当前文档查看者
- 项目级 metadata 协同锁已落地：项目 WebSocket 支持 `LOCK_REQUEST` / `LOCK_RELEASE`，广播 `TASK_LOCKED` / `TASK_UNLOCKED` / `LOCK_ERROR`；前端项目详情页会展示锁状态，任务详情编辑元数据时会申请并释放 `metadata` 锁
- 实时链路指标已落地：`GET /api/v1/realtime/metrics` 返回当前节点 project/content WebSocket hub 的房间、连接、在线用户和累计广播 / PubSub / 丢弃连接 / 正文 update / 锁错误计数
- 限流已改为 Redis 分布式 token bucket，位于 `server/middlewares/ratelimit.go`，Redis 异常时降级到本机 limiter
- 主要缓存击穿路径已接入 `loadWithCacheProtection`：本机 `singleflight` 合并进程内请求，Redis 分布式锁跨实例保护 DB 回源，未拿到锁的请求会短暂等待缓存回填
- 项目摘要缓存异步回填已接通：项目列表 / 搜索会读取摘要缓存，DB 查询成功后发布 `PutProjectsSummaryCache` 写入摘要并预热详情缓存；项目创建 / 更新 / 删除会 bump 用户级摘要版本使旧摘要失效
- Kafka 目前承担的是可靠异步副作用，不是实时广播链路
- 当前产品主线已经改为协同文档工作台：普通文档和会议纪要可协作，日记类似 Obsidian Daily Notes 且默认非协同，待办只是轻量辅助模块
- `tasks` 已新增 `doc_type` 和 `collaboration_mode`，当前用于区分普通文档/后续日记会议类型，以及协作文档/私人文档入口
- Markdown 本地导入已落地为 Redis 会话 + COS 分片对象 + 图片资源上传：完成导入时会组装 Markdown、改写本地图片引用，并复用 `TaskService.Create` 创建文档
- Obsidian Daily Notes 入口已落地：`POST /api/v1/diary/today` 会幂等创建/打开“日记”空间下当天 `YYYY-MM-DD.md`，文档标记为 `doc_type=diary`、`collaboration_mode=private`
- 日记正文已改为 owner-only plain Markdown 保存接口：`PATCH /api/v1/documents/:id/content`；`doc_type=diary` 不再接入任务正文 Yjs WebSocket / update log
- 文档评论已落地：`GET/POST /api/v1/documents/:id/comments`、`PATCH/DELETE /api/v1/comments/:id` 支持普通文档、会议纪要和待办的文档级评论；`POST` 已预留 `anchor_type` / `anchor_text`，`doc_type=diary` 会拒绝评论
- 文档/任务回收站已落地：`DELETE /api/v1/tasks/:id` 改为软删除；`GET /api/v1/trash/tasks`、`POST /api/v1/trash/tasks/:id/restore`、`DELETE /api/v1/trash/tasks/:id` 支持列出、恢复和彻底删除
- 空间级回收站已落地：`DELETE /api/v1/projects/:id` 改为软删除空间；`GET /api/v1/trash/spaces`、`POST /api/v1/trash/spaces/:id/restore`、`DELETE /api/v1/trash/spaces/:id` 支持列出、恢复和彻底删除
- 后端搜索接口已落地：`GET /api/v1/search` 聚合空间和当前用户可访问文档 / 会议 / 待办，前端 Search 页不再逐空间扫描任务列表；当前仍是 DB LIKE 查询，不是全文索引
- 前端项目详情页已具备事件驱动 patch；`MyTasksPage`、`Next7DaysPage`、`CalendarPage` 等待办视图已复用本地 patch helper 处理自身写操作，后续应收敛到 Todos 次级模块

## 当前主线规划

实时协同改造总清单见：

- `docs/plans/finallist.md`
- `docs/plans/obsidian-notion-product-redesign.md`
- `docs/plans/2026-04-20-plan-closure.md`

后续实现方向已经明确为：

- HTTP 继续做权威写路径
- WebSocket 负责低延迟同步
- Redis Pub/Sub 负责多节点实时广播
- `task_events + Sync API` 负责断线补偿
- `content_md` 正文协同使用 Yjs CRDT；服务端只负责鉴权、转发、Redis fan-out、update log / snapshot 持久化，不自研 CRDT/OT
- 日记正文默认使用 plain Markdown 保存，不启用多人协同
- 会议纪要默认启用多人协作，已提供行动项转 todo 的最小入口；后续仍可扩展结构化会议字段
- 待办模块保留基础创建/完成/截止时间能力，但不作为主要卖点
- Kafka 保留给到期提醒、通知等可靠异步任务

当前收口结论：

- P0 核心协同 MVP 和 P1 主要基础设施已基本完成
- P2 多节点压测 / 性能测试按用户要求暂缓
- 明确剩余边界：多节点压测 / 性能测试按要求暂缓；全文索引、Yjs 光标/选区 awareness、完整行内评论、会议结构化字段、日记标签/双链等高级能力仍是后续增强

## 常用命令

- 后端开发启动：`go run ./server`
- 调度器启动：`go run ./scheduler`
- 后端测试：`go test ./...`
- 前端安装依赖：`cd web && npm install`
- 前端开发：`cd web && npm run dev`
- 前端构建：`cd web && npm run build`
- 前端检查：`cd web && npm run lint`
- 生产编排：`docker compose -f docker-compose.prod.yml up --build`

## 配置规则

- 后端优先读取 `TODO_CONFIG_FILE`
- 如果未设置，则会在默认路径下查找 `config.yml`
- `server/config.yml`、`secrets/`、本地 `.env*`、`.vscode/`、`log/`、`web/node_modules/` 和 `web/dist/` 已通过 `.gitignore` 排除，提交或推送前不要强行 `git add -f` 这些本地配置、密钥和运行产物
- 如果需要共享配置结构，新增不含真实密钥的 `*.example` 文件，而不是提交本机配置或生产密钥
- 配置支持通过 `TODO_*` 环境变量覆盖，例如：
  - `TODO_JWT_SECRET`
  - `TODO_MYSQL_PASSWORD`
  - `TODO_REDIS_PASSWORD`
  - `TODO_DUE_SCHEDULER_CALLBACK_TOKEN`

## 文档约定

本项目以 `AGENTS.md` 为 AI 接力主文档，不强制维护 `CLAUDE.md`。

当前应维护的核心文档：

- 根目录 `AGENTS.md`
- 根目录 `README.md`
- `docs/TODO.md`
- `server/AGENTS.md`
- `web/AGENTS.md`
- `scheduler/AGENTS.md`
- 必要时同步 `docs/swagger.yaml`、`docs/swagger.json`、`docs/docs.go`

## 接力时优先关注

- 改接口时，同时检查 Swagger 生成产物是否需要重生
- 改缓存策略时，同时检查 `singleflight` 和 Redis 锁是否仍一致
- 改到期提醒时，同时检查 `server/service/task_service.go` 和 `scheduler/main.go`
- 改前端任务详情流时，同时检查 `ProjectDetailPage.jsx` 和 `TaskDetailPanel.jsx`
- 如果开始做实时协同，不要直接把 Kafka 当 WebSocket 广播总线
- 如果继续做 Search，优先补全文索引或搜索引擎；当前 `GET /api/v1/search` 已替代前端扫描，但仍是 DB LIKE 查询
## 最近进展

- 实时协同改造的前两步已经落地：`tasks.version` / `expected_version` / CAS 更新，以及 `task_events + Sync API`。
- 当前后端已经有两层补偿与并发保护：
  - Redis 分布式锁 + Watchdog 自动续期
  - MySQL 版本号 CAS 防止静默覆盖
- 当前新增接口：`GET /api/v1/projects/:id/sync`
- 当前新增实时指标接口：`GET /api/v1/realtime/metrics`
- 当前新增 Markdown 导入接口：
  - `POST /api/v1/documents/imports`
  - `PUT /api/v1/documents/imports/:upload_id/parts/:part_no`
  - `POST /api/v1/documents/imports/:upload_id/assets`
  - `POST /api/v1/documents/imports/:upload_id/complete`
  - `DELETE /api/v1/documents/imports/:upload_id`
- 当前新增日记接口：`POST /api/v1/diary/today`
  - 查找或创建当前用户的“日记”空间
  - 查找或创建当天 `YYYY-MM-DD.md`
  - 返回 `{ project, task }`，前端会跳转到对应空间并通过 `?task=<id>` 自动打开详情
- 当前新增指定日期日记接口：`POST /api/v1/diary/:date`
  - `:date` 使用 `YYYY-MM-DD`
  - 前端详情页的前一天 / 后一天导航会调用该接口幂等打开对应日记
- 当前新增日记正文保存接口：`PATCH /api/v1/documents/:id/content`
  - 仅允许 diary owner 保存 `content_md`
  - 需要 `expected_version` 做 CAS
  - 不写入 `task_content_updates`，不走 Yjs 正文协同通道
- 当前新增会议接口：`POST /api/v1/meetings`
  - 可选传 `project_id` / `title`，默认创建到“会议”空间
  - 文档类型固定为 `doc_type=meeting`，协作模式固定为 `collaboration_mode=collaborative`
  - 默认注入会议模板：时间、参会人、议题、结论、行动项
- 当前新增会议行动项接口：`POST /api/v1/meetings/:id/actions`
  - 在同一空间创建 `doc_type=todo` 的轻量待办，作为会议行动项转 todo 的最小入口
- 当前新增评论接口：
  - `GET /api/v1/documents/:id/comments`
  - `POST /api/v1/documents/:id/comments`
  - `PATCH /api/v1/comments/:id`
  - `DELETE /api/v1/comments/:id`
  - 创建评论可带 `anchor_type` / `anchor_text` 预留选区锚点；日记不开放评论；作者和文档 owner/editor 可解决或删除评论
- 当前新增回收站接口：
  - `GET /api/v1/trash/tasks`
  - `POST /api/v1/trash/tasks/:id/restore`
  - `DELETE /api/v1/trash/tasks/:id`
  - `DELETE /api/v1/tasks/:id` 现为软删除；恢复时若同空间已有同名活跃文档会返回冲突
- 当前新增空间回收站接口：
  - `GET /api/v1/trash/spaces`
  - `POST /api/v1/trash/spaces/:id/restore`
  - `DELETE /api/v1/trash/spaces/:id`
  - `DELETE /api/v1/projects/:id` 现为软删除空间；彻底删除会硬删除空间及其任务
- 当前新增搜索接口：`GET /api/v1/search`
  - 查询参数 `q` / `limit`
  - 返回 `spaces` 和 `documents`
- 当前新增项目事件 WebSocket 接口：`GET /api/v1/projects/:id/ws`
  - 支持 `Authorization: Bearer <token>` 或 `?token=<jwt>`
  - 查询参数支持 `cursor` 或 `last_event_id`
  - 消息类型包括 `PROJECT_INIT`、`PROJECT_SYNC`、`TASK_CREATED`、`TASK_UPDATED`、`TASK_DELETED`、`PRESENCE_SNAPSHOT`、`VIEW_DOCUMENT`、`LOCK_REQUEST`、`LOCK_RELEASE`、`TASK_LOCKED`、`TASK_UNLOCKED`、`LOCK_ERROR`、`PING`、`PONG`、`PROJECT_ERROR`
- 当前新增正文 WebSocket 接口：`GET /api/v1/tasks/:id/content/ws`
  - 支持 `Authorization: Bearer <token>` 或 `?token=<jwt>`
  - 消息类型包括 `CONTENT_INIT`、`CONTENT_SYNC`、`CONTENT_UPDATE`、`CONTENT_ACK`、`CONTENT_ERROR`
- 当前新增持久化对象：`server/models/task_event.go`
- 当前新增正文协同持久化对象：`server/models/task_content_update.go`
- 当前新增正文协同仓储：`server/repo/task_content_repo.go`
- 当前新增正文协同运行时：`server/realtime/content_hub.go` 和 `server/handler/content_ws.go`
- 当前新增项目事件运行时：`server/realtime/hub.go` 和 `server/handler/ws.go`
- 当前新增前端项目事件接入：`web/src/realtime/projectEventsSocket.js`、`web/src/realtime/protocol.js`、`web/src/store/collab-store.js`
- `web/src/pages/ProjectDetailPage.jsx` 已从任务 create/update/delete 后强制 `loadData()` 改为本地 patch + WebSocket 事件收敛；成员变更等暂仍可触发 reload
- `web/src/pages/MyTasksPage.jsx`、`web/src/pages/Next7DaysPage.jsx`、`web/src/pages/CalendarPage.jsx` 已在 toggle/delete/detail metadata save 成功后本地 patch；失败或成员变更等缺少 task snapshot 的路径仍可触发 reload 兜底
- `web/src/pages/ProjectDetailPage.jsx` 已展示项目在线人数，并会在打开 / 切换 / 关闭文档时发送 `VIEW_DOCUMENT`；`web/src/components/TaskDetailPanel.jsx` 会展示当前文档查看者
- `web/src/pages/ProjectDetailPage.jsx` 已消费项目级锁事件；`web/src/components/TaskDetailPanel.jsx` 会在编辑标题、优先级、截止时间等元数据前申请 `metadata` 锁，保存、取消、关闭或切换任务时释放锁；正文 textarea 仍走 Yjs 协同，不受 metadata 锁阻塞
- `web/src/pages/ProjectDetailPage.jsx` 已新增 Notion 风格协作文档块和私人文档块，两个块都支持新建文档、上传 `.md/.markdown` 并附加图片资源
- `web/src/components/TaskDetailPanel.jsx` 已新增评论区；普通文档、会议纪要和待办会加载评论列表，支持创建、解决/重开、删除和选区锚点文本预留，日记不展示评论入口
- `web/src/components/TaskDetailPanel.jsx` 已支持会议行动项转 todo 的最小入口，以及日记前一天 / 后一天导航
- `web/src/layouts/AppLayout.jsx` 已接入“日记”主导航入口，调用 `POST /api/v1/diary/today` 后刷新空间列表并跳转到日记文档
- `web/src/layouts/AppLayout.jsx` 已接入“会议”主导航入口，调用 `POST /api/v1/meetings` 后创建会议纪要并自动打开
- `web/src/layouts/AppLayout.jsx` 已接入“Search”主导航入口，`web/src/pages/SearchPage.jsx` 通过后端 `GET /api/v1/search` 搜索文档/会议/待办，文档结果会跳转到 `/projects/:id?task=:taskId`
- `web/src/pages/ProjectDetailPage.jsx` 支持 `?task=<id>` 自动打开指定文档详情
- Swagger 产物已随本轮接口变化重生成：`docs/swagger.yaml`、`docs/swagger.json`、`docs/docs.go`

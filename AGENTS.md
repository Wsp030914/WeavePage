# 项目接力说明

## 项目概览

这是一个前后端分离的协同文档工作台仓库，正在从原待办系统重定位为 Obsidian + Notion 风格产品。当前由 3 个主要运行单元组成：

- `server`
  - Go + Gin 后端 API
  - 负责用户、空间/项目、文档/任务、成员权限、缓存、实时协同、异步事件和轻量待办能力
- `web`
  - React + Vite 前端
  - 前端需要逐步改为 Spaces / Docs / Daily Notes / Meetings / Todos 信息架构；`ProjectDetailPage` 已接入项目级 WebSocket 事件流和本地增量 patch
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
- 项目级 presence 已落地：项目 WebSocket 会推送 `PRESENCE_SNAPSHOT`，前端 `ProjectDetailPage` 会显示在线人数
- 项目级 metadata 协同锁已落地：项目 WebSocket 支持 `LOCK_REQUEST` / `LOCK_RELEASE`，广播 `TASK_LOCKED` / `TASK_UNLOCKED` / `LOCK_ERROR`；前端项目详情页会展示锁状态，任务详情编辑元数据时会申请并释放 `metadata` 锁
- 限流是单机版，位于 `server/middlewares/ratelimit.go`
- `singleflight` 也是单机版，主要分布在 `server/service/*.go`
- Kafka 目前承担的是可靠异步副作用，不是实时广播链路
- 当前产品主线已经改为协同文档工作台：普通文档和会议纪要可协作，日记类似 Obsidian Daily Notes 且默认非协同，待办只是轻量辅助模块
- 前端项目详情页已具备事件驱动 patch；`MyTasksPage`、`Next7DaysPage`、`CalendarPage` 等待办视图已复用本地 patch helper 处理自身写操作，后续应收敛到 Todos 次级模块

## 当前主线规划

实时协同改造总清单见：

- `docs/plans/finallist.md`
- `docs/plans/obsidian-notion-product-redesign.md`

后续实现方向已经明确为：

- HTTP 继续做权威写路径
- WebSocket 负责低延迟同步
- Redis Pub/Sub 负责多节点实时广播
- `task_events + Sync API` 负责断线补偿
- `content_md` 正文协同使用 Yjs CRDT；服务端只负责鉴权、转发、Redis fan-out、update log / snapshot 持久化，不自研 CRDT/OT
- 日记正文默认使用 plain Markdown 保存，不启用多人协同
- 会议纪要默认启用多人协作，后续扩展结构化会议字段和行动项
- 待办模块保留基础创建/完成/截止时间能力，但不作为主要卖点
- Kafka 保留给到期提醒、通知等可靠异步任务

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
## 最近进展

- 实时协同改造的前两步已经落地：`tasks.version` / `expected_version` / CAS 更新，以及 `task_events + Sync API`。
- 当前后端已经有两层补偿与并发保护：
  - Redis 分布式锁 + Watchdog 自动续期
  - MySQL 版本号 CAS 防止静默覆盖
- 当前新增接口：`GET /api/v1/projects/:id/sync`
- 当前新增项目事件 WebSocket 接口：`GET /api/v1/projects/:id/ws`
  - 支持 `Authorization: Bearer <token>` 或 `?token=<jwt>`
  - 查询参数支持 `cursor` 或 `last_event_id`
  - 消息类型包括 `PROJECT_INIT`、`PROJECT_SYNC`、`TASK_CREATED`、`TASK_UPDATED`、`TASK_DELETED`、`PRESENCE_SNAPSHOT`、`LOCK_REQUEST`、`LOCK_RELEASE`、`TASK_LOCKED`、`TASK_UNLOCKED`、`LOCK_ERROR`、`PING`、`PONG`、`PROJECT_ERROR`
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
- `web/src/pages/ProjectDetailPage.jsx` 已展示项目在线人数；presence 目前是快照感知，不包含光标、选区或正在编辑字段
- `web/src/pages/ProjectDetailPage.jsx` 已消费项目级锁事件；`web/src/components/TaskDetailPanel.jsx` 会在编辑标题、优先级、截止时间等元数据前申请 `metadata` 锁，保存、取消、关闭或切换任务时释放锁；正文 textarea 仍走 Yjs 协同，不受 metadata 锁阻塞
- Swagger 产物还未随本轮接口变化重生成，后续如果要交付接口文档，需要补 `docs/swagger.yaml`、`docs/swagger.json`、`docs/docs.go`

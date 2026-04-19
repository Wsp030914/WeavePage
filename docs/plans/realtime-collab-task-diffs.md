# 实时协同 Task Diff 复盘记录

本文档用于记录每个实现 task 的变更差异，方便逐步 review。总方案仍以 `docs/plans/finallist.md` 为准。

注意：当前 `E:\todolist` 不是 Git 仓库，无法用 `git diff` 自动生成统一 diff。这里按实际 `apply_patch` 改动记录每个 task 的文件级 diff、关键代码差异和验证结果。

## Task 1：将 Yjs CRDT 正文协同提升为主线计划

状态：已完成

变更文件：
- `docs/plans/finallist.md`

Diff 摘要：
- 将目标从“冲突控制、断线恢复和多节点一致性”扩展为“多人在线正文协同编辑、冲突控制、断线恢复和多节点一致性”。
- 将 `content_md` 从 P2 可选增强提升为 P0 核心协同能力。
- 明确算法分层：任务元数据继续走 HTTP + CAS + `task_events`；任务正文走 Yjs CRDT update。
- 新增正文协同后端计划项：`task_content_updates`、task 正文 room、正文 WebSocket handler、`CONTENT_*` 消息协议。
- 新增正文协同前端计划项：`yjs` 依赖、`yjsTaskContentProvider.js`、正文编辑区接入。
- 调整测试清单、简历表述和实施优先级，把正文协同作为后续主线。

## Task 2：后端正文协同数据模型与仓储

状态：已完成

变更文件：
- `AGENTS.md`
- `docs/TODO.md`
- `server/AGENTS.md`
- `server/initialize/mysql.go`
- `server/models/task_content_update.go`
- `server/repo/task_content_repo.go`
- `server/repo/task_content_repo_test.go`
- `server/repo/task_repo.go`
- `server/service/task_service_test.go`

Diff 摘要：
- 新增 `TaskContentUpdate` 模型，用于持久化 Yjs 任务正文 update。
- 新增 `TaskContentRepository`，支持追加 update、按 `message_id` 幂等查询、按 task cursor 查询、查询最新正文快照。
- MySQL 初始化流程新增 `task_content_updates` 自动迁移。
- `TaskRepository` 新增 `UpdateContentSnapshot`，用于刷新 `tasks.content_md` 只读渲染缓存，且不提升任务元数据 `version`。
- 适配已有 `TaskRepository` 测试 stub，避免接口扩展破坏现有 service 测试。
- 更新 `docs/TODO.md`、根 `AGENTS.md`、`server/AGENTS.md`，标记正文 update 表和仓储已落地，WebSocket/Yjs 前端接入仍未完成。

验证：
- `go test ./...` 通过。
- `go test -race ./...` 未通过环境检查：当前机器缺少 `gcc`，`runtime/cgo` 无法构建。

## Task 3：后端正文 WebSocket room、Yjs update 持久化和广播

状态：已完成

变更文件：
- `AGENTS.md`
- `README.md`
- `docs/TODO.md`
- `docs/plans/realtime-collab-task-diffs.md`
- `go.mod`
- `go.sum`
- `server/AGENTS.md`
- `server/handler/content_ws.go`
- `server/main.go`
- `server/middlewares/Auth.go`
- `server/realtime/content_hub.go`
- `server/realtime/content_protocol.go`
- `server/router.go`
- `server/service/auth_service.go`
- `server/service/task_content.go`
- `server/service/task_service.go`
- `server/service/task_service_test.go`

Diff 摘要：
- 引入 `github.com/gorilla/websocket` 作为 WebSocket 实现库，避免自写升级协议和帧处理。
- `AuthService` 新增 `ValidateClaims`，普通 HTTP 鉴权和 WebSocket 握手复用同一套 JTI / token version 校验。
- 新增 `TaskService` 正文协同方法：
  - `OpenTaskContentSession`：校验任务访问权限，区分 owner/editor/viewer。
  - `SyncTaskContentUpdates`：按 cursor 返回 task 正文 Yjs update。
  - `AppendTaskContentUpdate`：校验编辑权限，按 `message_id` 幂等入库，必要时刷新 `tasks.content_md` 快照缓存。
- 新增 `server/realtime/content_protocol.go`，定义 `CONTENT_SYNC`、`CONTENT_INIT`、`CONTENT_UPDATE`、`CONTENT_ACK`、`CONTENT_ERROR` 等消息结构。
- 新增 `server/realtime/content_hub.go`，实现 task 正文 room、本机 WebSocket 广播、连接读写泵、心跳、Redis Pub/Sub 多节点 fan-out。
- 新增 `server/handler/content_ws.go`，暴露 `GET /api/v1/tasks/:id/content/ws`，支持 `Authorization: Bearer <token>` 和 `?token=<jwt>` 两种鉴权入口。
- `server/main.go` 初始化 `TaskContentRepository`、`ContentHub` 并启动 Redis Pub/Sub 订阅。
- `server/router.go` 注册正文 WebSocket 路由。
- 新增服务层测试，覆盖 owner 可编辑、viewer 不可写、正文 update 入库与 snapshot 刷新、正文 cursor 同步分页。
- 同步 README、TODO、AGENTS 和 server 接力文档，标记前端 Yjs provider 仍未实现。

验证：
- `go test ./...` 通过。
- `go test -race ./...` 未通过环境检查：当前机器缺少 `gcc`，`runtime/cgo` 无法构建。
- `go mod tidy` 已执行；第一次沙箱内因构建缓存/模块缓存权限失败，已按权限流程用非沙箱执行成功。

## Task 4：后端项目级 WebSocket room 与任务事件实时广播

状态：已完成

变更文件：
- `AGENTS.md`
- `README.md`
- `docs/TODO.md`
- `docs/plans/realtime-collab-task-diffs.md`
- `server/AGENTS.md`
- `server/handler/content_ws.go`
- `server/handler/ws.go`
- `server/handler/ws_auth.go`
- `server/main.go`
- `server/realtime/hub.go`
- `server/realtime/project_protocol.go`
- `server/realtime/upgrader.go`
- `server/router.go`
- `server/service/task_service.go`
- `server/service/task_service_test.go`
- `server/service/task_sync.go`

Diff 摘要：
- 新增 `GET /api/v1/projects/:id/ws` 项目级任务事件 WebSocket，支持 `Authorization: Bearer <token>` 或 `?token=<jwt>` 自鉴权。
- 新增项目 room 权限入口 `OpenProjectRealtimeSession`，握手阶段复用项目访问校验，避免未授权用户加入 project room。
- 新增 `PROJECT_INIT` / `PROJECT_SYNC` / `TASK_CREATED` / `TASK_UPDATED` / `TASK_DELETED` / `PROJECT_ERROR` 等项目事件协议。
- 新增 `ProjectHub`，支持本机 project room、心跳、连接清理、连接后按 cursor 补偿 `task_events`，并通过 Redis Pub/Sub 做多节点 fan-out。
- `TaskService` 新增 `TaskEventBroadcaster` 钩子，任务 create/update/delete 在事务成功后广播已落库 `task_event`；Kafka 仍只负责可靠异步副作用，不参与实时广播。
- update/delete 在广播前先失效任务详情缓存，降低前端收到实时事件后立即读取旧缓存的概率。
- 抽出 `handler/ws_auth.go` 和 `realtime/upgrader.go`，让正文 WebSocket 和项目 WebSocket 复用握手鉴权与升级逻辑，避免重复实现。
- 新增服务层测试，覆盖任务更新后广播已持久化事件，以及 project room 握手前的项目访问校验。
- 同步根 `AGENTS.md`、`README.md`、`docs/TODO.md`、`server/AGENTS.md`，标记后端 project room 已落地，前端增量 patch store、presence 和 Swagger 重生成仍未完成。

验证：
- `go test ./...` 通过。
- `go test -race ./...` 未通过：默认环境 `CGO_ENABLED=0`。
- `$env:CGO_ENABLED='1'; go test -race ./...` 未通过环境检查：当前机器缺少 `gcc`，`runtime/cgo` 无法构建。

## Task 5：前端项目详情页接入项目事件增量 patch

状态：已完成

变更文件：
- `AGENTS.md`
- `README.md`
- `docs/TODO.md`
- `docs/plans/realtime-collab-task-diffs.md`
- `web/AGENTS.md`
- `web/README.md`
- `web/vite.config.js`
- `web/src/api/task.js`
- `web/src/components/TaskDetailPanel.jsx`
- `web/src/pages/ProjectDetailPage.css`
- `web/src/pages/ProjectDetailPage.jsx`
- `web/src/realtime/projectEventsSocket.js`
- `web/src/realtime/protocol.js`
- `web/src/realtime/socket.js`
- `web/src/realtime/yjsTaskContentProvider.js`
- `web/src/store/collab-store.js`

Diff 摘要：
- 新增 `web/src/realtime/socket.js`，集中封装实时连接的 token、WebSocket URL 和错误消息清洗逻辑。
- `yjsTaskContentProvider.js` 改为复用公共实时连接工具，避免正文 WebSocket 和项目 WebSocket 各自维护一套 token/URL 拼接逻辑。
- 新增 `web/src/realtime/protocol.js`，集中定义 `PROJECT_INIT`、`PROJECT_SYNC`、`TASK_CREATED`、`TASK_UPDATED`、`TASK_DELETED` 等项目事件协议常量。
- 新增 `web/src/realtime/projectEventsSocket.js`，负责连接 `GET /api/v1/projects/:id/ws`、断线重连、处理 `PROJECT_INIT` 补偿分页、分发 `TASK_*` 实时事件并维护 cursor。
- 新增 `web/src/store/collab-store.js`，提供任务 upsert/remove、事件 apply、选中任务同步和乐观版本号 patch helper；页面不直接解析事件 payload。
- `ProjectDetailPage.jsx` 接入项目级 WebSocket：初次仍走 HTTP 快照，之后消费 `PROJECT_INIT` 和 `TASK_*` 事件做本地增量 patch。
- `ProjectDetailPage.jsx` 的任务创建、状态切换、删除和任务详情元数据保存不再成功后强制 `loadData()`；失败时仍回退重新拉取以收敛状态。
- `TaskDetailPanel.jsx` 的元数据保存和状态切换在成功后向父组件回传更新后的 task snapshot；其他页面原有 `loadTasks/loadData` 回调仍兼容。
- `web/src/api/task.js` 新增 `syncProjectEvents()` 客户端，供后续 HTTP 补偿链路复用。
- `web/vite.config.js` 为 `/api` 代理开启 `ws: true`，保证本地开发环境能代理项目事件和正文协同 WebSocket。
- 同步根文档、web 文档和 TODO，明确本轮只完成 `ProjectDetailPage` 增量 patch；`MyTasksPage`、`Next7DaysPage`、`CalendarPage` 等聚合页仍待改。

验证：
- `npm run lint` 首次被 PowerShell execution policy 拦截；改用 `npm.cmd run lint` 后通过。
- `npm.cmd run build` 在默认沙箱内因 esbuild 子进程 `spawn EPERM` 失败。
- 提升权限后 `npm.cmd run build` 通过。

## Task 6：项目级 presence 快照与在线人数展示

状态：已完成

变更文件：
- `AGENTS.md`
- `README.md`
- `docs/TODO.md`
- `docs/plans/realtime-collab-task-diffs.md`
- `server/AGENTS.md`
- `server/handler/ws.go`
- `server/realtime/hub.go`
- `server/realtime/hub_test.go`
- `server/realtime/project_protocol.go`
- `server/service/task_sync.go`
- `web/AGENTS.md`
- `web/README.md`
- `web/src/pages/ProjectDetailPage.css`
- `web/src/pages/ProjectDetailPage.jsx`
- `web/src/realtime/projectEventsSocket.js`
- `web/src/realtime/protocol.js`
- `web/src/store/collab-store.js`

Diff 摘要：
- 项目 WebSocket 协议新增 `PRESENCE_SNAPSHOT`，服务端消息中新增 `presence` 字段。
- `ProjectRealtimeSession` 增加 `Username`，握手 handler 从 JWT claims 中补齐用户名，presence 快照可显示真实用户名。
- `ProjectHub` 在连接加入、离开时生成当前节点 presence 快照并广播给本机 room，同时通过 Redis Pub/Sub fan-out 到其他节点。
- `ProjectHub` 增加 15 秒 presence 心跳，周期发布本节点活跃项目的快照，降低跨节点新连接拿不到远端在线态的概率。
- 新增 `server/realtime/hub_test.go`，覆盖同一用户多连接去重、连接数统计和断开后的快照更新。
- 前端 `projectEventsSocket.js` 增加 `PRESENCE_SNAPSHOT` 分发。
- `collab-store.js` 增加 `mergePresenceSnapshot()` 和 `flattenPresenceUsers()`，按 `server_node_id` 合并多节点快照并按用户去重。
- `ProjectDetailPage.jsx` 显示 `Online: N` 在线人数徽标，tooltip 展示在线用户名列表。
- 同步根文档、server/web 文档和 TODO，明确本轮完成的是项目级在线态快照；协同锁、光标/选区/正在编辑字段仍未实现。

验证：
- `go test ./...` 通过。
- `npm.cmd run lint` 通过。
- `npm.cmd run build` 在默认沙箱内因 esbuild 子进程 `spawn EPERM` 失败。
- 提升权限后 `npm.cmd run build` 通过。
- `go test -race ./...` 未通过：默认环境 `CGO_ENABLED=0`。
- `$env:CGO_ENABLED='1'; go test -race ./...` 未通过环境检查：当前机器缺少 `gcc`，`runtime/cgo` 无法构建。

## Task 7：前端接入项目级 metadata 协同锁

状态：已完成

变更文件：
- `AGENTS.md`
- `README.md`
- `docs/TODO.md`
- `docs/plans/realtime-collab-task-diffs.md`
- `server/AGENTS.md`
- `web/AGENTS.md`
- `web/src/components/TaskDetailPanel.css`
- `web/src/components/TaskDetailPanel.jsx`
- `web/src/components/TaskSection.css`
- `web/src/components/TaskSection.jsx`
- `web/src/pages/ProjectDetailPage.jsx`
- `web/src/realtime/projectEventsSocket.js`
- `web/src/store/collab-store.js`

Diff 摘要：
- `projectEventsSocket.js` 新增 `requestLock()` / `releaseLock()`，复用已有项目 WebSocket 发送 `LOCK_REQUEST` / `LOCK_RELEASE`。
- `collab-store.js` 新增 metadata 锁 key 规范化、锁事件 apply、当前用户/其他用户持锁判断等 helper，避免页面和组件重复解析 `TASK_LOCKED` / `TASK_UNLOCKED`。
- `ProjectDetailPage.jsx` 保存 `locksByKey` 和 lock error 状态，接收项目 WebSocket 的 `TASK_LOCKED`、`TASK_UNLOCKED`、`LOCK_ERROR`，并把当前任务的 metadata 锁传给任务列表和详情面板。
- `TaskSection.jsx` 在任务行展示 `Locked by you` / `Locked by <user>` 标签；当 metadata 锁由其他用户持有时，禁用完成切换和删除按钮，避免直接覆盖写入。
- `TaskDetailPanel.jsx` 在进入元数据编辑时申请 `metadata` 锁；保存、取消、关闭或切换任务时释放锁；锁等待或锁失败时禁用标题、优先级、截止时间保存。
- `TaskDetailPanel.jsx` 保持正文 textarea 不受 metadata 锁阻塞，正文仍通过 Yjs provider 实时协同保存。
- 同步根文档、server/web 文档和 TODO，明确协同锁前后端闭环已落地；聚合页事件驱动状态层、分布式限流、分布式缓存击穿保护和 Swagger 重生成仍待做。

验证：
- `npm.cmd run lint` 通过。
- `npm.cmd run build` 在默认沙箱内因 esbuild 子进程 `spawn EPERM` 失败。
- 提升权限后 `npm.cmd run build` 通过。
- `go test ./...` 通过。

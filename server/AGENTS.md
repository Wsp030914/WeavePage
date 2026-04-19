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

## 当前架构事实

- Kafka 是当前唯一异步事件总线实现
- Redis 同时承担缓存和分布式锁
- 到期提醒依赖独立调度器，不是进程内 cron
- `task_service.go` 已经是业务最重的文件之一
- `project_service.go` 和 `task_service.go` 都已经混用了缓存、锁和 `singleflight`

## 当前非显而易见约束

- 当前限流挂在全局中间件上，先于鉴权执行  
  症状：看起来支持按 `uid` 限流  
  原因：`uid` 在 `AuthMiddleware` 之后才写入 context  
  结论：现在实际上大多数请求还是按 `IP` 限流

- 当前 `singleflight` 只能在单进程内去重  
  症状：单机压测时看起来 DB 压力可控  
  原因：`singleflight.Group` 无法跨节点共享  
  结论：多实例部署后 cache miss 仍可能同时打 DB

- Kafka 当前不负责实时协同广播  
  症状：已有 Kafka，容易自然联想到直接给 WebSocket 用  
  原因：它现在主要用于可靠异步副作用  
  结论：实时协同主链路应走 Redis Pub/Sub + Sync API

- 正文协同不等同于普通任务事件同步  
  症状：`task_events + Sync API` 可以补偿任务元数据变更，看起来也能同步正文  
  原因：多人同时编辑长文本需要合并插入/删除操作，CAS 只能发现冲突，不能自动合并  
  结论：`content_md` 正文主线使用 Yjs CRDT update；服务端只做鉴权、持久化和广播，不自研 CRDT/OT

- Swagger 文档是生成产物  
  症状：改了 handler 或注释后，文档可能还是旧的  
  原因：`docs/swagger.yaml`、`docs/swagger.json`、`docs/docs.go` 不会自动更新  
  结论：接口变化时要明确同步或标记需重生成

## 重点文件

- `service/task_service.go`
  - 任务 CRUD、成员、到期提醒、缓存回填、锁
- `service/project_service.go`
  - 项目列表缓存和详情缓存
- `service/user_service.go`
  - 注册、登录、资料更新、头像上传
- `service/auth_service.go`
  - token 版本校验和黑名单检查
- `middlewares/ratelimit.go`
  - 当前单机限流实现
- `cache/lock.go`
  - 当前 Redis 分布式锁
- `async/`
  - Kafka producer、consumer、handler
- `realtime/content_hub.go`
  - 任务正文 WebSocket room、本机广播、Redis Pub/Sub fan-out
- `realtime/hub.go`
  - 项目级任务事件 WebSocket room、本机广播、Redis Pub/Sub fan-out、presence 快照和 metadata 协同锁消息
- `realtime/lock_manager.go`
  - 项目级任务 metadata 协同锁；复用 Redis 分布式锁和 Watchdog，连接断开时释放当前连接持有的锁
- `handler/content_ws.go`
  - 正文协同 WebSocket 握手鉴权和任务权限校验
- `handler/ws.go`
  - 项目事件 WebSocket 握手鉴权和项目权限校验

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
- `TaskRepository` 已新增 `UpdateContentSnapshot`，用于后续 CRDT 链路刷新 `tasks.content_md` 只读缓存；该方法不提升任务元数据 `version`。
- `TaskService` 的 create/update/delete 现在会在同一事务里同时写任务数据和事件日志，避免补偿层漏记。
- `cache/lock.go` 已升级为 Redis Watchdog 续期锁；续期通过 Lua 先校验 owner 再 `PEXPIRE`，不要改回盲续期。
- `realtime/lock_manager.go` 已复用 Watchdog 锁做项目级 metadata 协同锁；不要为 WebSocket 锁再写一套独立 Redis owner/TTL 逻辑。
- Swagger 产物还没有随本轮接口变化重新生成；如果继续改接口，记得统一补一次。

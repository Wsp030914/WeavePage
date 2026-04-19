# ToDoList

一个正在从待办系统重定位为 Obsidian + Notion 风格的协同文档工作台。当前包含用户、空间/项目、协同文档、个人日记、会议纪要、轻量待办和到期提醒底座；仓库内同时包含后端 API、前端页面和独立调度器服务。

## 当前功能

- 用户注册、登录、退出登录
- 用户资料更新和头像上传
- 空间/项目列表、创建、重命名、删除
- 空间内文档创建、编辑、删除、详情查看
- Obsidian Daily Notes 风格日记规划：当天 `YYYY-MM-DD.md`，归属“日记”空间，默认非协同
- Notion Meeting Notes 风格会议纪要规划：默认模板，支持多人协作
- 轻量待办模块规划：可创建待办事项，但不作为主要卖点
- 任务成员添加与移除
- 我的任务、未来 7 天、日历、个人资料页面
- 任务到期调度与邮件提醒
- Redis 缓存、Kafka 异步处理、Swagger API 文档
- 项目级任务事件 WebSocket room：基于 `task_events` 推送 `TASK_CREATED` / `TASK_UPDATED` / `TASK_DELETED`
- 项目级在线态感知：项目 WebSocket 推送 `PRESENCE_SNAPSHOT`，项目详情页展示在线人数
- 项目级 metadata 协同锁：任务详情编辑元数据时申请 `metadata` 锁，其他在线用户可看到锁状态并避免覆盖写入
- 正文协同后端底座：Yjs update WebSocket 网关、Redis Pub/Sub fan-out、MySQL update log
- 前端项目详情页增量状态同步：任务创建、状态切换、删除和详情元数据保存会先本地 patch，再通过项目 WebSocket 事件收敛
- 前端聚合页局部增量更新：今日、未来 7 天、日历页的自身任务写操作会先本地 patch，失败或缺少任务快照时再重新拉取
- 前端任务详情正文编辑区已接入 Yjs provider，通过 WebSocket 实时同步正文 update

## 仓库结构

- `server/`
  - Go 后端服务
- `web/`
  - React 前端
- `scheduler/`
  - Redis 驱动的定时调度服务
- `docs/`
  - Swagger 产物与规划文档

## 运行依赖

项目当前依赖以下服务：

- MySQL
- Redis
- Kafka
- Scheduler

生产编排文件已经提供：

- `docker-compose.prod.yml`

## 快速开始

### 方式一：Docker Compose

1. 准备环境变量文件，并将 `ENV_FILE` 指向它  
   `docker-compose.prod.yml` 默认读取 `${ENV_FILE:-/opt/todolist/secrets/.env.prod}`
2. 启动服务  
   `docker compose -f docker-compose.prod.yml up --build`
3. 访问：
   - 前端：`http://localhost/`
   - 后端 Swagger：`http://localhost:8080/swagger/index.html`

### 方式二：本地开发

建议先自己启动 MySQL、Redis、Kafka，再分别启动 3 个服务。

后端：

```bash
go run ./server
```

调度器：

```bash
go run ./scheduler
```

前端：

```bash
cd web
npm install
npm run dev
```

Vite 开发代理已配置为：

- `/api` -> `http://localhost:8080`

## 配置说明

后端配置由 `server/config.yml` 和 `TODO_*` 环境变量共同决定。

`server/config.yml`、`secrets/`、本地 `.env*`、`.vscode/`、`log/`、`web/node_modules/` 和 `web/dist/` 已通过 `.gitignore` 排除，不应提交到远端仓库。需要共享配置结构时，新增不含真实密钥的 example 文件，而不是提交本机配置或生产密钥。

常见环境变量包括：

- `TODO_CONFIG_FILE`
- `TODO_JWT_SECRET`
- `TODO_MYSQL_PASSWORD`
- `TODO_REDIS_PASSWORD`
- `TODO_DUE_SCHEDULER_CALLBACK_TOKEN`

当前 `server/config.yml` 中仍包含 `MUST_SET_IN_ENV` 占位值；在 `release` 模式下，这些占位值不能直接用于生产。

## 当前架构要点

- 后端主服务使用 Gin 暴露 `/api/v1` 和内部调度回调接口
- 调度器通过 HTTP 回调 `/api/internal/scheduler/task-due`
- Kafka 当前用于到期提醒、头像/COS 清理、Token 版本缓存等异步副作用
- Redis 当前用于缓存和分布式锁
- Redis Pub/Sub 当前用于项目级任务事件和任务正文 update 的多节点实时 fan-out

## 当前限制

- `ProjectDetailPage` 已接入项目级 WebSocket 增量 patch；`MyTasksPage`、`Next7DaysPage`、`CalendarPage` 已减少自身写操作后的 reload，但暂未订阅跨项目实时事件
- 限流是单机实现
- `singleflight` 也是单机语义
- 聚合页仍通过 HTTP 快照初始化，跨项目远端变更暂不实时推送

实时协同改造方案见：

- `docs/plans/finallist.md`
- `docs/plans/obsidian-notion-product-redesign.md`
## 最近新增的协同基础能力

- 任务更新链路已支持 `version + expected_version + CAS`，用于拦截陈旧写入。
- Redis 分布式锁已升级为带 Watchdog 的自动续期实现，降低长编辑会话下的锁误过期风险。
- 后端已新增 `task_events` 事件日志表和 `GET /api/v1/projects/:id/sync` 增量同步接口，为后续 WebSocket 实时协同提供断线补偿基础。
- 后端已新增 `GET /api/v1/projects/:id/ws` 项目事件 WebSocket：支持 JWT 自鉴权、项目权限校验、连接后 `PROJECT_INIT` 补偿、本机 room 广播和 Redis Pub/Sub 多节点 fan-out。
- 前端已新增项目事件客户端和本地 patch store，`ProjectDetailPage` 会消费 `PROJECT_INIT` / `TASK_CREATED` / `TASK_UPDATED` / `TASK_DELETED`，减少任务写操作后的整页重新拉取。
- 项目事件 WebSocket 已支持 `PRESENCE_SNAPSHOT` 在线态快照；前端会合并各节点快照并在项目详情页显示在线人数。
- 项目事件 WebSocket 已支持任务 metadata 协同锁；前端项目详情页会展示 `TASK_LOCKED` / `TASK_UNLOCKED`，任务详情面板在编辑元数据前申请锁，并在保存、取消、关闭或切换任务时释放锁。
- 前端 Today、Next 7 Days 和 Calendar 聚合页已复用本地 patch helper：任务状态切换、删除和详情元数据保存成功后不再强制整页重新拉取；成员变更等没有 task snapshot 的路径仍重新拉取兜底。
- 后端已新增 `GET /api/v1/tasks/:id/content/ws` 正文协同 WebSocket：支持 JWT 鉴权、task room、本机广播、Redis Pub/Sub 多节点 fan-out、Yjs update 入库和 `message_id` 幂等 ACK。
- 前端 `TaskDetailPanel` 正文 textarea 已接入 Yjs，正文输入会实时写入正文 WebSocket；任务详情 Save 只更新标题、优先级、截止时间等元数据。

> 说明：本仓库已经具备“实时协同底座”的一部分，但还没有完成聚合页跨项目实时事件订阅、分布式限流、分布式缓存击穿保护和 Swagger 产物重生成。

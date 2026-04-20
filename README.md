# WeavePage

Collaborative Markdown workspace for spaces, live documents, lightweight todos, and knowledge workflows.

![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)
![React](https://img.shields.io/badge/React-19-61DAFB?logo=react&logoColor=20232a)
![Vite](https://img.shields.io/badge/Vite-7-646CFF?logo=vite&logoColor=white)
![Yjs](https://img.shields.io/badge/Yjs-CRDT-f7df1e)
![MySQL](https://img.shields.io/badge/MySQL-required-4479A1?logo=mysql&logoColor=white)
![Redis](https://img.shields.io/badge/Redis-required-DC382D?logo=redis&logoColor=white)
![Kafka](https://img.shields.io/badge/Kafka-required-231F20?logo=apachekafka&logoColor=white)

English | [中文](#中文)

---

## English

### What is WeavePage?

WeavePage is a full-stack collaborative document workspace. It combines a Go API server, a React web app, and a Redis-backed scheduler to support authenticated spaces, Markdown documents, real-time collaboration, presence, metadata locks, and lightweight todo workflows.

The project is evolving from a traditional todo app into an Obsidian + Notion inspired workspace: spaces organize documents, documents can be edited collaboratively, todos stay lightweight, and daily notes / meeting notes are product-level concepts being built on the same foundation.

### Features

- Spaces: create, rename, delete, and browse project-like document spaces.
- Documents: create, update, delete, open details, and manage metadata with optimistic version checks.
- Markdown import: create larger Markdown documents through resumable chunk upload, attach image assets, and rewrite local image references to object-storage URLs.
- Collaborative/private document blocks: the space page separates Notion-style shared documents and private documents.
- Daily Notes: open or create today's `YYYY-MM-DD.md` from the sidebar, navigate to previous/next day from the detail panel, then save diary body content through the plain Markdown API.
- Meeting Notes: create a collaborative note with a default meeting template from the sidebar, then convert action items into lightweight todos.
- Search: top-level workspace search for spaces plus backend document, meeting, and todo results through `GET /api/v1/search`.
- Live Markdown body: Yjs CRDT updates over WebSocket, persisted through MySQL update logs.
- Realtime project stream: WebSocket project room for `TASK_CREATED`, `TASK_UPDATED`, `TASK_DELETED`, `PROJECT_INIT`, and sync compensation.
- Presence: project-level online user snapshots plus lightweight document-viewing presence through `PRESENCE_SNAPSHOT`.
- Metadata lock: collaborative lock flow for title, priority, reminder time, and other metadata fields.
- Realtime metrics: authenticated local-node metrics for project/content WebSocket rooms, connections, broadcasts, Pub/Sub, dropped clients, content updates, and lock errors.
- Comments: document-level comments for documents, meeting notes, and todos, with reserved selection anchor text; diary documents intentionally reject comments.
- Trash: deleting a task/document or space now moves it to trash first, with dedicated restore and permanent-delete APIs.
- Todos: today, next 7 days, and calendar views remain available as lightweight action views.
- Scheduler: standalone Redis scheduler for due reminders and server callbacks.
- Async side effects: Kafka producer / consumer flow with retry and DLQ support.
- Cache warmup: Redis project summary cache is filled asynchronously after list/search DB reads and invalidated by per-user summary versions.
- API docs: Swagger output is stored in `docs/`.

### Architecture

```text
WeavePage/
├── server/       # Go + Gin API, MySQL repositories, Redis cache/locks, Kafka, WebSocket hubs
├── web/          # React + Vite frontend, Yjs provider, project event socket, workspace UI
├── scheduler/    # Standalone Redis-backed job scheduler and callback worker
├── docs/         # Swagger artifacts, TODO, and implementation plans
└── .agents/      # Project agent skills for docs, commits, branches, and PR workflows
```

### Runtime Model

```text
Browser
  ├─ HTTP /api/v1
  ├─ WS /api/v1/projects/:id/ws
  └─ WS /api/v1/tasks/:id/content/ws
        │
        ▼
Go API Server ── MySQL
   │  │           ├─ projects / tasks
   │  │           ├─ task_events
   │  │           └─ task_content_updates
   │  │
   │  ├─ Redis cache + distributed locks + Pub/Sub fan-out
   │  └─ Kafka async events / retry / DLQ
   │
   ▼
Scheduler ── Redis sorted-set jobs ── callback to API server
```

### Quick Start

#### Prerequisites

- Go 1.26+
- Node.js 20+
- MySQL
- Redis
- Kafka

#### Clone

```bash
git clone git@github.com:Wsp030914/WeavePage.git
cd WeavePage
```

#### Backend

Create a local config file outside version control, or point `TODO_CONFIG_FILE` to your own config path.

```bash
# Windows PowerShell example
$env:TODO_CONFIG_FILE="E:\path\to\config.yml"
go run ./server
```

The API server listens on port `8080` by default and exposes:

- API: `http://localhost:8080/api/v1`
- Swagger: `http://localhost:8080/swagger/index.html`
- Markdown import: `http://localhost:8080/api/v1/documents/imports`
- Daily Notes: `POST http://localhost:8080/api/v1/diary/today`
- Diary content save: `PATCH http://localhost:8080/api/v1/documents/:id/content`
- Meeting Notes: `POST http://localhost:8080/api/v1/meetings`
- Comments: `GET/POST http://localhost:8080/api/v1/documents/:id/comments`, `PATCH/DELETE http://localhost:8080/api/v1/comments/:id`
- Trash: `GET http://localhost:8080/api/v1/trash/tasks`, `POST http://localhost:8080/api/v1/trash/tasks/:id/restore`, `DELETE http://localhost:8080/api/v1/trash/tasks/:id`, soft delete via `DELETE http://localhost:8080/api/v1/tasks/:id`
- Realtime metrics: `GET http://localhost:8080/api/v1/realtime/metrics`

#### Scheduler

```bash
go run ./scheduler
```

The scheduler listens on port `9090` by default and stores jobs in Redis.

#### Frontend

```bash
cd web
npm install
npm run dev
```

The Vite dev server proxies `/api` to `http://localhost:8080` and enables WebSocket proxying for collaboration channels.

### Configuration

Server configuration is loaded from `TODO_CONFIG_FILE` first. If it is not set, the server looks for `config.yml` in the current directory, `./server`, and `./..`.

Environment variables use the `TODO_` prefix. Common examples:

```bash
TODO_CONFIG_FILE=/absolute/path/to/config.yml
TODO_JWT_SECRET=change-me
TODO_MYSQL_PASSWORD=change-me
TODO_REDIS_PASSWORD=change-me
TODO_DUE_SCHEDULER_CALLBACK_TOKEN=change-me
```

Sensitive local files are intentionally ignored:

- `server/config.yml`
- `secrets/`
- `.env*`
- `.vscode/`
- `log/`
- `web/node_modules/`
- `web/dist/`

If you need to share configuration shape, add an example file without real credentials.

### Development

```bash
# Backend tests
go test ./...

# Frontend lint
cd web
npm run lint

# Frontend production build
npm run build
```

### Current Status

WeavePage already has the core realtime collaboration foundation: project event WebSocket, document body WebSocket, Yjs update persistence, Redis Pub/Sub fan-out, sync API compensation, presence snapshots, metadata locks, and local-node realtime metrics.

The frontend now exposes the main product IA directly from the sidebar: Spaces, Daily Notes, Meetings, Search, and lightweight Todo views. Search currently reuses existing project/task list APIs rather than a dedicated full-text index.

Daily Notes and Meeting Notes now both have product-semantic entries: `POST /api/v1/diary/today` opens today's private diary document, `PATCH /api/v1/documents/:id/content` saves diary Markdown without Yjs, and `POST /api/v1/meetings` creates a collaborative meeting note with a default template.

Document comments are also live in the detail panel: `GET/POST /api/v1/documents/:id/comments` and `PATCH/DELETE /api/v1/comments/:id` support comment list, create, resolve, reopen, and delete for documents, meetings, and todos, while diary comments are rejected by the backend.

Known closure gaps are tracked in `docs/plans/2026-04-20-plan-closure.md`: multi-node performance testing is intentionally skipped for now, Search is not a backend full-text index yet, and trash recovery currently covers task/document/todo records rather than full space recovery.

---

## 中文

### WeavePage 是什么？

WeavePage 是一个全栈协同文档工作台。它由 Go API 服务、React 前端和基于 Redis 的独立调度器组成，支持登录鉴权、空间、Markdown 文档、实时协同、在线态、元数据协同锁和轻量待办工作流。

项目正在从传统待办应用演进为 Obsidian + Notion 风格的工作台：用空间组织文档，用实时协同编辑正文，用轻量待办承接行动项，并在同一底座上继续建设日记和会议纪要能力。

### 功能特性

- 空间：创建、重命名、删除和浏览项目式文档空间。
- 文档：创建、更新、删除、打开详情，并通过版本号保护元数据写入。
- Markdown 导入：通过可恢复分片上传创建较大的 Markdown 文档，支持上传图片资源并把本地图片引用改写为对象存储 URL。
- 协作/私人文档块：空间页按 Notion 风格区分协作文档入口和私人文档入口。
- 日记：侧边栏可打开或创建当天 `YYYY-MM-DD.md`，详情页可前后切换日期，正文通过 plain Markdown 接口保存。
- 会议纪要：侧边栏可创建默认模板的协作会议纪要并自动打开，详情页可把行动项转成轻量 todo。
- 搜索：顶层 Search 入口支持搜索空间，并通过后端 `GET /api/v1/search` 搜索当前用户空间内的文档、会议纪要和待办。
- 实时 Markdown 正文：基于 Yjs CRDT，通过 WebSocket 传输，并持久化到 MySQL update log。
- 项目事件流：项目 WebSocket room 推送 `TASK_CREATED`、`TASK_UPDATED`、`TASK_DELETED`、`PROJECT_INIT` 和断线补偿事件。
- 在线态：通过 `PRESENCE_SNAPSHOT` 展示项目级在线用户快照，并支持轻量文档查看 presence。
- 元数据锁：标题、优先级、提醒时间等元数据编辑会走协同锁，降低多人覆盖写入风险。
- 实时指标：鉴权后可查看当前节点 project/content WebSocket room、连接、广播、Pub/Sub、丢弃连接、正文 update 和锁错误计数。
- 评论：普通文档、会议纪要和待办支持文档级评论，并预留选区锚点文本；日记默认不开放评论。
- 待办：今日、未来 7 天和日历视图作为轻量行动模块保留。
- 调度器：独立 Redis 调度服务，用于到期提醒和后端回调。
- 异步副作用：Kafka producer / consumer，支持重试和 DLQ。
- 缓存预热：项目摘要缓存会在列表 / 搜索回源后异步写入，并通过用户级版本号失效旧摘要。
- API 文档：Swagger 产物保存在 `docs/`。

### 架构

```text
WeavePage/
├── server/       # Go + Gin API、MySQL 仓储、Redis 缓存/锁、Kafka、WebSocket hubs
├── web/          # React + Vite 前端、Yjs provider、项目事件 socket、工作台 UI
├── scheduler/    # 独立 Redis 定时任务调度器
├── docs/         # Swagger 产物、TODO 和实施计划
└── .agents/      # 项目内 agent skills：文档、提交、分支、PR 工作流
```

### 运行模型

```text
Browser
  ├─ HTTP /api/v1
  ├─ WS /api/v1/projects/:id/ws
  └─ WS /api/v1/tasks/:id/content/ws
        │
        ▼
Go API Server ── MySQL
   │  │           ├─ projects / tasks
   │  │           ├─ task_events
   │  │           └─ task_content_updates
   │  │
   │  ├─ Redis cache + distributed locks + Pub/Sub fan-out
   │  └─ Kafka async events / retry / DLQ
   │
   ▼
Scheduler ── Redis sorted-set jobs ── callback to API server
```

### 快速开始

#### 前置要求

- Go 1.26+
- Node.js 20+
- MySQL
- Redis
- Kafka

#### 克隆仓库

```bash
git clone git@github.com:Wsp030914/WeavePage.git
cd WeavePage
```

#### 后端

请使用本地配置文件，或通过 `TODO_CONFIG_FILE` 指向你的配置路径。真实配置不要提交到仓库。

```bash
# Windows PowerShell 示例
$env:TODO_CONFIG_FILE="E:\path\to\config.yml"
go run ./server
```

默认端口：

- API：`http://localhost:8080/api/v1`
- Swagger：`http://localhost:8080/swagger/index.html`
- Markdown 导入：`http://localhost:8080/api/v1/documents/imports`
- 今日日记：`POST http://localhost:8080/api/v1/diary/today`
- 指定日期日记：`POST http://localhost:8080/api/v1/diary/:date`
- 日记正文保存：`PATCH http://localhost:8080/api/v1/documents/:id/content`
- 会议纪要：`POST http://localhost:8080/api/v1/meetings`
- 会议行动项转 todo：`POST http://localhost:8080/api/v1/meetings/:id/actions`
- 搜索：`GET http://localhost:8080/api/v1/search?q=keyword`
- 评论：`GET/POST http://localhost:8080/api/v1/documents/:id/comments`，`PATCH/DELETE http://localhost:8080/api/v1/comments/:id`
- 回收站：`GET http://localhost:8080/api/v1/trash/tasks`，`POST http://localhost:8080/api/v1/trash/tasks/:id/restore`，`DELETE http://localhost:8080/api/v1/trash/tasks/:id`；`GET http://localhost:8080/api/v1/trash/spaces`，`POST http://localhost:8080/api/v1/trash/spaces/:id/restore`，`DELETE http://localhost:8080/api/v1/trash/spaces/:id`；`DELETE http://localhost:8080/api/v1/tasks/:id` 和 `DELETE http://localhost:8080/api/v1/projects/:id` 为软删除
- 实时指标：`GET http://localhost:8080/api/v1/realtime/metrics`

#### 调度器

```bash
go run ./scheduler
```

调度器默认监听 `9090`，并将任务存储在 Redis 中。

#### 前端

```bash
cd web
npm install
npm run dev
```

Vite 开发服务器会把 `/api` 代理到 `http://localhost:8080`，并已启用 WebSocket 代理以支持实时协同连接。

### 配置

服务端优先读取 `TODO_CONFIG_FILE`。如果没有设置，会在当前目录、`./server` 和 `./..` 查找 `config.yml`。

环境变量使用 `TODO_` 前缀，常见示例：

```bash
TODO_CONFIG_FILE=/absolute/path/to/config.yml
TODO_JWT_SECRET=change-me
TODO_MYSQL_PASSWORD=change-me
TODO_REDIS_PASSWORD=change-me
TODO_DUE_SCHEDULER_CALLBACK_TOKEN=change-me
```

以下本地配置、密钥和运行产物已被忽略，不应上传：

- `server/config.yml`
- `secrets/`
- `.env*`
- `.vscode/`
- `log/`
- `web/node_modules/`
- `web/dist/`

如果需要共享配置结构，请新增不包含真实密钥的 example 文件。

### 开发命令

```bash
# 后端测试
go test ./...

# 前端检查
cd web
npm run lint

# 前端构建
npm run build
```

### 当前状态

WeavePage 已经具备实时协同底座：项目事件 WebSocket、正文 WebSocket、Yjs update 持久化、Redis Pub/Sub 多节点 fan-out、Sync API 断线补偿、在线态快照、元数据协同锁和本节点实时链路指标。

前端主信息架构已经从侧边栏直接呈现为 Spaces、Daily Notes、Meetings、Search 和轻量 Todo 视图。当前 Search 已改为后端聚合接口 `GET /api/v1/search`，但仍是 DB LIKE 查询，不是专门的全文索引。

日记和会议纪要都已有产品语义入口：`POST /api/v1/diary/today` 会打开当天私人日记，`POST /api/v1/diary/:date` 支持前后一天导航，`PATCH /api/v1/documents/:id/content` 会以 plain Markdown 保存日记正文且不写 Yjs update log，`POST /api/v1/meetings` 会创建带默认模板的协作会议纪要，`POST /api/v1/meetings/:id/actions` 可把会议行动项转为同空间 todo。

文档评论也已接入详情面板：`GET/POST /api/v1/documents/:id/comments` 和 `PATCH/DELETE /api/v1/comments/:id` 支持普通文档、会议纪要和待办的评论列表、创建、解决/重开与删除；创建评论可带 `anchor_type` / `anchor_text` 预留选区锚点；日记评论会被后端直接拒绝。

当前收口缺口记录在 `docs/plans/2026-04-20-plan-closure.md`：多节点性能测试暂按要求跳过；Search 已有后端聚合接口但还不是全文索引；Yjs 光标/选区 awareness、完整行内评论、会议结构化字段和日记标签/双链仍是后续增强。

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
- Live Markdown body: Yjs CRDT updates over WebSocket, persisted through MySQL update logs.
- Realtime project stream: WebSocket project room for `TASK_CREATED`, `TASK_UPDATED`, `TASK_DELETED`, `PROJECT_INIT`, and sync compensation.
- Presence: project-level online user snapshots through `PRESENCE_SNAPSHOT`.
- Metadata lock: collaborative lock flow for title, priority, reminder time, and other metadata fields.
- Todos: today, next 7 days, and calendar views remain available as lightweight action views.
- Scheduler: standalone Redis scheduler for due reminders and server callbacks.
- Async side effects: Kafka producer / consumer flow with retry and DLQ support.
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

WeavePage already has the core realtime collaboration foundation: project event WebSocket, document body WebSocket, Yjs update persistence, Redis Pub/Sub fan-out, sync API compensation, presence snapshots, and metadata locks.

Daily Notes and Meeting Notes are product concepts built on top of this foundation. The current repository still uses the legacy `projects` and `tasks` data model internally while the UI and product semantics move toward spaces and documents.

---

## 中文

### WeavePage 是什么？

WeavePage 是一个全栈协同文档工作台。它由 Go API 服务、React 前端和基于 Redis 的独立调度器组成，支持登录鉴权、空间、Markdown 文档、实时协同、在线态、元数据协同锁和轻量待办工作流。

项目正在从传统待办应用演进为 Obsidian + Notion 风格的工作台：用空间组织文档，用实时协同编辑正文，用轻量待办承接行动项，并在同一底座上继续建设日记和会议纪要能力。

### 功能特性

- 空间：创建、重命名、删除和浏览项目式文档空间。
- 文档：创建、更新、删除、打开详情，并通过版本号保护元数据写入。
- 实时 Markdown 正文：基于 Yjs CRDT，通过 WebSocket 传输，并持久化到 MySQL update log。
- 项目事件流：项目 WebSocket room 推送 `TASK_CREATED`、`TASK_UPDATED`、`TASK_DELETED`、`PROJECT_INIT` 和断线补偿事件。
- 在线态：通过 `PRESENCE_SNAPSHOT` 展示项目级在线用户快照。
- 元数据锁：标题、优先级、提醒时间等元数据编辑会走协同锁，降低多人覆盖写入风险。
- 待办：今日、未来 7 天和日历视图作为轻量行动模块保留。
- 调度器：独立 Redis 调度服务，用于到期提醒和后端回调。
- 异步副作用：Kafka producer / consumer，支持重试和 DLQ。
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

WeavePage 已经具备实时协同底座：项目事件 WebSocket、正文 WebSocket、Yjs update 持久化、Redis Pub/Sub 多节点 fan-out、Sync API 断线补偿、在线态快照和元数据协同锁。

日记和会议纪要是建立在这套底座上的产品语义。当前仓库内部仍复用旧的 `projects` 和 `tasks` 数据模型，UI 与产品表达正在逐步迁移到空间和文档。

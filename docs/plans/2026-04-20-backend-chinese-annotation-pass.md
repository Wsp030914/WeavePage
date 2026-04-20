# Backend Chinese Annotation Pass Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 为 `server/` 与 `scheduler/` 下全部 Go 文件修复真实存在的中文乱码，并补齐中文注释，覆盖文件职责、核心类型、主要函数、关键业务逻辑与实现取舍。

**Architecture:** 采用“先扫描、后分层、再统一验证”的执行方式。先确认源码文件中真实存在的乱码，再按入口层、接口层、服务层、存储层、协同实时层、基础设施层逐批补中文注释，最后统一 `gofmt` 与 `go test ./...` 收尾，确保大规模注释改动不影响编译和行为。

**Tech Stack:** Go, Gin, GORM, Redis, Kafka, WebSocket, Zap, PowerShell, `gofmt`, `go test`

---

### Task 1: 建立后端文件清单与分层改造顺序

**Files:**
- Modify: `docs/plans/2026-04-20-backend-chinese-annotation-pass.md`
- Inspect: `server/**/*.go`
- Inspect: `scheduler/main.go`

**Step 1: 枚举全部后端 Go 文件**

Run: `rg --files server scheduler -g '*.go' | Sort-Object`
Expected: 输出后端与调度器所有 Go 文件，作为全覆盖范围。

**Step 2: 按层归类**

分为：
- 入口与路由：`server/main.go`、`server/router.go`、`scheduler/main.go`
- 配置与初始化：`server/config/*`、`server/initialize/*`
- 中间件与工具：`server/middlewares/*`、`server/utils/*`
- 处理器：`server/handler/*`
- 服务：`server/service/*`
- 仓储与缓存：`server/repo/*`、`server/cache/*`
- 实时协同与异步：`server/realtime/*`、`server/async/*`
- 数据模型与错误：`server/models/*`、`server/errors/*`
- 测试文件：`*_test.go`

**Step 3: 明确注释粒度**

每个文件至少补：
- 文件职责注释
- 核心结构体/接口注释
- 导出函数注释
- 复杂逻辑块注释

简单常量、直白赋值与显而易见的返回不写废话注释。

### Task 2: 修复真实存在的中文乱码

**Files:**
- Modify: `server/**/*.go`
- Modify: `scheduler/main.go`

**Step 1: 扫描真实乱码**

Run: `Get-Content -Encoding utf8 <file>` 对抽样文件做 UTF-8 校验，区分“终端显示问题”和“源码真实乱码”。

**Step 2: 逐文件修复源码中的真实乱码字符串**

重点关注：
- handler 层接口错误信息
- service 层业务错误信息
- 测试用例中文断言
- 注释中已有损坏文本

**Step 3: 每修完一批立即跑局部 `go test`**

Run: `go test ./server/...`
Expected: 保证修正文案不引入语法问题。

### Task 3: 补文件级中文职责注释

**Files:**
- Modify: `server/**/*.go`
- Modify: `scheduler/main.go`

**Step 1: 在每个文件的 `package` 之前或之后补简短中文职责说明**

内容包含：
- 这个文件负责什么
- 在整条业务链路里的位置

**Step 2: 对测试文件补“测试目标”注释**

说明：
- 测的是什么
- 为什么要覆盖这些边界

### Task 4: 补类型与接口级中文注释

**Files:**
- Modify: `server/models/*.go`
- Modify: `server/repo/*.go`
- Modify: `server/service/*.go`
- Modify: `server/cache/*.go`
- Modify: `server/realtime/*.go`
- Modify: `server/async/*.go`

**Step 1: 为核心结构体与接口写注释**

示例对象：
- `TaskService`
- `ProjectService`
- `TaskRepository`
- `ProjectHub`
- `ContentHub`
- `DistributedLock`

**Step 2: 写明抽象边界与设计原因**

例如：
- 为什么 service 层持有 repo/cache/bus
- 为什么 repo 层封装数据库访问
- 为什么实时层单独拆 hub 和 protocol

### Task 5: 补主要函数与关键业务逻辑注释

**Files:**
- Modify: `server/handler/*.go`
- Modify: `server/service/*.go`
- Modify: `server/repo/*.go`
- Modify: `server/realtime/*.go`
- Modify: `server/async/*.go`
- Modify: `server/cache/*.go`
- Modify: `server/initialize/*.go`
- Modify: `server/middlewares/*.go`
- Modify: `scheduler/main.go`

**Step 1: 为主要导出函数补中文说明**

说明：
- 功能是什么
- 输入输出是什么
- 关键约束是什么

**Step 2: 为复杂逻辑块补“为什么这样实现”注释**

重点覆盖：
- 分布式限流
- 缓存击穿保护
- Redis 分布式锁
- 任务版本 CAS
- WebSocket 鉴权与补偿同步
- Yjs update log 与 snapshot 持久化
- 导入会话分片流程
- 调度器回调链路

**Step 3: 注明实现好处**

例如：
- 降低并发覆盖风险
- 避免缓存雪崩
- 支持多节点实时 fan-out
- 支持断线补偿

### Task 6: 统一格式与验证

**Files:**
- Modify: `server/**/*.go`
- Modify: `scheduler/main.go`

**Step 1: 运行格式化**

Run: `gofmt -w <all backend go files>`
Expected: 注释位置、缩进、空行全部规范化。

**Step 2: 运行全量测试**

Run: `go test ./...`
Expected: 所有后端包通过测试，前端目录不因本次后端改动受影响。

**Step 3: 人工复核注释质量**

检查：
- 是否有废话注释
- 是否有注释与代码不一致
- 是否仍残留真实乱码

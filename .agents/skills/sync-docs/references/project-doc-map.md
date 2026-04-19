# 当前项目文档映射

## 1. 真实存在的文档

- `web/README.md`
  - 当前仍是 Vite 默认模板内容，基本不反映本项目真实功能
  - 只要这次工作涉及前端功能、构建方式、运行方式或项目说明，就应考虑重写或补充
- `docs/plans/2026-03-10-remove-redis-stream.md`
  - 旧的实施计划文档
- `docs/plans/finallist.md`
  - 当前实时协同系统的中文总清单
- `docs/swagger.yaml`
- `docs/swagger.json`
- `docs/docs.go`
  - 这三份是后端 API 文档/生成产物，接口变化时需要同步检查

## 2. 当前标准文档基线

- `AGENTS.md`
  - 根目录，当前项目的 AI 接力主文档
- `README.md`
  - 根目录，给用户和开发者看的项目总说明
- `docs/TODO.md`
  - 统一维护进行中/已完成/待确认事项
- `server/AGENTS.md`
  - 记录后端关键路径、坑点、设计决策
- `web/AGENTS.md`
  - 记录前端状态流、接口契约、协同 UI 约束
- `scheduler/AGENTS.md`
  - 记录调度器职责、回调链路、与服务端约束

可选兼容文档：

- `CLAUDE.md`
  - 当前项目不作为主规范文档
  - 只有仓库明确采用时才维护

## 3. 文档与代码的对应关系

### 根目录 `AGENTS.md`

重点从这些地方取信息：

- `server/main.go`
- `server/router.go`
- `server/config.yml`
- `docker-compose.prod.yml`
- `Dockerfile.server`
- `Dockerfile.scheduler`
- `web/package.json`
- `docs/plans/*.md`

应该写的内容：

- 项目整体结构
- 运行/测试命令
- 主要依赖：MySQL、Redis、Kafka、Scheduler、Web 前端
- AI 改代码时的边界与注意事项

### 模块级 `AGENTS.md`

#### `server/AGENTS.md`

重点来源：

- `server/service/*.go`
- `server/repo/*.go`
- `server/middlewares/*.go`
- `server/cache/*.go`
- `server/async/*.go`

应该写的内容：

- 缓存、锁、异步事件、鉴权、限流、接口更新规则
- 症状 -> 原因 -> 解决方案形式的踩坑记录

#### `web/AGENTS.md`

重点来源：

- `web/src/pages/*.jsx`
- `web/src/components/*.jsx`
- `web/src/api/*.js`
- `web/src/store/*`

应该写的内容：

- 页面状态流
- API 调用约束
- 表单、任务详情、协同状态更新的关键约束

#### `scheduler/AGENTS.md`

重点来源：

- `scheduler/main.go`
- `server/service/due_scheduler.go`
- `server/service/task_service.go`

应该写的内容：

- 调度器职责
- 回调入口
- 到期提醒链路与失败处理

### 根目录 `README.md`

重点来源：

- `docker-compose.prod.yml`
- `server/main.go`
- `scheduler/main.go`
- `web/package.json`
- `docs/plans/finallist.md`

应该写的内容：

- 项目简介
- 核心功能
- 本地开发/生产部署方式
- 前后端启动方式
- 依赖服务说明

### `docs/TODO.md`

适合记录：

- 当前在做的功能
- 已完成事项
- 后续待办
- 需补文档/需补测试/需补生成产物

### `docs/swagger.yaml` / `docs/swagger.json` / `docs/docs.go`

需要检查的触发场景：

- 新增路由
- 修改请求参数
- 修改响应结构
- 修改鉴权要求
- 修改 handler 注释导致 Swagger 信息变化

注意：

- 这三份通常属于生成产物
- 如果无法在当前会话中重新生成，不要手改到看似完整；应明确告诉用户“接口已变更，但 Swagger 仍需重新生成”

## 4. 当前项目下的推荐同步顺序

1. 先确认本次代码改动范围
2. 先补齐缺失的标准文档骨架
3. 再更新 `README.md` / 根目录 `AGENTS.md` / 对应模块 `AGENTS.md`
4. 若涉及计划、架构、专项方案，再更新 `docs/plans/*.md` 或新增 `docs/*.md`
5. 若涉及后端 API，再检查 Swagger 生成产物

## 5. 不要做的事

- 不要把模板 README 原样留着
- 不要在 `AGENTS.md` 里抄代码实现细节而不写约束和原因
- 不要为没发生的变更写文档
- 不要忽略缺失标准文档
- 不要把 Swagger 生成产物当成人工主维护文档来瞎改

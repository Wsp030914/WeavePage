# AI 写作与会议工作流实施计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 基于 `eino` 落地 Draft / Continue / Meeting Preview 三条 AI 工作流，只生成预览结果，由用户在当前文档界面手动应用。

**Architecture:** 保持现有 Gin `handler -> service` 外层结构不变，在服务层内新增一层 AI 编排模块，内部构建一个 `eino` 主 graph，并分流到 Draft、Continue、Meeting 三条 workflow。Draft 和 Continue 通过 HTTP 流式输出预览文本，Meeting 返回结构化预览 JSON，前端只负责展示与手动应用，不直接触发自动写库或自动建 todo。

**Tech Stack:** Go、Gin、GORM、`eino`、React、Vite、HTTP 流式响应、Swagger、`go test`、`npm run lint`

---

### Task 1: 增加 AI 配置与服务装配

**Files:**
- Modify: `server/config/config.go`
- Modify: `server/main.go`
- Modify: `go.mod`
- Modify: `go.sum`

**Step 1: 先写失败测试或编译目标**

覆盖：
- `TODO_AI_PROVIDER`
- `TODO_AI_API_KEY`
- `TODO_AI_BASE_URL`
- `TODO_AI_MODEL`

**Step 2: 运行测试，确认当前失败**

Run: `go test ./server/config -run TestLoadConfigAI -count=1`
Expected: FAIL，因为当前不存在 `AIConfig` 结构和对应环境变量绑定。

**Step 3: 写最小实现**

新增：
- `AIConfig` 字段：
  - `Provider`
  - `APIKey`
  - `BaseURL`
  - `Model`
  - `Timeout`
  - `MaxInputChars`
- `bindEnvs` 中的 `TODO_AI_*` 映射
- 默认超时和输入长度上限
- `main.go` 中的 AI 依赖初始化与注入

**Step 4: 再跑测试，确认通过**

Run: `go test ./server/config -run TestLoadConfigAI -count=1`
Expected: PASS

**Step 5: 提交**

```bash
git add server/config/config.go server/main.go go.mod go.sum
git commit -m "feat: add ai configuration and wiring"
```

### Task 2: 定义 AI 请求类型与 Provider 抽象

**Files:**
- Create: `server/service/ai_types.go`
- Create: `server/service/ai_provider.go`
- Create: `server/service/ai_provider_fake.go`
- Test: `server/service/ai_provider_test.go`

**Step 1: 先写失败测试**

覆盖：
- draft 请求必须校验 `title`
- continue 请求必须校验 `instruction`，并要求 `selected_text` 与 `full_context` 至少有一个
- meeting 请求必须要求 `transcript` 或 `notes` 至少有一个
- fake provider 能返回 draft 流式分片和 meeting 结构化结果

**Step 2: 运行测试，确认当前失败**

Run: `go test ./server/service -run TestAIProvider -count=1`
Expected: FAIL，因为当前不存在 AI 请求类型和 provider 接口。

**Step 3: 写最小实现**

新增：
- 请求 DTO：
  - `AIDraftRequest`
  - `AIContinueRequest`
  - `AIMeetingRequest`
- 响应 DTO：
  - `AITextChunk`
  - `AIMeetingPreview`
  - `AIMeetingAction`
- provider 接口方法：
  - `StreamDraft`
  - `StreamContinue`
  - `GenerateMeetingPreview`
- 单测用 fake provider

**Step 4: 再跑测试，确认通过**

Run: `go test ./server/service -run TestAIProvider -count=1`
Expected: PASS

**Step 5: 提交**

```bash
git add server/service/ai_types.go server/service/ai_provider.go server/service/ai_provider_fake.go server/service/ai_provider_test.go
git commit -m "feat: define ai request types and provider interface"
```

### Task 3: 构建 Eino 主 Graph 与 Draft Workflow

**Files:**
- Create: `server/service/ai_graph.go`
- Create: `server/service/ai_draft_workflow.go`
- Create: `server/service/ai_service.go`
- Test: `server/service/ai_draft_workflow_test.go`

**Step 1: 先写失败测试**

覆盖：
- 主 graph 能把 `mode=draft` 路由到 draft workflow
- draft workflow 能从 `title`、`instruction`、`doc_type` 组装 prompt 输入
- 流式分片顺序与模型输出顺序一致

**Step 2: 运行测试，确认当前失败**

Run: `go test ./server/service -run TestAIDraftWorkflow -count=1`
Expected: FAIL，因为当前不存在主 graph 和 draft workflow。

**Step 3: 写最小实现**

新增：
- `AIService.StreamDraft`
- `BuildAIGraph`，先接通 draft 分支
- draft prompt 组装逻辑，明确要求输出 Markdown
- workflow 到 handler 的文本分片适配层

**Step 4: 再跑测试，确认通过**

Run: `go test ./server/service -run TestAIDraftWorkflow -count=1`
Expected: PASS

**Step 5: 提交**

```bash
git add server/service/ai_graph.go server/service/ai_draft_workflow.go server/service/ai_service.go server/service/ai_draft_workflow_test.go
git commit -m "feat: add eino draft workflow"
```

### Task 4: 增加 Continue Workflow，并明确“仅预览”

**Files:**
- Create: `server/service/ai_continue_workflow.go`
- Modify: `server/service/ai_graph.go`
- Modify: `server/service/ai_service.go`
- Test: `server/service/ai_continue_workflow_test.go`

**Step 1: 先写失败测试**

覆盖：
- 主 graph 能把 `mode=continue` 路由到 continue workflow
- continue workflow 能组合 `selected_text`、`full_context`、`instruction`
- 当 `selected_text` 为空时，可退化为基于全文上下文继续写

**Step 2: 运行测试，确认当前失败**

Run: `go test ./server/service -run TestAIContinueWorkflow -count=1`
Expected: FAIL，因为当前 continue workflow 尚未注册。

**Step 3: 写最小实现**

新增：
- continue prompt builder
- 支持以下意图：
  - continue
  - rewrite
  - expand
  - shorten
- 保证输出仍是预览文本，不调用任务更新接口

**Step 4: 再跑测试，确认通过**

Run: `go test ./server/service -run TestAIContinueWorkflow -count=1`
Expected: PASS

**Step 5: 提交**

```bash
git add server/service/ai_continue_workflow.go server/service/ai_graph.go server/service/ai_service.go server/service/ai_continue_workflow_test.go
git commit -m "feat: add eino continue workflow"
```

### Task 5: 增加 Meeting Preview Workflow

**Files:**
- Create: `server/service/ai_meeting_workflow.go`
- Modify: `server/service/ai_graph.go`
- Modify: `server/service/ai_service.go`
- Test: `server/service/ai_meeting_workflow_test.go`

**Step 1: 先写失败测试**

覆盖：
- 主 graph 能把 `mode=meeting` 路由到 meeting workflow
- meeting workflow 会拒绝同时为空的 `transcript` 和 `notes`
- meeting workflow 返回：
  - `minutes_markdown`
  - `summary`
  - `decisions`
  - `actions`

**Step 2: 运行测试，确认当前失败**

Run: `go test ./server/service -run TestAIMeetingWorkflow -count=1`
Expected: FAIL，因为当前不存在 meeting workflow。

**Step 3: 写最小实现**

新增：
- transcript 清洗与拆分 helper
- summary prompt
- decisions 抽取 prompt
- actions 抽取 prompt
- 最终渲染层，把中间结果组合成统一 meeting preview 对象
- 明确限制：workflow 不直接建 todo，也不直接改 meeting 正文

**Step 4: 再跑测试，确认通过**

Run: `go test ./server/service -run TestAIMeetingWorkflow -count=1`
Expected: PASS

**Step 5: 提交**

```bash
git add server/service/ai_meeting_workflow.go server/service/ai_graph.go server/service/ai_service.go server/service/ai_meeting_workflow_test.go
git commit -m "feat: add eino meeting preview workflow"
```

### Task 6: 暴露 AI HTTP 接口

**Files:**
- Create: `server/handler/ai.go`
- Modify: `server/router.go`
- Modify: `server/main.go`
- Test: `server/handler/ai_test.go`

**Step 1: 先写失败测试**

覆盖：
- `POST /api/v1/ai/draft/stream` 需要鉴权并能流式输出
- `POST /api/v1/ai/continue/stream` 能校验请求体
- `POST /api/v1/ai/meetings/generate` 能返回结构化预览 JSON
- 当 `task_id` 不可访问时返回权限错误

**Step 2: 运行测试，确认当前失败**

Run: `go test ./server/handler -run TestAIHandler -count=1`
Expected: FAIL，因为当前 AI handler 和路由尚未注册。

**Step 3: 写最小实现**

新增：
- handler DTO 绑定
- 鉴权保护后的路由：
  - `/api/v1/ai/draft/stream`
  - `/api/v1/ai/continue/stream`
  - `/api/v1/ai/meetings/generate`
- draft 与 continue 的 HTTP 流式输出
- meeting preview 的 JSON 返回
- 当请求中带 `task_id` 时，补访问权限校验

**Step 4: 再跑测试，确认通过**

Run: `go test ./server/handler -run TestAIHandler -count=1`
Expected: PASS

**Step 5: 提交**

```bash
git add server/handler/ai.go server/handler/ai_test.go server/router.go server/main.go
git commit -m "feat: add ai preview endpoints"
```

### Task 7: 增加前端 AI API Client 与预览面板

**Files:**
- Create: `web/src/api/ai.js`
- Create: `web/src/components/AIPreviewPanel.jsx`
- Create: `web/src/components/AIPreviewPanel.css`
- Modify: `web/src/components/TaskDetailPanel.jsx`
- Modify: `web/src/components/TaskDetailPanel.css`

**Step 1: 先写失败的前端行为清单**

覆盖：
- 可从任务详情打开 AI 预览面板
- 可触发 draft 生成
- 可触发 continue 生成
- 会议文档可触发 meeting preview
- 正在生成时可取消请求

**Step 2: 先跑检查，确认当前能力不存在**

Run: `cd web && npm run lint`
Expected: PASS，但当前没有任何 AI UI。

**Step 3: 写最小实现**

新增：
- AI API helper：
  - `streamDraftPreview`
  - `streamContinuePreview`
  - `generateMeetingPreview`
- 预览面板状态：
  - idle
  - streaming
  - complete
  - error
- `TaskDetailPanel` 中的入口动作：
  - `Generate Draft`
  - `Continue`
  - `Rewrite`
  - `Generate Meeting Minutes`

**Step 4: 再跑检查，确认通过**

Run: `cd web && npm run lint`
Expected: PASS

**Step 5: 提交**

```bash
git add web/src/api/ai.js web/src/components/AIPreviewPanel.jsx web/src/components/AIPreviewPanel.css web/src/components/TaskDetailPanel.jsx web/src/components/TaskDetailPanel.css
git commit -m "feat: add ai preview panel and api client"
```

### Task 8: 在编辑器表面补手动应用动作

**Files:**
- Modify: `web/src/components/TaskDetailPanel.jsx`
- Modify: `web/src/components/DocumentMarkdownEditor.jsx`
- Modify: `web/src/components/AIPreviewPanel.jsx`
- Modify: `web/src/components/TaskDetailPanel.css`

**Step 1: 先写失败的手工验证清单**

覆盖：
- 草稿预览能插入到空白文档
- continue 结果能插入到选区后
- AI 结果能替换当前选区
- 会议纪要预览只有在用户点击后才替换整篇内容

**Step 2: 先确认当前能力不存在**

Run: `cd web && npm run lint`
Expected: PASS，但当前还没有手动应用动作。

**Step 3: 写最小实现**

新增：
- 编辑器选区捕获
- 手动应用函数：
  - `insertAtCursor`
  - `replaceSelection`
  - `replaceWholeContent`
- 约束：
  - 不自动保存
  - 不做隐式正文变更
  - 所有应用动作必须来自显式点击

**Step 4: 再跑检查，确认通过**

Run: `cd web && npm run lint`
Expected: PASS

**Step 5: 提交**

```bash
git add web/src/components/TaskDetailPanel.jsx web/src/components/DocumentMarkdownEditor.jsx web/src/components/AIPreviewPanel.jsx web/src/components/TaskDetailPanel.css
git commit -m "feat: add manual apply actions for ai previews"
```

### Task 9: 同步文档并完成验证

**Files:**
- Modify: `AGENTS.md`
- Modify: `README.md`
- Modify: `docs/TODO.md`
- Modify: `server/AGENTS.md`
- Modify: `web/AGENTS.md`
- Modify: `docs/swagger.yaml`
- Modify: `docs/swagger.json`
- Modify: `docs/docs.go`
- Create: `docs/plans/2026-04-20-ai-writing-meeting-design.md`
- Create: `docs/plans/2026-04-20-ai-writing-meeting-implementation.md`

**Step 1: 先写验证清单**

覆盖：
- AI service 和 handler 的后端测试通过
- 前端 lint 通过
- handler 注释完成后重生成 Swagger
- 文档明确写清“仅预览、手动应用”的边界

**Step 2: 运行阶段性验证**

Run: `go test ./server/service ./server/handler -count=1`
Expected: PASS

Run: `cd web && npm run lint`
Expected: PASS

**Step 3: 写最小文档更新**

更新：
- 根文档和模块文档中关于 AI 的入口、边界、接口说明
- Swagger 生成产物
- 设计文档与实施计划文档

**Step 4: 运行最终验证**

Run: `go test ./...`
Expected: PASS

Run: `cd web && npm run lint`
Expected: PASS

**Step 5: 提交**

```bash
git add AGENTS.md README.md docs/TODO.md server/AGENTS.md web/AGENTS.md docs/swagger.yaml docs/swagger.json docs/docs.go docs/plans/2026-04-20-ai-writing-meeting-design.md docs/plans/2026-04-20-ai-writing-meeting-implementation.md
git commit -m "docs: document ai preview workflows"
```

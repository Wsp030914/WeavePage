# AI 写作与会议工作流设计

## 背景

当前项目已经完成协同文档工作台的主骨架：

- 后端已有稳定的 `Gin -> handler -> service -> repo` 分层
- 文档、会议纪要、日记、轻量 todo 统一复用 `tasks` 主模型
- 前端已存在统一的文档详情面板和 Markdown 编辑器
- 会议纪要已有独立语义入口与“行动项转 todo”最小能力

本轮目标是在不破坏现有协同链路的前提下，增加 3 个 AI 能力：

1. 文稿生成 `Draft`
2. 文稿续写/改写 `Continue`
3. 会议纪要整理 `Meeting Workflow`

用户已确认第一阶段边界：

- AI 只生成预览
- 用户手动决定是否应用
- AI 不直接写回文档
- AI 不直接创建 todo

这意味着第一阶段重点是补齐 AI 编排层、模型调用层、流式预览接口和前端预览交互，而不是做自动执行 agent。

## 目标

- 在当前 Go 后端内接入 `eino`，形成独立 AI 编排层
- 用一个主 graph 按 `mode` 路由到三条 workflow
- 为 Draft 和 Continue 提供低延迟流式预览
- 为 Meeting 提供结构化预览结果，供用户手动应用
- 保持现有 Yjs 协同正文、项目 WebSocket、任务 CRUD、会议 action todo 接口不变

## 非目标

- 第一阶段不做 AI 自动保存正文
- 第一阶段不做 AI 自动创建会议行动项 todo
- 第一阶段不做第二阶段的 Agent Entry、Notion 检索、外部任务同步
- 第一阶段不把 AI 输出接入现有 project/content WebSocket 协议
- 第一阶段不在 README 或产品文案中宣称“已支持 AI 自动执行”

## 总体方案

采用“现有业务架构保持不变，AI 能力以内聚模块增量接入”的方式落地。

外层仍然使用当前后端主结构：

- `router.go` 注册 HTTP 接口
- `handler` 做鉴权、参数校验、协议转换、流式响应
- `service` 组装业务依赖

新增一层 AI 模块，内部使用 `eino` 实现 graph 编排：

- `Main Graph / Router`
  - 负责根据 `mode` 分流
- `Draft Workflow`
  - 输入标题与补充指令，输出 Markdown 草稿
- `Continue Workflow`
  - 输入选中文本、全文上下文与编辑指令，输出续写或改写结果
- `Meeting Workflow`
  - 输入 transcript 或 notes，输出会议纪要预览、决策和行动项候选

该方案的核心原则是：

- AI 工作流只生成候选内容
- 现有权威写路径仍然是现有 HTTP 接口
- AI 模块不绕过现有业务约束和权限模型

## 模块分层

建议新增以下后端模块：

- `server/config/config.go`
  - 增加 `ai` 配置段
- `server/service/ai_service.go`
  - 对外提供统一 AI 调用入口
- `server/service/ai_graph.go`
  - 构建 `eino` 主 graph/router
- `server/service/ai_draft_workflow.go`
  - Draft workflow
- `server/service/ai_continue_workflow.go`
  - Continue workflow
- `server/service/ai_meeting_workflow.go`
  - Meeting workflow
- `server/service/ai_types.go`
  - AI 请求/响应类型
- `server/handler/ai.go`
  - HTTP 协议层

前端建议新增：

- `web/src/api/ai.js`
  - AI 接口封装
- `web/src/components/AIPreviewPanel.jsx`
  - 统一预览面板
- `web/src/components/AIPreviewPanel.css`
  - 预览面板样式
- `web/src/components/TaskDetailPanel.jsx`
  - 挂载 AI 按钮、请求状态和应用动作

## Main Graph / Router 设计

`Main Graph` 不承担业务写库职责，只做输入标准化和 workflow 路由。

建议输入结构统一为：

```json
{
  "mode": "draft | continue | meeting",
  "title": "optional",
  "instruction": "optional",
  "selected_text": "optional",
  "full_context": "optional",
  "transcript": "optional",
  "notes": "optional",
  "doc_type": "optional",
  "task_id": 123
}
```

主 graph 的职责：

1. 校验 `mode`
2. 按不同 mode 做输入裁剪
3. 组装 workflow 所需上下文
4. 调用对应 workflow
5. 把 workflow 结果转为统一响应或流式事件

这样可以保持 handler 薄、workflow 专注，也便于第二阶段把 `agent` 作为第四条分支接入。

## Draft Workflow

### 输入

- `title`
- 可选 `instruction`
- 可选 `doc_type`

### 处理链路

1. 清洗标题和指令
2. 生成 prompt template
3. 调用 chat model 流式生成
4. 输出 Markdown 草稿文本

### 输出

```json
{
  "mode": "draft",
  "content": "# ...markdown draft..."
}
```

### 交互原则

- 只展示预览
- 用户可选择插入或复制
- 不自动覆盖已有正文

### 适用场景

- 空白文档起草
- 给普通文档或会议补初稿
- 根据标题快速生成提纲或正文

## Continue Workflow

### 输入

- `selected_text`
- `full_context`
- `instruction`

### 处理链路

1. 标准化输入
2. 用 input lambda 组装选区、上下文和指令
3. 套用 continue/rewrite prompt
4. 流式输出候选文本

### 输出

```json
{
  "mode": "continue",
  "content": "...rewritten or continued content..."
}
```

### 交互原则

- 第一阶段不自动改正文
- 只返回建议文本
- 用户点击后再执行“替换选区”或“插入到选区后”

### 风险与约束

- 当前协作文档正文走 Yjs，AI 自动改写容易与多人同时编辑冲突
- 因此前端必须把“生成”和“应用”分成两个动作
- 如果当前没有选中文本，则该链路默认退化为“基于全文上下文继续写”

## Meeting Workflow

### 输入

- `transcript` 或 `notes`
- 可选 `title`
- 可选 `instruction`

### 处理链路

1. 清洗原始 transcript 或 notes
2. 拆出关键信息段
3. 生成会议摘要
4. 提取决策项
5. 提取行动项候选
6. 渲染为会议纪要 Markdown

### 输出

```json
{
  "mode": "meeting",
  "minutes_markdown": "# 会议纪要 ...",
  "summary": "本次会议主要结论 ...",
  "decisions": [
    "..."
  ],
  "actions": [
    {
      "title": "...",
      "owner_hint": "...",
      "due_hint": "..."
    }
  ]
}
```

### 交互原则

- 会议纪要和行动项只作为候选预览返回
- 用户可以手动把纪要替换或插入到当前 meeting 文档
- 用户可以手动选择单条 action，再调用现有 `/meetings/:id/actions`

### 与现有业务的关系

- 不新增平行会议表
- 不绕过现有 `meeting -> todo` 入口
- 会议 AI 只是“整理和建议”，不是“权威创建”

## API 设计

第一阶段建议保持接口语义清晰，不强制所有能力复用单一端点。

推荐接口：

- `POST /api/v1/ai/draft/stream`
- `POST /api/v1/ai/continue/stream`
- `POST /api/v1/ai/meetings/generate`

设计理由：

- Draft 和 Continue 更适合流式输出
- Meeting 更适合返回完整结构化 JSON
- Swagger、前端 API 封装、问题排查都更直观

如果后续需要统一，也可以增加一个门面：

- `POST /api/v1/ai/generate`

但不应牺牲第一阶段的清晰边界。

## 为什么不复用现有 WebSocket

当前项目已有两类实时链路：

- 项目事件 WebSocket
- 正文协同 WebSocket

它们的职责是：

- 权限校验后的实时广播
- room fan-out
- 协同同步与补偿

AI 请求的语义不同：

- 一次请求对应一次结果流
- 不需要 room 广播
- 不需要 Redis Pub/Sub fan-out
- 也不属于协同协议的一部分

因此第一阶段 AI 输出应走独立 HTTP 流式响应，不接入现有 project/content WS 协议。

## 前端交互设计

AI 入口挂在 `TaskDetailPanel`，按文档类型暴露不同操作：

- 普通文档
  - Generate Draft
  - Continue
  - Rewrite
- 会议纪要
  - Generate Draft
  - Continue
  - Generate Meeting Minutes
- 日记
  - 可选支持 Draft/Continue
  - 不做多人协同假设

预览面板需要支持：

- 生成中状态
- 取消生成
- 错误提示
- 结果预览
- 复制
- 插入到当前位置
- 替换当前选区
- 替换整个 meeting 内容

第一阶段不要求做复杂的 diff 视图，直接展示 AI 结果即可。

## 权限、限流与安全

AI 接口继续走现有鉴权体系：

- 需要登录
- 只能对当前用户可访问的文档触发生成

额外约束：

- 对 `task_id` 做访问权限校验
- 对 transcript、context 长度设置上限
- 对流式接口设置超时和取消
- 记录请求耗时、失败原因、模型错误类型

第一阶段不做持久化 prompt 日志或 AI 生成历史表，避免过早扩模型。

## 错误处理

需要明确区分以下错误：

- 参数错误
  - 缺标题、缺 transcript、mode 不合法
- 权限错误
  - 用户不可访问该文档
- 模型配置错误
  - API key/base URL/model 未配置
- 模型调用错误
  - 超时、限流、上游 5xx
- 中途取消
  - 用户关闭面板或取消生成

前端不应把这些错误直接映射成“保存失败”，而应明确标成“AI 生成失败”。

## 测试策略

第一阶段重点测试以下层级：

- service/workflow 单测
  - 输入映射是否正确
  - workflow 路由是否正确
- handler 单测
  - 参数校验
  - 鉴权
  - 流式输出协议
- 前端组件测试或最小人工 checklist
  - 能否发起请求
  - 能否取消
  - 能否把预览手动应用到编辑器

不建议第一版把测试依赖真实模型服务。应通过 fake provider 或 mock runner 隔离。

## 分阶段实施顺序

### Phase 1

- 补 `ai` 配置
- 引入 `eino`
- 落地 `AIService` 和主 graph

### Phase 2

- 打通 Draft workflow
- 前端接入 Draft 预览

### Phase 3

- 打通 Continue workflow
- 前端补选区应用动作

### Phase 4

- 打通 Meeting workflow
- 前端支持会议纪要和行动项候选预览

### Phase 5

- 补测试
- 补 Swagger
- 补文档
- 视情况进入第二阶段 Agent Entry

## 当前结论

在“只生成预览，用户手动应用”的边界下，当前项目非常适合以 `eino` 为核心新增一层 AI 编排能力。

最重要的设计决策有三个：

1. AI 层是增量模块，不接管现有业务主架构
2. AI 输出走独立 HTTP 接口，不复用现有协同 WebSocket
3. 第一阶段严格禁止 AI 自动写回文档和自动创建 todo

按这个设计落地，可以在不破坏现有协同能力的前提下，先把高频 AI 写作与会议整理场景接进来，并为第二阶段的 agent 能力预留统一入口。

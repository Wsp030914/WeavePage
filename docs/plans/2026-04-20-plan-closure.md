# 2026-04-20 Plan 收口记录

## 结论

当前项目已经基本满足 `docs/plans/finallist.md` 和 `docs/plans/obsidian-notion-product-redesign.md` 中的 MVP 主线要求：产品形态已经从待办 CRUD 转为协同文档工作台，P0 核心协同与 P1 协同体验 / 分布式基础设施的主要功能已经落地。

本轮按用户要求不执行多节点压测、WebSocket 长连压测或其它性能测试，因此性能验证不纳入本次收口完成范围。

## 已收口范围

- 产品信息架构：侧边栏和主要页面已收敛为 Spaces / Docs / Daily Notes / Meetings / Search / Todos / Trash。
- 普通文档：空间页支持协作文档块和私人文档块，文档详情以 Markdown 正文为核心。
- 日记：`POST /api/v1/diary/today` 可幂等创建或打开当天 `YYYY-MM-DD.md`，`POST /api/v1/diary/:date` 支持前一天 / 后一天导航，正文通过 `PATCH /api/v1/documents/:id/content` plain Markdown 保存，不接入多人 Yjs。
- 会议纪要：`POST /api/v1/meetings` 可创建默认模板的协作会议纪要，`POST /api/v1/meetings/:id/actions` 提供行动项转 todo 的最小入口。
- 轻量待办：空间内可快速创建 todo，Today / Next 7 Days / Calendar 作为次级 todo 视图保留。
- 正文协同：CodeMirror + Yjs、正文 WebSocket、Redis Pub/Sub fan-out、`task_content_updates` update log、正文快照缓存路径已落地。
- 元数据一致性：`tasks.version` / `expected_version` / CAS、Redis metadata 锁、Sync API、`task_events` 已落地。
- 项目实时事件：项目 WebSocket、`PROJECT_INIT` 补偿、`TASK_*` 广播、项目级 presence 快照已落地。
- 轻量文档查看 presence：项目 WebSocket 支持 `VIEW_DOCUMENT`，presence 快照携带 `viewing_task_ids`，前端详情页展示当前文档查看者。
- 评论：普通文档、会议纪要和待办支持文档级评论；日记拒绝评论；评论创建已预留 `anchor_type` / `anchor_text`，用于后续升级行内或选区锚点。
- 文档 / 待办回收站：`DELETE /api/v1/tasks/:id` 已改为软删除，支持列表、恢复和彻底删除。
- 空间级回收站：`DELETE /api/v1/projects/:id` 已改为软删除，`GET /api/v1/trash/spaces`、`POST /api/v1/trash/spaces/:id/restore`、`DELETE /api/v1/trash/spaces/:id` 支持列表、恢复和彻底删除。
- 搜索：`GET /api/v1/search` 已落地，Search 页文档结果不再逐空间拉取任务列表做前端扫描；当前是 DB LIKE 聚合查询，不是全文索引。
- 分布式基础设施：Redis 分布式限流、缓存击穿保护、锁续期、实时链路指标、项目摘要缓存异步回填已接入。

## 明确未收口范围

- 多节点压测 / 性能测试：按用户要求跳过。
- 后端全文索引：已有 `GET /api/v1/search` 后端聚合接口，但还不是 MySQL FULLTEXT、Elasticsearch、Meilisearch 等全文索引方案。
- Yjs awareness 深化：当前已支持“谁正在查看文档”的轻量 presence，尚未实现正文光标、选区、正在编辑段落等 awareness。
- 完整行内评论：当前已预留 selection anchor 文本，尚未实现可随正文编辑漂移的 CRDT 位置锚点。
- 会议结构化字段：当前已支持行动项转 todo 的最小入口，尚未实现参会人模型、议题模型或会议时间字段。
- 日记高级能力：当前已支持前一天 / 后一天导航，尚未实现日历式导航、标签、双链、反向链接或日记全文搜索。
- 产品语义 API 别名：当前仍大量复用 `projects` / `tasks` API，尚未完全补齐 `POST /documents`、`POST /todos`、通用 `/trash` 等语义化别名。
- 集群级可观测性：`GET /api/v1/realtime/metrics` 是本节点快照，不是 Prometheus 或日志系统层面的集群聚合。

## 验证状态

- `go test ./...` 已通过。
- `cmd /c npm run lint` 已通过。
- `cmd /c npm run build` 已通过。
- `go test -race ./...` 未纳入本轮收口验证。
- 多节点压测、WebSocket 长连压测和性能测试未执行，按本轮要求跳过。

## 后续建议

1. 如需继续搜索能力，优先把 `GET /api/v1/search` 的 DB LIKE 实现替换为全文索引，而不是回到前端扫描。
2. 做一轮端到端冒烟脚本或手工 checklist，覆盖登录、空间、文档、日记、会议、评论、回收站、搜索和协同连接。
3. 如需增强协作体验，下一步优先做 Yjs awareness 光标 / 选区和可随正文编辑漂移的评论锚点。
4. 再按需要进入多节点压测、WebSocket 长连稳定性和 chunk 拆分优化。

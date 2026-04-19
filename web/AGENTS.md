# web 接力说明

## 模块职责

`web/` 是 React + Vite 前端，正在从待办 UI 重定位为 Obsidian + Notion 风格的协同文档工作台，负责：

- 登录 / 注册
- Spaces / Docs
- Daily Notes
- Meetings
- Todos
- 待办次级视图：今日、未来 7 天、日历
- 用户资料

## 当前技术栈

- React 19
- React Router
- Axios
- Yjs
- Vite
- ESLint

## 关键入口

- `src/App.jsx`
  - 前端路由入口
- `src/store/AuthContext.jsx`
  - 登录态
- `src/store/ProjectContext.jsx`
  - 项目数据
- `src/api/client.js`
  - Axios 实例、鉴权头、错误处理

## 当前状态流事实

- 当前已在 `TaskDetailPanel.jsx` 接入任务正文 Yjs 协同 provider；普通文档和会议纪要启用协同，日记跳过 Yjs provider
- `TaskDetailPanel.jsx` 对 `doc_type=diary` 使用 CodeMirror plain Markdown 模式，并通过 `PATCH /api/v1/documents/:id/content` 保存正文
- `ProjectDetailPage.jsx` 已接入项目级 WebSocket 事件流和本地增量 patch store
- `ProjectDetailPage.jsx` 已显示项目级在线人数，来源是 `PRESENCE_SNAPSHOT` 快照
- `ProjectDetailPage.jsx` 已消费项目级 `TASK_LOCKED` / `TASK_UNLOCKED` / `LOCK_ERROR`，任务行和详情面板会展示 metadata 锁状态
- `ProjectDetailPage.jsx` 已新增 Notion 风格协作文档块和私人文档块；两个块都支持快速新建文档、分片导入 `.md/.markdown`、附加图片资源并由后端改写 Markdown 图片引用
- `AppLayout.jsx` 已接入“日记”主导航入口：调用 `api/diary.js` 的 `openTodayDiary()`，成功后刷新空间列表并跳转到 `/projects/:id?task=:taskId`
- `AppLayout.jsx` 已接入“会议”主导航入口：调用 `api/meeting.js` 的 `createMeetingNote()`，成功后创建会议纪要并跳转到 `/projects/:id?task=:taskId`
- `ProjectDetailPage.jsx` 已支持 `?task=<id>` 自动打开指定文档详情，日记入口和后续全局文档入口都可复用这个机制
- `MyTasksPage.jsx`、`Next7DaysPage.jsx`、`CalendarPage.jsx` 已复用本地 patch helper，后续应收敛为 Todos 模块的次级视图
- 新产品主导航应参考 `docs/plans/obsidian-notion-product-redesign.md`，优先 Spaces / Docs / Daily Notes / Meetings，Todos 是轻量辅助模块
- `ProjectDetailPage.jsx` 和 `TaskDetailPanel.jsx` 是当前任务交互的核心路径
- `api/client.js` 统一处理 401、5xx 和基础错误映射

## 当前非显而易见约束

- 前端 API 基地址固定为 `/api/v1`
- Vite 开发代理在 `vite.config.js` 中将 `/api` 转发到 `http://localhost:8080`，并已开启 `ws: true` 以支持项目事件和正文协同 WebSocket
- 登录态依赖 `localStorage` 中的 `access_token`
- 当前 profile 更新接口可能返回新 token
  结论：改用户资料接口时，要同时检查 `updateUserToken` 的调用链

- 当前“我的任务”页并不依赖专门的聚合接口
  症状：页面加载所有项目后再按项目拉任务
  原因：`getTasksAcrossProjects()` 会并发拉多个项目任务
  结论：如果项目数变大，这条链路会先遇到性能问题

- 当前任务详情的元数据保存已带 `expected_version`
  结论：改 `TaskDetailPanel.jsx` 的保存逻辑时，不要移除 `task.version`；版本冲突失败后应让调用页重新拉取或通过实时事件收敛

- 当前项目详情页已经接入 coarse-grained metadata 协同锁
  症状：任务详情编辑标题、优先级、截止时间前会发送 `LOCK_REQUEST`，保存、取消、关闭或切换任务时发送 `LOCK_RELEASE`
  原因：正文使用 Yjs CRDT 自动合并，但任务元数据仍走 HTTP + CAS，锁用于降低多人同时改元数据时的冲突概率
  结论：不要用 metadata 锁阻塞正文 textarea；正文仍通过 `yjsTaskContentProvider.js` 实时同步

- 当前任务正文已经不应再通过普通 `PATCH /projects/:id/tasks/:task_id` 整段保存
  症状：标题、优先级和截止时间仍然点击 Save 保存
  原因：正文 `content_md` 已接入 `web/src/realtime/yjsTaskContentProvider.js`，通过 `GET /api/v1/tasks/:id/content/ws` 实时写 Yjs update
  结论：改 `TaskDetailPanel.jsx` 时不要把 `content_md` 加回普通 `updateTask` payload

- Markdown 导入是会话式分片流程
  症状：本地 `.md` 可能较大且可能引用本地图片
  原因：单次上传不适合大文件和多资源导入
  结论：前端通过 `src/api/documentImport.js` 先创建导入会话，再上传图片资源、按 `chunk_size` 上传 Markdown 分片，最后调用 complete；失败时调用 abort 清理临时对象

- 私人文档入口不是协作者入口
  症状：私人文档和协作文档在空间页是两个块
  原因：后端以 `collaboration_mode=private` 拒绝添加协作者和非 owner 正文访问
  结论：`TaskDetailPanel.jsx` 对私人文档展示说明，不展示 Add Member 表单

- 日记正文已改为 plain Markdown 保存
  症状：侧边栏“日记”已经能打开当天 `YYYY-MM-DD.md`
  原因：`doc_type=diary` 是个人记录，不需要多人正文 CRDT，也不应显示协同锁暗示
  结论：`TaskDetailPanel.jsx` 根据 `doc_type=diary` 跳过 Yjs provider 和 metadata 锁，正文保存走 `saveDocumentContent()`

- 会议纪要默认可以协作
  症状：会议类似 Notion Meeting Notes
  原因：会议需要多人共同记录
  结论：meeting 类型继续复用 Yjs provider，并默认带会议模板

## 当前重点页面

- `pages/ProjectListPage.jsx`
  - 项目列表、创建、重命名、删除
- `pages/ProjectDetailPage.jsx`
  - 项目详情、协作文档块、私人文档块、Markdown 导入、任务列表、`?task=<id>` 自动打开文档
- `components/TaskDetailPanel.jsx`
  - 任务详情编辑、成员管理、优先级和截止时间
- `layouts/AppLayout.jsx`
  - 主导航、日记快捷入口、空间列表
- `pages/MyTasksPage.jsx`
  - 今日任务视图
- `pages/ProfilePage.jsx`
  - 用户资料更新和头像上传

## 后续实时协同改造提醒

- 不要继续用“每次写完都 `loadData()`”作为主同步方案；`ProjectDetailPage.jsx` 已经改为本地 patch + 项目事件 WebSocket 收敛
- 需要新增本地协同 store，统一维护：
  - `tasksByID`
  - `projectTaskIDs`
  - `locks`
  - `presence`
  - `connectionStatus`
  - `lastCursor`
  - `processedMessageIDs`
- `ProjectDetailPage.jsx` 和 `TaskDetailPanel.jsx` 会是协同改造的第一落点
- `realtime/yjsTaskContentProvider.js` 已负责单任务正文的 Yjs 文档、WebSocket 同步、seed 初始化和断线前 outbox 缓冲
- `realtime/projectEventsSocket.js` 已负责项目级任务事件连接、断线重连、`PROJECT_INIT` 补偿、`TASK_*` 消息分发、`PRESENCE_SNAPSHOT` 分发和 `LOCK_REQUEST` / `LOCK_RELEASE` 发送
- `store/collab-store.js` 已提供项目任务 upsert/remove、事件 apply、选中任务同步、presence 快照合并和 metadata 锁状态 helper；后续聚合页改造优先复用这里，不要在页面里复制事件解析逻辑
- `api/documentImport.js` 是 Markdown 导入客户端封装；不要把分片上传逻辑散落到页面外的其它 API 文件
- `api/diary.js` 是今日日记入口封装；不要在布局或页面里直接拼 `/diary/today`
- `api/task.js` 的 `saveDocumentContent()` 是 diary plain Markdown 正文保存封装；不要在组件里直接拼 `/documents/:id/content`
- `api/meeting.js` 是会议纪要入口封装；不要在布局或页面里直接拼 `/meetings`

## 改动时必须联动检查

- 改登录或 token 结构：检查 `src/api/client.js` 和 `AuthContext`
- 改任务更新接口：检查 `ProjectDetailPage.jsx`、`TaskDetailPanel.jsx`
- 改路由：检查 `src/App.jsx`
- 改错误返回结构：检查 Axios response interceptor

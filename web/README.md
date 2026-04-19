# web 模块说明

这是 ToDoList 的前端模块，基于 React + Vite 构建。

## 当前页面

- 登录
- 注册
- 项目列表
- 项目详情
- 今日任务
- 未来 7 天
- 日历
- 个人资料

## 开发命令

安装依赖：

```bash
npm install
```

启动开发环境：

```bash
npm run dev
```

构建：

```bash
npm run build
```

检查：

```bash
npm run lint
```

## 开发代理

Vite 已配置开发代理：

- `/api` -> `http://localhost:8080`
- `/api` WebSocket 代理已开启 `ws: true`

因此本地联调时通常需要先启动后端服务。

## 当前前端约束

- API 统一通过 `src/api/client.js` 发起
- 登录 token 保存在 `localStorage`
- `src/components/TaskDetailPanel.jsx` 的正文编辑区已接入 Yjs provider，通过正文 WebSocket 实时保存
- `src/pages/ProjectDetailPage.jsx` 已接入项目级 WebSocket 事件流和本地增量 patch
- `src/pages/ProjectDetailPage.jsx` 已显示项目级在线人数，来源是 `PRESENCE_SNAPSHOT`
- `src/pages/ProjectDetailPage.jsx` 已消费项目级 metadata 锁事件；任务行和详情面板会展示锁状态，详情面板编辑元数据时会申请/释放 `metadata` 锁
- 今日任务、未来 7 天、日历等聚合页仍然通过重新拉取 HTTP 数据刷新页面

如果后端开始接入实时协同、版本控制或 WebSocket，需要优先联动：

- `src/pages/ProjectDetailPage.jsx`
- `src/components/TaskDetailPanel.jsx`
- `src/realtime/`
- `src/store/collab-store.js`

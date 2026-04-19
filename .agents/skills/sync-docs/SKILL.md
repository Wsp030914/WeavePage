---
name: sync-docs
description: 根据本次工作内容，同步更新或补齐项目文档，保持文档与代码实现一致。用户说“更新文档”“同步文档”“文档过时了”“更新 CLAUDE.md”“更新 AGENTS.md”“更新 README”“更新 TODO”或输入 `/sync-docs` 时触发；在完成功能开发、bug 修复、接口变更、架构调整、脚本命令变化后也应主动使用。
---

# Sync Docs

## 概述

根据本次实际改动，识别哪些项目文档已经过时、缺失或需要补充，然后只更新必要部分。  
如果项目标准文档缺失，不要跳过，直接创建最小可用版本并写入当前真实状态。

先阅读 [references/project-doc-map.md](references/project-doc-map.md)，确认当前仓库的文档布局、缺失项和特殊规则，再开始同步。

## 工作流

### 1. 回顾本次工作

- 先梳理这次到底改了什么：
  - 新增了什么功能
  - 修复了什么 bug
  - 改了哪些接口、命令、启动方式、目录结构、约束规范
  - 有没有新的踩坑、限制、设计决策
- 不要先改文档再反推代码；先读源码、配置和已有文档。

### 2. 识别受影响文档

- 以 [references/project-doc-map.md](references/project-doc-map.md) 为准，判断这次需要同步哪些文档。
- 这个项目的标准文档是“都要有”的：
  - 根目录 `AGENTS.md`
  - 各模块 `AGENTS.md`
  - 根目录 `README.md`
  - `docs/TODO.md`
  - `docs/*.md`
- 如果这些文档缺失，而本次工作已经触发了对应内容，就创建最小可用版本，不要因为文件不存在而跳过。

### 3. 逐一同步

- 根目录 `AGENTS.md`
  - 作为当前项目的 AI 接力主文档
  - 更新项目架构、目录说明、运行命令、约束规范、文档同步规则
- `CLAUDE.md`
  - 当前项目不是主文档
  - 只有项目显式引入时才维护，默认按兼容文档处理
- `AGENTS.md`
  - 按模块更新，如 `server/AGENTS.md`、`web/AGENTS.md`、`scheduler/AGENTS.md`
  - 重点写“接力必读”：症状、原因、解决方案、非显而易见约束
- `README.md`
  - 从用户视角更新项目功能、启动方式、开发命令、接口入口
  - 当前项目如果根目录 `README.md` 缺失，应创建，不要只保留 `web/README.md`
- `docs/TODO.md`
  - 将已完成项改成 `[x]`
  - 新增本次发现但未完成的事项
- `docs/*.md`
  - 更新或新增专项说明文档，例如架构方案、构建问题、接口调整、部署说明
  - 若本次改动影响 API，优先检查 `docs/swagger.yaml`、`docs/swagger.json`、`docs/docs.go`

### 4. 只写真实发生的变化

- 不要编造“已支持”但代码里没有的功能
- 不要写“未来计划”来冒充现状
- 不要重写整个文档，只改确实变化的部分
- 如果文档缺失，需要创建时，也只写当前能从仓库确认的内容

### 5. API 文档特殊规则

- 这个项目已经有生成产物：
  - `docs/swagger.yaml`
  - `docs/swagger.json`
  - `docs/docs.go`
- 当后端路由、请求体、响应体、鉴权方式发生变化时，必须检查这些文件是否需要同步。
- 如果当前会话无法安全或完整地重新生成 Swagger，至少要在对应文档中明确标注“需要重新生成”，不要假装已经同步。

### 6. 输出时说明改动

- 告诉用户你改了哪些文档
- 每个文档说明一句“改了什么、为什么要改”
- 如果某个该改的文档没改，说明原因，比如：
  - 文件不存在且本次尚无足够信息创建
  - Swagger 需要后续生成
  - 当前改动不影响该文档

## 书写原则

- 写“为什么”而不是只写“是什么”
- `AGENTS.md` 的坑要写成：症状 -> 原因 -> 解决方案
- `README.md` 写用户和开发者会真的用到的内容，不保留模板废话
- 根目录 `AGENTS.md` 写 AI 接力时真正需要的目录、命令、边界和约束
- `docs/TODO.md` 只维护真实状态，不做形式化罗列
- 不确定时先读代码、配置、路由、脚本，再决定怎么写

## 本项目的强制同步项

- 本项目标准文档按“都要有”处理：
  - `AGENTS.md`
  - `README.md`
  - `docs/TODO.md`
  - `server/AGENTS.md`
  - `web/AGENTS.md`
  - `scheduler/AGENTS.md`
- 如果本次工作影响接口或后端行为，也要检查：
  - `docs/swagger.yaml`
  - `docs/swagger.json`
  - `docs/docs.go`
- 如果本次工作输出了方案、计划或专项说明，也要同步 `docs/plans/*.md` 或新增对应 `docs/*.md`

## 参考资料

- 先读 [references/project-doc-map.md](references/project-doc-map.md)

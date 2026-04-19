---
name: project-new-branch
description: 开始新功能或修复前的标准流程：同步 main、切新分支、推送到远程。当用户说"开始新功能"、"切分支"、"新建分支"、"开始开发"、"new branch"或输入 /new-branch 时触发。
---

在开始编写任何代码前，先完成分支准备工作，确保基于最新的 main 开发。

## 步骤

1. **收集信息**：询问用户本次工作的类型和简短描述，用于生成分支名
   - 类型：`feat` / `fix` / `refactor` / `docs`
   - 描述：2-4 个英文单词，用 `-` 连接（如 `cli-channel-selection`）
   - 如果用户已经说明了要做什么，直接推断分支名，无需再问

2. **同步 main**：
   ```bash
   git checkout main
   git pull --rebase
   ```

3. **切新分支**：
   ```bash
   git checkout -b feat/xxx
   ```

4. **推送到远程并关联**：
   ```bash
   git push -u origin feat/xxx
   ```

5. 告知用户分支已就绪，可以开始开发

## 分支命名规范

| 类型 | 格式 | 示例 |
|------|------|------|
| 新功能 | `feat/xxx` | `feat/cli-channel-selection` |
| Bug 修复 | `fix/xxx` | `fix/acp-output-duplicate` |
| 文档 | `docs/xxx` | `docs/update-readme` |
| 重构 | `refactor/xxx` | `refactor/session-manager` |

## 规则

- 禁止在非 main 分支上执行此流程（先回到 main）
- 如果 `git pull --rebase` 有冲突，停下来告知用户，不要自动处理
- 分支名全小写，单词用 `-` 连接，不用下划线

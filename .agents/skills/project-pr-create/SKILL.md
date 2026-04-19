---
name: project-pr-create
description: 基于当前分支与目标分支的差异，使用 gh CLI 创建 GitHub Pull Request。当用户说"创建 PR"、"提 PR"、"开 PR"、"pr-create"或输入 /pr-create 时触发。
---

分析当前分支变更，生成结构化的 PR 标题和描述，并通过 `gh pr create` 创建 Pull Request。

## 步骤

1. 并行运行以下命令了解当前状态：
   - `git branch --show-current` — 确认当前分支
   - `git log --oneline main..HEAD` — 查看本分支相对 main 的所有提交
   - `git diff main...HEAD --stat` — 查看变更文件概览
   - `gh pr status` — 确认当前分支是否已有 PR

2. 如果当前分支已有 PR，告知用户并停止

3. 如果当前分支是 main，告知用户应先创建功能分支，并停止

4. 分析提交历史和变更内容，起草 PR 信息：
   - **标题**：简洁描述本次变更，格式参考 Conventional Commits（如 `feat(core): add session list command`）
   - **描述**：包含变更摘要、测试方式（如适用）

5. 向用户展示起草的标题和描述，**等待用户确认或修改**

6. 用户确认后，推送分支并创建 PR：
   ```bash
   git push -u origin HEAD
   gh pr create --title "..." --body "$(cat <<'EOF'
   ## 变更摘要
   ...

   ## 测试方式
   ...
   EOF
   )"
   ```

7. 输出 PR URL

## 规则

- 禁止在用户确认前自动创建 PR
- 禁止在 main 分支上创建 PR
- 如果当前分支已有 PR，提示用户使用 `gh pr edit` 修改
- 推送前不需要额外确认（push 是创建 PR 的必要前提）

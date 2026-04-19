---
name: project-commit
description: 分析当前 git 变更，生成符合 Conventional Commits 规范的提交。当用户说"帮我提交"、"commit 一下"、"提交代码"或输入 /commit 时触发。
---

分析当前 git 变更并创建提交，遵循项目的 Conventional Commits 规范。

## 步骤

1. 并行运行以下命令了解当前状态：
   - `git status` — 查看文件状态（不要用 -uall）
   - `git diff` — 查看未暂存变更
   - `git diff --staged` — 查看已暂存变更
   - `git log --oneline -5` — 了解仓库的提交风格

2. 分析变更内容，确定合理的暂存范围

3. 用 `git add <具体文件>` 暂存相关文件
   - 禁止 `git add -A` 或 `git add .`（避免意外提交 .env 等敏感文件）

4. 撰写提交信息，格式：`type(scope): subject`
   - type：feat / fix / docs / style / refactor / test / chore / perf
   - scope：可选，影响的包或模块（如 core、executor、channel）
   - subject：祈使句，小写开头，不加句号

5. 通过 HEREDOC 执行提交，运行 `git status` 确认结果

## 规则

- 禁止 --no-verify（hooks 是质量门禁，不能绕过）
- 禁止提交 .env 或包含密钥的文件
- 没有可提交的变更时直接告知，不创建空提交
- pre-commit hook 失败时：先向用户说明失败信息和原因，再询问是否需要修复，等待用户确认后再处理

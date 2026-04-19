---
name: project-pr-fix-comments
description: 拉取当前 PR 所有未解决的 review 评论，分析问题并给出解决方案，询问用户是否修复。当用户说"处理 PR 评论"、"修复 review 意见"、"fix comments"、"解决评论"或输入 /pr-fix-comments 时触发。
---

自动处理当前分支 PR 中所有未解决的 review 评论，让评审意见不被遗漏。

## 步骤

### 1. 确认当前状态

并行运行：

```bash
# 确认分支和 PR 号
gh pr status

# 获取所有未解决的 review thread（含行内评论）
gh api graphql -f query='
{
  repository(owner:"OWNER", name:"REPO") {
    pullRequest(number: PR_NUMBER) {
      reviewThreads(first: 50) {
        nodes {
          id
          isResolved
          comments(first: 5) {
            nodes {
              body
              path
              line
              diffHunk
            }
          }
        }
      }
    }
  }
}'

# 获取 PR 级别的评论（非行内）
gh pr view PR_NUMBER --comments --json comments
```

- `owner` / `repo`：从 `gh pr status` 的 URL 提取
- 如果当前分支没有关联 PR，告知用户并停止

### 2. 筛选未解决评论

从 GraphQL 结果中过滤 `isResolved: false` 的 thread，提取：
- `body`：评论内容
- `path`：涉及的文件路径
- `line`：涉及的行号（可能为 null，表示评论指向旧版本代码）
- `diffHunk`：被评论的代码片段（上下文）

同时收集 PR 级别评论（`gh pr view --comments` 结果），这类评论没有 isResolved 状态，需要人工判断是否是待处理的问题。

如果没有任何未解决评论，告知用户"无未解决评论"并停止。

### 3. 读取涉及的源文件

对每条未解决 review 评论，读取 `path` 对应的完整文件内容（非只读 diff），理解当前代码上下文，然后分析：

- 评论指出了什么问题？
- 当前代码为什么不符合评论期望？
- 修复方案是什么？

### 4. 输出分析报告

按评论逐条列出：

```
评论 1/N（文件: path, 行: line）
──────────────────────────────
问题描述：[用自己的话复述评论核心意思]
当前代码：[引用涉及的代码片段]
解决方案：[具体改法，如果涉及多个文件一并说明]
```

PR 级别评论单独一节列出，说明是否判断为需要处理的问题及原因。

### 5. 询问用户

展示完所有分析后，询问：

> 以上 N 条评论是否全部按方案修复？还是需要先确认某条？

- 用户确认全部修复 → 依次实施每条修复，每修复完一条说明进度
- 用户指定某条 → 只修复该条
- 用户说"不"或想自己改 → 结束，不做任何修改

### 6. 实施修复

按确认的方案修改代码，修改完后：

```bash
pnpm build   # 确保编译通过
```

编译通过后提示用户使用 `/commit` 提交。

## 规则

- 禁止在用户确认前修改任何文件
- 如果某条评论无法确定修复方案（模糊/冲突），在分析报告中标注"需要用户澄清"，不要猜测
- PR 级别评论无法通过 API 判断已解决，保守处理：列出所有非空 PR 评论，由用户决定是否处理
- 修复完所有评论后，不自动 resolve thread（留给用户在 GitHub 界面确认后手动 resolve，或等 PR 合并后自动关闭）

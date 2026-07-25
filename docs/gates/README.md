# Gate 记录

本目录只保存 REQ-08 定义的阶段放行事实，不增加 `Design → PRD → Plan → Acceptance` 之外的流程阶段。当前没有通过记录。

Gate 只核对活跃正式文档的 MVP allowlist、追踪和证据；未被正式输入定义的能力不作为缺口、条件或放行承诺。

文件名固定为 `GATE-<gate_scope>-<YYYYMMDDTHHMMSSZ>.yaml`，`gate_scope` 只允许 `requirement_readiness/design_entry/implementation_start/implementation_release`。记录一经提交不得原位修改；输入变化或复核结论变化时创建新文件，并用 `supersedes` 引用旧记录。

```yaml
gate_scope: implementation_start
result: passed
input_versions:
  PLAN-01: {version: 0.1.0, commit: "<git-sha>"}
owner: "<name>"
reviewers: ["<name>"]
decided_at: "<UTC RFC3339>"
gaps: []
evidence: ["<review-or-ci-uri>"]
next_stage: implementation
supersedes: null
```

Owner 组织评审并创建记录；Reporter 校验输入文件版本与 Git commit 一致后提交；Reviewer 的批准必须可由 PR review 或签名提交追溯。只有最新适用记录为 `passed` 才可放行，`conditional/failed` 按 REQ-08 处理。

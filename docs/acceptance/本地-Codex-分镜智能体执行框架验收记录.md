# 本地 Codex 分镜智能体执行框架验收记录

日期：2026-08-24

## 业务验收

- 输入包含至少两个确认场景、人物动作/对白、跨空间动作和来源明确出现的固定道具。
- 本机 Codex 经过五个 Skill 阶段生成候选，不使用 DeepSeek。
- required 来源全部覆盖，无跨场景引用，资产只绑定固定输入位置。
- 场景/总时长合格，镜号和时间码连续。
- 每个关键分镜行有 purpose、连续性、动作节拍与首帧；候选至少有一个尾帧。
- 来源明确出现的关键道具至少绑定到一个承载对应来源位置的镜头。
- Reviewer 无证据资产 blocker 或无效 scope 不触发自动修复，以 warning/risk code 保留。
- 最终状态为 `needs_review`，不自动批准或写正式 Shot。
- 合法 checkpoint 可恢复，损坏或跨任务 checkpoint 不复用；terminal hash/timeline/status 不一致时 fail closed。
- 活跃 run lease 的重投 requeue 且不启动第二次 Codex；heartbeat 续租，过期后新 token 可恢复，旧 token 不能保存 checkpoint 或提交结果。
- CSV/HTML 含时间码、摄影、动作、对白/声音、连续性、三类帧、来源和资产列。

## 可执行验收

```bash
cd backend
./.venv/bin/pytest -q tests/unit/storyboards tests/integration/storyboards
./.venv/bin/pytest -q tests/architecture/test_agent_domain_layout.py
LANVERSE_RUN_CODEX_LOCAL_CONTRACT=1 ./.venv/bin/pytest -q -s tests/contract/test_codex_local_storyboard_draft.py
./.venv/bin/ruff check .
./.venv/bin/pyright
```

五个项目自有 Skill 分别通过 `skill-creator/scripts/quick_validate.py`；在不包含 `vendor/`、`.gitmodules` 或任何上游 checkout 的工作区中，Skill 完整性校验和本地 Codex 合约仍必须通过。

## 残余风险

- 模型输出存在非确定性；失败必须暴露 hard gate/reviewer issue，不能静默放宽引用约束。
- 当前 Tool 只覆盖基础画面侧别；复杂轴线、视线、动作匹配和音画桥仍需要 Reviewer 与人工审核。
- 当前不验收场景图、人物三视图或资产图生成；后续必须通过 `assets`、`media`、`production` 边界单独实现。

# DES-05 Agent 与 Harness

| 项 | 内容 |
| --- | --- |
| 文档状态 | 草案，待评审（2026-09-25） |
| 上游 | [DES-01 系统架构 §7](01-系统架构设计.md)、[DES-08 技术选型 §4](08-技术选型决策.md)、[DES-04 工作流](04-工作流与生成操作.md)；功能 [REQ-12](../requirement/12-剧本导入与分集.md)、[REQ-13](../requirement/13-逐集结构解析.md)、[REQ-14](../requirement/14-设定集抽取与造型.md)、[REQ-18](../requirement/18-分镜生成与镜头编辑.md)、[REQ-28](../requirement/28-全能参考生视频.md)、[REQ-34](../requirement/34-V2与待定需求池.md) |
| 下游 | `agent/` 代码、`contracts/activities/`、`agent/skills/`、`agent/evals/`、TST-03 AI 评测方案 |
| 范围 | Agent 服务的边界与代码结构；Harness 各组件设计；MVP Skill 规格；供应商适配器；内容审核；V2 对话式 Agent；可观测与测试 |

## 1. 边界

| Agent 服务负责 | Agent 服务不负责 |
| --- | --- |
| 执行 `agent` / `agent.<provider>` 队列的 Activity | 连接业务数据库、写业务数据 |
| Harness：LLM Skill 的上下文、调用、校验、修复、预算、追踪 | 编排业务流程（Go 工作流负责） |
| 供应商适配器：生图、视频、TTS、ASR、审核的提交 / 查询 / 取消与用量解析 | 决定是否重新提交未知结果的付费请求 |
| 解密供应商凭据（私钥只在本服务） | 持有对象存储管理凭据（只用预签名 URL） |
| V2：对话式 Agent（AG-UI 事件流、规划、只读工具、提案） | 执行写命令（提案由用户确认后经 Go 执行） |

## 2. 代码结构

```text
agent/
  pyproject.toml · uv.lock
  src/lanverse_agent/
    app/                 FastAPI（agent-api）：health、harness 调试、evals、V2 agent runs
    worker/              Temporal Activity Worker 入口与 Activity 注册
    contracts/           由 contracts/activities/*.schema.json 生成的 Pydantic 模型（禁止手改）
    harness/
      skills.py          Skill Registry
      context.py         Context Builder
      router.py          Model Router
      tools.py           Tool Registry
      loop.py            Execution Loop
      validators/        schema、source_span、业务规则校验器
      budget.py · trace.py
    providers/
      base.py            适配器协议
      ark/ minimax/ openrouter/ mock/
      errors.py          供应商错误 → 平台错误码映射
    moderation/
    credentials/         凭据解封（私钥）与内存缓存
    conversation/        V2：规划器、AG-UI 事件、提案构建
  skills/<skill_key>/    Skill 包（见 §3.1）
  evals/                 评测集与评测脚本（TST-03）
  tests/
```

`agent-api` 与 `agent-worker` 共享代码，以 `lanverse-agent api|worker` 启动。

## 3. Harness

### 3.1 Skill 包

```text
skills/parse_episode/
  SKILL.md               元数据（frontmatter）+ 任务说明（系统提示词主体）
  input.schema.json      输入 schema
  output.schema.json     输出 schema（同时作为 LLM 结构化输出约束）
  references/            参考资料（术语、镜头语言词表、示例）
  examples/              少样本示例（输入 → 输出）
  validators.py          该 Skill 的业务校验（可选）
  evals/                 该 Skill 的评测用例
```

`SKILL.md` frontmatter：

```yaml
key: parse_episode
version: 1.3.0
description: 将单集剧本解析为场、台词、动作，所有结果带原文偏移
default_model: ark.doubao-pro
fallback_models: []            # MVP 不自动切换（GEN-08 为 V2）
max_input_tokens: 60000
max_output_tokens: 16000
max_repair_rounds: 2
timeout_s: 600
tools: [get_source_text]       # 允许的只读工具
validators: [schema, source_span, unique_lines]
chunking: { strategy: by_scene_marker, max_chars: 30000 }
```

发布：CI 计算 Skill 包内容 hash，生成 `skills/index.json`（key → version、hash）；任何内容变更必须提升版本号并通过该 Skill 的评测（REQ-02 AIQ-06）。任务 Trace 记录 `skill_key@version#hash`。

### 3.2 Skill Registry

- 启动时加载 `skills/index.json` 与全部包；校验 schema 合法、版本与 hash 一致，否则拒绝启动。
- Activity 输入带 `skill_key` 与 `skill_version`（由 Go 在报价时冻结）；Registry 找不到该版本时返回不可重试错误 `skill_version_unavailable`。
- 同一 Skill 允许并存多个版本（部署窗口内在途任务使用旧版本）。

### 3.3 Context Builder

职责：把冻结输入组装为消息，控制 token。

```text
system  = SKILL.md 主体 + references（按 frontmatter 选择）+ 输出格式要求
examples= examples/ 中按相似度或固定顺序选择，受 token 预算限制
user    = 由 input.schema 渲染的结构化输入；原文片段按行加偏移标注，如 "[1203] 李明：你怎么来了？"
```

- token 计数使用模型对应的分词器（无公开分词器时按字符数 × 系数估算，系数在 Router 中配置）。
- 超过 `max_input_tokens`：按 `chunking` 策略切分为多次调用并由 Skill 声明的 `merge` 函数合并；不支持切分的 Skill 明确失败（`context_too_large`），**不静默截断**。

### 3.4 Model Router

- 输入 `model_key`（来自 Operation 冻结的模型版本），读取模型参数（温度、top_p、结构化输出方式）。
- 统一接口 `complete(messages, output_schema, params) -> (json, usage)`；按供应商选择结构化输出方式：原生 JSON Schema 约束 > JSON 模式 + 校验 > 文本提取。
- 记录 `usage`（输入 / 输出 token）与按价格规则计算的 `cost_micros`。
- MVP 不做自动降级到其他模型。

### 3.5 Tool Registry

| 工具 | 作用 | 可用于 |
| --- | --- | --- |
| `get_source_text(from, to)` | 读取冻结输入快照中的原文片段 | parse_episode、extract_bible |
| `lookup_bible(name)` | 在冻结的设定摘要中查找条目 | storyboard |
| `shot_language_glossary()` | 镜头语言词表 | storyboard、compose_prompt |
| `estimate_duration(text)` | 按字数估算台词时长 | storyboard |

- 所有工具只读；Activity 中的工具只读取冻结输入快照，不访问实时数据（保证可复现）。
- 每个工具声明参数 schema 与输出大小上限；超限截断并标注“已截断”。

### 3.6 Execution Loop

```python
def run(skill, inputs, budget):
    msgs = context.build(skill, inputs)
    for round in range(skill.max_repair_rounds + 1):
        heartbeat()                                  # Temporal 心跳；检测取消
        out, usage = router.complete(msgs, skill.output_schema, tools=skill.tools)
        budget.charge(usage)                         # 超限抛 BudgetExceeded（不可重试）
        errors = validators.run(skill, inputs, out)
        trace.step(round, msgs_digest, out_digest, usage, errors)
        if not errors:
            return Result(out, trace, budget.usage)
        msgs = context.repair(msgs, out, errors)     # 把校验错误作为反馈追加
    raise ValidationExhausted(errors, raw_output=out)   # 保留原始输出供排查（REQ-13 R7）
```

- 工具调用在 `router.complete` 内部循环处理，工具调用轮次上限 8。
- 取消：心跳返回取消时立即中止并抛出 `Cancelled`。

### 3.7 Validators

| 校验器 | 规则 |
| --- | --- |
| `schema` | 输出满足 `output.schema.json` |
| `source_span` | 每个带 `span` 的条目：偏移在输入范围内，且原文 `text[span]` 去除引号、说话人前缀、空白后与内容一致（相似度 ≥ 0.95） |
| `unique_lines` | 原文中每句台词恰好出现一次；未覆盖的台词列入 `unassigned_lines` 并作为错误反馈一次，第二次允许剩余项留在“待处理” |
| `line_assignment` | 分镜：场内每个 `line_key` 恰好分配一次 |
| `bible_refs` | 分镜引用的造型 / 场景 / 道具 ID 属于输入中的设定集 |
| `duration_range` | 镜头时长在允许范围 |
| `episode_boundaries` | 分集：不重叠、不遗漏、单调递增 |

### 3.8 Budget 与 Trace

- Budget：`max_tokens`、`max_cost_micros`（= Operation 报价的预留金额）、`deadline`；任一超限终止。
- Trace（随 Activity 结果返回，由 Go 存入 `operation_output.json_payload.trace` 的摘要与对象存储中的完整版本）：

```json
{ "skill": "parse_episode@1.3.0#a1b2c3", "model": "ark.doubao-pro", "input_hash": "…",
  "steps": [ { "round": 0, "prompt_tokens": 18234, "completion_tokens": 6120, "latency_ms": 41200,
               "validation_errors": [ { "code": "source_span_mismatch", "path": "scenes[2].items[5]" } ] },
             { "round": 1, "prompt_tokens": 24410, "completion_tokens": 6300, "latency_ms": 43100, "validation_errors": [] } ],
  "usage": { "input_tokens": 42644, "output_tokens": 12420 }, "cost_micros": 380000 }
```

完整 Trace 不含凭据；原文只以偏移与摘要记录（REQ-02 PRV-02），完整提示词存对象存储、仅管理员可见。

## 4. MVP Skill 规格

| Skill | 输入（要点） | 输出（要点） | 校验 | 评测指标（TST-03） |
| --- | --- | --- | --- | --- |
| `split_episodes` | 规范化全文（带偏移）；规则分集结果（可为空） | `episodes[{seq_no, title, span}]`；`preamble_span` | `episode_boundaries` | 分集边界准确率 100%（样例集） |
| `parse_episode` | 单集原文（带偏移）；项目已知角色名与别名（可选） | `scenes[{heading, location_text, time_of_day, span, items[{type: line/action, kind, speaker_text, content, emotion, span}]}]`；`unassigned_lines[]` | `schema`、`source_span`、`unique_lines` | 台词召回 ≥ 99%、说话人 ≥ 95%、场边界 ≥ 90%（AIQ-01） |
| `extract_bible` | 各集结构摘要（说话人、场景标题、动作中的名词）；已有设定（增量时） | `characters[{name, aliases, description, episode_seqs, looks[{name, description, applies_to}]}]`、`locations[…]`、`props[…]`、`merge_suggestions[]` | `schema`、别名不重叠 | 主要角色召回 100%、别名合并 ≥ 90%（AIQ-02） |
| `storyboard` | 场结构（台词带 `line_key`）；本场设定（造型 ID、描述、锁定状态）；画幅、风格；镜头语言词表；默认模型的时长范围 | `shots[{description, entity_refs, shot_size, camera_angle, camera_movement, duration_ms, line_keys, look_ids, location_id, prop_ids, generation_mode, references[{role, ref}]}]` | `schema`、`line_assignment`、`bible_refs`、`duration_range` | 台词分配完整率 100%（AIQ-03）；人工评分 ≥ 3.5 / 5 |
| `compose_prompt` | 镜头版本、参考组合（带指代编号与描述）、目标模型的提示词习惯 | `prompt`、`negative_prompt` | 长度 ≤ 模型上限；每个参考指代都出现 | 人工评分；生成一致性对比（A/B） |

`compose_prompt` 默认由 Go 的规则模板生成（免费，REQ-27）；上表的 Skill 仅在用户选择“AI 优化提示词”时使用。

## 5. 供应商适配器

### 5.1 协议

```python
class ProviderAdapter(Protocol):
    key: str                                        # adapter_key
    def credential_schema(self) -> dict: ...
    async def submit(self, req: SubmitRequest, cred: Credential) -> SubmitResult: ...
    async def query(self, ref: TaskRef, cred: Credential) -> QueryResult: ...
    async def cancel(self, ref: TaskRef, cred: Credential) -> CancelResult: ...
    async def test_credential(self, cred: Credential) -> TestResult: ...
    def map_error(self, exc_or_resp) -> PlatformError: ...

SubmitRequest = { operation_id, request_key, provider_model_id, capability, mode, params,
                  inputs[{seq, role, media_url, media_type, duration_ms, label, text}], output_count }
SubmitResult  = { outcome: accepted|rejected|not_submitted|unknown, provider_task_id?, error? }
QueryResult   = { state: pending|running|succeeded|failed|not_found, progress?, result_urls[], usage?, error? }
```

### 5.2 实现要点

| 要点 | 做法 |
| --- | --- |
| 用途映射 | 每个适配器维护 `role → 供应商参数` 映射表（如方舟全能参考：`subject/scene/prop/style` → 参考图数组并附文字说明；`motion/camera` → 参考视频；`audio` → 参考音频）；映射表有单元测试 |
| 幂等 | 供应商支持客户端请求 ID 时传 `request_key`；不支持时在本服务 Redis 记录 `request_key → provider_task_id`（24h），重复 submit 直接返回已记录的任务 |
| `unknown` 判定 | 请求已写出（或无法确定是否写出）后发生超时、连接重置、5xx 且无任务 ID → `unknown`；连接建立失败、DNS 失败、本地限流 → `not_submitted` |
| 媒体输入 | 使用 Go 生成的预签名 GET URL（有效期 ≥ 预计处理时长 + 1h）；供应商要求先上传的，由适配器上传并缓存供应商侧 ID |
| 用量 | 解析供应商返回的计费用量（秒数、张数、token、字符）；无返回时按请求参数计算并标注 `estimated` |
| 凭据 | Activity 输入带 `credential{id, key_id, ciphertext}`（Go 传入的密文）→ 私钥解封 → 按 `id` 进程内缓存 5 分钟；不落日志（DES-07 §5.2） |
| 模拟供应商 | `providers/mock`：通过参数控制延迟、失败、超时、重复回调、结果过期，用于开发与故障注入（REQ-02 DEP-06） |

### 5.3 MVP 适配器

| 适配器 | 能力 | 说明 |
| --- | --- | --- |
| `ark` | text.structured、image.generate、video.generate（image2video、omni_reference）、audio.tts | 火山方舟：豆包 LLM、Seedream、Seedance、豆包语音【待 P0 核实各接口细节】 |
| `minimax` | video.generate、audio.tts | 海螺视频、Speech |
| `openrouter` | image.generate（GPT Image）、video.generate（海外版 Seedance） | 境外；OpenAI 兼容 API |
| `mock` | 全部 | 测试 |

## 6. 内容审核

- `moderation.check(kind, media_url | text)` → `{status: passed|rejected, labels[], provider}`。
- 视频按每 2 秒抽帧（上限 60 帧）+ 音频转写文本审核；图片直接审核；文本（剧本、提示词）按段审核。
- 审核服务作为一种适配器接入（阿里云 / 网易易盾 / 数美【待定，REQ-02 N3】）；模型自带审核（`moderation = provider`）时可跳过平台审核。
- 审核不可用时结果保持 `pending`，不放行（安全优先）。

## 7. 对话式 Agent（V2 要点）

V2 规划时展开设计（PRD-01 P24）。已确定的约束：

- 协议与 LibTV 对齐：前端 CopilotKit + AG-UI 事件流，经 Go 代理鉴权与持久化后转发到 `agent-api`（DES-01 §7.4）。
- Agent **没有写工具**：只能读取当前项目（只读工具，带会话范围令牌），修改以“提案”呈现，用户确认后以用户身份经命令层执行；付费生成只能以报价草稿出现，须二次确认（PRD-01 P21）。
- 需要在 V2 设计的内容：规划循环、AG-UI 事件映射、提案结构与白名单、提示注入防护与评测、会话记忆、会话级 LLM 预算。

## 8. 可观测

- OpenTelemetry：Activity 继承 Temporal 传播的 trace；每次 LLM / 供应商调用一个 span（属性：provider、model、tokens、cost、outcome）。
- 指标：Skill 成功率、修复轮次分布、校验错误码分布、token 与费用、供应商调用延迟与错误率、审核通过率。
- 日志：结构化 JSON；禁止记录凭据、预签名 URL 签名部分、剧本全文。

## 9. 配置与部署

| 配置 | 来源 |
| --- | --- |
| Temporal 地址、命名空间、队列 | 环境变量 |
| Redis 地址 | 环境变量 |
| 凭据私钥 | 生产：火山引擎 KMS；开发：本地文件（`.env` 不入库） |
| 服务令牌（Go ↔ agent-api） | 密钥管理，双向校验，定期轮换 |
| Skill 包 | 随镜像发布（不从网络动态加载） |

## 10. 测试

| 层 | 内容 |
| --- | --- |
| 单元 | Validators、Context Builder 切分与合并、错误映射、用途映射、预算 |
| 契约 | Pydantic 模型与 `contracts/activities/*.schema.json` 一致（CI 生成比对） |
| 集成 | Temporal dev server + 模拟供应商：submit / query / cancel / unknown；Skill 用录制的 LLM 响应回放 |
| 评测 | `evals/` 按 Skill 的指标（TST-03）；Skill 或模型变更时 CI 运行，指标下降 > 3 个百分点阻断（AIQ-06） |
| 安全 | 提示注入与越权用例（V2） |

## 11. 待确认

| # | 问题 | 默认处理 |
| --- | --- | --- |
| AG-Q1 | 各 Skill 的默认 LLM | P0 评测后确定（候选：豆包、DeepSeek、通义，均为境内） |
| AG-Q2 | 独立审核服务选型 | DES-07 中确定默认方案，P0 核实价格 |
| AG-Q3 | ag-ui-protocol 版本与许可 | V2 开发前核实（T6） |

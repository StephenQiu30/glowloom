# Agent 服务实施计划

- 日期：2026-09-08
- 授权依据：用户要求开始 Agent 服务实现及必要目录调整。
- 设计：[0015 完整能力设计](../design/0015-Harness能力缺口评审与创作画布闭环详细设计.md)、[3004 专业能力](../design/3004-AgentHarness专业能力与创作流程设计.md)。
- 基线：main / aca802a7；保留已有 Workflow、源桥及平台采纳的并发增量。

## 已完成切片一：模块边界与受限执行可靠性

1. 将摘要编码移动到 protocol，将文本签名放入 text_contract；可信服务不依赖候选进程包。
2. 提取 reasoning/Codex 适配器，专业模块不从旧 StoryGraph 借用底层进程实现；只建立有实际消费者的目录。
3. Red 固定 stdin 阻塞时输出超限、重复 JSON 键等失败场景，Green 完成终止/等待和严格结果检查。
4. 增加候选能力就绪接口，区分已安装发布、CLI 可执行与未经验证的真实模型链。
5. 验证双镜像导入隔离、既有跨语言摘要和专业发布摘要，执行 Agent lint、格式、严格类型和测试。

## 验收记录

- [x] 公共合同不导入候选/专业/可信运行层；两种运行层依赖方向正确。
- [x] 所有仓库内导入及镜像指向新位置，不保留重复实现或旧路径转发层。
- [x] 阻塞 stdin 时输出超限仍及时终止并等待子进程。
- [x] 重复键、非有限数字、非法结构化输出文件被拒绝。
- [x] readiness 区分发布缺失/漂移及 CLI 不可用，不执行真实模型。
- [x] 合法输入摘要与现有专业包摘要保持一致，定向和完整检查结果记录。

2026-09-08 本轮验证（工作目录 `agent/`）：

| 命令/检查 | 结果与边界 |
| --- | --- |
| `.venv/bin/pytest -q tests/unit/test_codex_result_boundary.py`（修复前） | 7 项失败，复现 stdin 阻塞/重复键/非有限数/符号链接接受问题 |
| `.venv/bin/pytest -q tests/integration/test_candidate_readiness.py`（实现前） | 4 项失败，接口不存在 |
| 受限进程、三个专业 Harness 定向测试 | 34 项通过；使用真实合成子进程验证终止/等待，不调用真实模型 |
| 架构、合同、单元、内部 HTTP 与平台签名定向组合 | 119 项通过，含只复制可信镜像声明源码包后的独立导入测试 |
| `.venv/bin/pytest -q` | 127 项通过、30 项跳过；未配置独立测试 PostgreSQL/Temporal 和真实 Codex opt-in |
| `.venv/bin/ruff check app tests` | 通过 |
| `.venv/bin/ruff format --check app tests` | 87 个文件格式通过 |
| `.venv/bin/pyright app tests` | 0 errors、0 warnings |

首次完整测试曾遇到并发 Workflow 恢复修改中的一项测试失败；并发任务同步该测试后重新执行，得到上表最终结果。本轮没有修改其 Workflow 业务逻辑。上述结果是对应命令执行时的工作区快照，不能扩展为后续并发修改的验收。未进行 Docker 镜像构建、真实模型/数据库/Temporal 联调、业务迁移或部署；镜像验证仅覆盖源码复制清单与导入隔离。

## 第二切片：持久执行尝试与结果追踪

- [x] 追加独立迁移，保留既有迁移 checksum；旧步骤明确无可用尝试历史。
- [x] reserve 原子保存 Attempt、当前指针和额度；并发领取仅产生一次尝试。
- [x] finish 原子保存尝试终态、草案、输出绑定和 Outbox，数据库失败全部回滚。
- [x] unknown/租约过期保留消费额度与原始 fence；伪造/迟到 Attempt 无写入权。
- [x] 精确签名的运行内查询接口返回安全摘要，跨运行/无授权访问被拒绝。
- [x] 使用本轮创建的独立临时 PostgreSQL 数据库验证真实事务、迁移、并发及 HTTP；完成后仅删除本轮测试库。

此切片暂不开放重新付费调用、结构化修复或额外 Attempt 的创建入口；先固定真实执行账与恢复证据。

本轮验证：新增 14 项 Attempt 测试；首次 8 项失败后完成实现。使用本轮新建的独立 PostgreSQL 测试库运行 `.venv/bin/pytest -q`：162 passed、9 skipped（Temporal/真实 Codex 条件未启用）。Ruff check、90 个文件格式、Pyright 0 errors/0 warnings 均通过。覆盖并发领取、成功与 unknown 竞争、伪造/迟到尝试、草案与事件关联约束、插入/Outbox 故障回滚、终态不可变、旧数据迁移、checksum 漂移及签名 HTTP。已发布的 command/execution SQL 文本保持不变。

测试只验证本轮工作区时刻的 Agent 与事务合同，不表示 Go 公共接口、画布消费、Kafka Publisher 或完整 Temporal/真实模型链已验收。本轮未迁移运行中的业务实例、未提交或推送；完成后删除本轮专用测试库，保留其他任务的工作区增量。

## 后续依赖

当前切片不代表 42 项能力完成。S1 中已审阅身份/状态/披露映射、S2 的 Manifest/Attempt/多输出和事件发布按 0015 继续推进；正式回执、事件消费、画布和供应商合同需要与各 Owner 联合验收。并发 Workflow/采纳实现单独核验，不将其代码归为本切片交付。

## 本轮 main 发布范围

2026-09-08 发布时从 HEAD 与本轮路径白名单构造独立快照，排除并发 Workflow/平台采纳/前端增量以及混合文件中的相关内容。提交组合重新运行 Agent 全部测试：121 passed、25 skipped；Ruff、格式、Pyright 均通过。可信镜像源码烟测覆盖此次提交已有的 API 和 Temporal 启动交接模块，不引用未提交 Worker。上方 127/30 是此前混合工作区的历史验证记录，不作为本次发布快照的测试数量。


## 第二切片发布快照验证

2026-09-08 从 main `53321b95` 与本任务 10 个路径构造隔离快照，排除并发 Workflow、Go 采纳和前端增量。独立临时 PostgreSQL 上完整 Agent 测试为 152 passed、8 skipped（2 项 native Temporal、6 项真实 Codex）；Ruff、84 个文件格式、Pyright 0 errors/0 warnings 通过。此前 162/9 是混合工作区历史结果。仅发布本切片代码，未迁移业务库或部署；远端 CI 不由本地结果代替。

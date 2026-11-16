# Printfilm 与 Frametell：确定性编排和交付质量复核

调查日期：2026-11-12。固定提交、只读源码调查，没有安装或执行上游程序、运行其测试、调用模型或制作样片。GitHub 插件用于仓库发现、固定提交的关键文件和公开 Actions/日志读取；本地浅克隆用于调用链核对。两者不能当成相同类型的短剧产品比较：Printfilm 面向漫剧/短视频；Frametell 主要面向页面动画式知识视频。

## 1. 版本与产品入口

| 项目 | 本次固定提交 | 提交时间 | 根许可证 |
| --- | --- | --- | --- |
| yi1108/printfilm | 9c41796d8ae4ee0a903c44a0911f29d57f69836b | 2026-11-12 14:09:11 +08:00 | MIT；另读了四份内置短剧 Skill 的 MIT 文件，不代替完整依赖许可核查 |
| sitor-ai/frametell | dbd7802a04984bac587e5c655cbea042fc808639 | 2026-11-09 15:03:55 +08:00 | AGPL-3.0；子包标注及素材需分别核对 |

Printfilm README 把当前短视频描述为用户逐阶段启动。新 Studio 的 `printfilm_studio_enabled` 配置默认 false，入口 `ensure_enabled` 会返回 404；存在实现与实际部署启用不能合并判断。[P1] Frametell README 的单次示例成本/时间、产线统计是作者陈述，本轮没有独立复现。

## 2. Printfilm：单助手工具循环与依赖图执行器分开

### 2.1 调用关系和阶段产物

Studio 用户请求 → 持久 Session/Run/Event → `iter_agent_turn` → 受限工具 → 画布节点/资产/正式 TaskRun。

- 当前助手是一个模型循环，每轮最多 12 次工具执行；JSON 工具数组可表达多操作，画布写入批次有原子提交路径。媒体工具返回 task_created，只代表排队。规划角色/场景或分镜成功后会直接 return，等待用户启动下一阶段。[P2]
- Skill 经 `compose_task_skills` 按任务标签和用户选择加载，再由 `render_skill_block` 拼入系统提示，设置总字符上限。编剧、资产、分镜、视频提示词等名字并不构成独立 Agent 实例。[P3]
- 在 `backend/app/printfilm_studio` 和 `backend/app/services/agent` 对 mcp/subagent/sub_agent/delegate 的有界检索未找到执行型主子委托或统一外部 MCP 客户端；asyncio 的 task/gather 是协程调度，不能计作子 Agent。其他入口及未来版本不据此推断。
- 画布工作流是另一条确定性 DAG：用户报价 → 检查画布版本/费用 → 明确 start → 按依赖执行文本及媒体节点。DFS 检查循环与引用连线，复用已有结果，参数变化时重新安排文本写作。[P4]
- `plan_story_shots` 的一个分镜节点在其 prompt 中表示可生成的视频片段，内部可含多个微镜头。它没有统一等同于叙事镜头、供应商生成单元或最终时间线片段；报告不按 UI 卡片数推算片长或成本。

### 2.2 自动推进、恢复和局部改动

- 报价保存 `plan.ids/settings/items/total/version`。启动会锁项目/画布行，拒绝版本或价格变化，复用已启动 run，检查余额和待承诺费用。此处可以确认准入检查，不能推定全链费用已经是不可变合同。[P4]
- 执行器并发上限为 6，已提交的媒体节点可以直接接续原 taskId。每个分支使用独立数据库会话，按依赖完成集启动下游；分支失败后不再准入新分支，但会收集已启动兄弟分支的结果与结算。[P5]
- 媒体等待读取原 TaskRun，成功必须返回 URL，失败/取消即停止；最多 3600 次、间隔 2 秒。这个循环是进程内等待，不是可证明跨重启保持的绝对 deadline。[P4]
- Workflow 更新 heartbeat；HTTP status 查询发现 90 秒失联后标 failed，并保留已提交媒体。Agent Run 也有 request_id 去重、事件序列和 heartbeat，过期时聚合全部 message_delta，记录 run_interrupted。它证明“保留结果后失败/人工继续”，不证明完整模型循环自动恢复。[P6]
- 底层 scheduler 有数据库租约、SKIP LOCKED、孤儿任务恢复。inline HTTP 执行被中断时会落 failed/cancelled，不自动重新 POST；普通任务可重新排队，已有 provider_task_id 会随任务恢复。视频下载失败专门保留原供应商 ID、退避重试；没有 ID 时提示按原任务取回而非假装安全再生成。[P7]
- 本轮没有审计全部 provider 适配器和提交协议，因此“没有拿到供应商 ID 时是否可以安全重发”“跨进程费用守恒”保留未验证，不能以租约或 request_id 代替供应商幂等证据。

### 2.3 成片与质量边界

短视频 `pipeline._compose_stage` 从逐镜素材建立 ShotMedia，真实调用 FFmpeg，包含画幅、字幕、旁白、原声音轨/BGM及本地资源下载。源视频缺失时可用图片；二者都缺失时 `compose_project` 会生成纯色片段。[P8]

同一合成函数成功后先设 AUDITING，随后的事务直接设 DONE；该片段没有读取视觉审查或人工审片结论。不能把 AUDITING 状态名当成已经完成审核，也不能把 MP4 合成成功当成故事没有遗漏、没有占位片段。[P10]

仓库有工作流并发、失败阻断下游、关机、编辑冲突退款、下载重试、FFmpeg SIGTERM 等测试。`test_parallel_failure_drains_started_sibling_and_blocks_descendants` 明确模拟失败分支和已启动兄弟分支，验证只保存兄弟结果、不调用其子节点及费用变化。它用 monkeypatch 模型，不是供应商集成测试。[P9]

所选 SHA 的公开 Actions runs 查询返回 0，所检源码没有 `.github/workflows`；不能推断外部 CI 或其他版本永远不存在。

## 3. Frametell：宿主编排多个会话，质量工具确实执行但有软硬之分

### 3.1 调用关系与产物

本地 Studio/Core Run → `Director.run` → research/planner → storyboard.json → 宿主批准回调 → script.js → 配音、共享绘制组件和图片 → 页面 Agent 会话 → 独立 reviewer 会话 → live preview ready → 用户 export → renderer/FFmpeg MP4。[F1]

- Director 是确定性协调器；每阶段是独立 Agent 会话，可选择 Pi、Claude SDK 或 Codex SDK；页面按 lane 分组并发。独立 reviewer 使用无写入工具、只返回判决的会话，不与画页面的对话混用。它是实际分会话协作，不能据此推定主模型自由创建无限子 Agent。[F2]
- Style/Kit/组件、领域 prompt 与文件/命令工具构成制作方法；本轮选定 agent/core/runtime 没有找到与 Toonflow 等价的任意外部 MCP 服务注册入口，不能把多 SDK 适配称作 MCP 发现机制。
- Core `start()` 传入等待用户批准的回调；`Director.run` 库函数本身有默认回调，宿主还有可配置的定时继续路径，故“先审批”需要连同调用入口与配置判断。[F3]
- 产物有 storyboard/script/voice/pages、`.studio/state.json`、会话引用和 Git 历史。恢复分镜前先保存 planner transcript 引用；恢复时继续旧会话/已有文件，不重新发最初规划。状态文件使用同步写入；仅凭这个函数不能证明断电一致性或多宿主共享存储事务。[F4]

### 3.2 局部重做和受限质量循环

修订器生成明确 page ID 的计划；修改页面时仅将目标页面排队，清除 exported，使旧成片不能继续代表新内容。只改配音有独立路径；已经生成的配音/页面可缓存复用。测试源中能看到对“只重画 p02/p03”“修订后未改到页面需说明”等断言，但本轮未执行。[F5]

页面完成前实际调用 lint、截图和 reviewer，把问题回给绘制会话；轮次、会话 turns、并发和限时有上限。运行失败或调用不存在引擎 API 的页面禁止当作正常完成；审美/布局问题修复到上限仍可标 flagged。reviewer 不返回合法判决时会有限重问，随后以 lint 结果处理。质量层不是一个统一布尔值。[F6]

### 3.3 最终质量门禁不是全部阻断式

- `ready` 表示可预览，MP4 导出是另一次用户动作；不能把 stage=done 当成 MP4 已交付。
- `render` 先写 exported=true、发出 done，再异步调用最终 MP4 的 film gate，错误回调为空。即使 gate 报 red，也不能由此函数断言文件交付已被阻断。[F7]
- film-gate 确实读取实际 MP4；当前红灯判断针对片中空白段。静止比例、运动强度等指标只报告，源码明确尚无标定集。这不包含剧情忠实度、角色身份、口型或对白保真自动验收。[F8]
- 所选 SHA 的公开 CI run **42576923919** 为 failure，失败步骤 `pnpm test`；日志含 blueprint/diorama 等 benchmark 的 `Unexpected end of JSON input` 及其他基准失败。这里只记录症状和终态，没有诊断上游根因，也没有用失败反推所有功能不可用。[F9]

## 4. 调查结论与未验证项

| 问题 | Printfilm | Frametell |
| --- | --- | --- |
| 真正的运行组织 | 单助手工具循环 + 用户启动的依赖图执行器 | 确定性 Director + 多个有边界的创作/审查会话 |
| 自动推进 | 用户授权当前 DAG 后，依赖满足即推进；创作规划阶段主动停下 | 依阶段状态推进；分镜/首屏批准依宿主配置，导出另发起 |
| 恢复 | Agent/Workflow失联标失败，底层原媒体任务恢复另行处理 | 项目状态、旧会话和文件/Git版本恢复；断电/多宿主未验证 |
| 局部重做 | 依赖节点与已有结果复用，未证明整剧采用版本闭环 | 页面计划、重画及 exported 失效有实现；不属于人物短剧全制作模型 |
| 交付质量 | 有 FFmpeg，允许纯色退化素材，审核状态不能当判决 | 有 lint/截图/多模态 reviewer，最终 MP4 gate 在 done 之后报告 |
| 真实质量验收 | 本轮未执行 | 本轮未执行；所选公开 CI 有失败 |

## 固定提交源码与 CI

[P1]: https://github.com/yi1108/printfilm/blob/9c41796d8ae4ee0a903c44a0911f29d57f69836b/backend/app/config.py#L197
[P2]: https://github.com/yi1108/printfilm/blob/9c41796d8ae4ee0a903c44a0911f29d57f69836b/backend/app/printfilm_studio/agent/runtime.py#L270-L309
[P3]: https://github.com/yi1108/printfilm/blob/9c41796d8ae4ee0a903c44a0911f29d57f69836b/backend/app/services/agent/compose.py#L17-L80
[P4]: https://github.com/yi1108/printfilm/blob/9c41796d8ae4ee0a903c44a0911f29d57f69836b/backend/app/printfilm_studio/workflow.py#L125-L204
[P5]: https://github.com/yi1108/printfilm/blob/9c41796d8ae4ee0a903c44a0911f29d57f69836b/backend/app/printfilm_studio/workflow.py#L354-L419
[P6]: https://github.com/yi1108/printfilm/blob/9c41796d8ae4ee0a903c44a0911f29d57f69836b/backend/app/printfilm_studio/agent/runs.py#L107-L124
[P7]: https://github.com/yi1108/printfilm/blob/9c41796d8ae4ee0a903c44a0911f29d57f69836b/backend/app/services/tasks/scheduler.py#L531-L594
[P8]: https://github.com/yi1108/printfilm/blob/9c41796d8ae4ee0a903c44a0911f29d57f69836b/backend/app/services/ffmpeg_compose.py#L1204-L1238
[P9]: https://github.com/yi1108/printfilm/blob/9c41796d8ae4ee0a903c44a0911f29d57f69836b/backend/tests/test_printfilm_studio_workflow_run.py#L307-L335
[F1]: https://github.com/sitor-ai/frametell/blob/dbd7802a04984bac587e5c655cbea042fc808639/packages/agent/src/director.ts#L2060-L2080
[F2]: https://github.com/sitor-ai/frametell/blob/dbd7802a04984bac587e5c655cbea042fc808639/packages/agent/src/director.ts#L27-L69
[F3]: https://github.com/sitor-ai/frametell/blob/dbd7802a04984bac587e5c655cbea042fc808639/packages/core/src/runner.ts#L469-L485
[F4]: https://github.com/sitor-ai/frametell/blob/dbd7802a04984bac587e5c655cbea042fc808639/packages/agent/src/director.ts#L614-L638
[F5]: https://github.com/sitor-ai/frametell/blob/dbd7802a04984bac587e5c655cbea042fc808639/packages/agent/src/director.ts#L1895-L1912
[F6]: https://github.com/sitor-ai/frametell/blob/dbd7802a04984bac587e5c655cbea042fc808639/packages/agent/src/director.ts#L1670-L1718
[F7]: https://github.com/sitor-ai/frametell/blob/dbd7802a04984bac587e5c655cbea042fc808639/packages/agent/src/director.ts#L1769-L1798
[F8]: https://github.com/sitor-ai/frametell/blob/dbd7802a04984bac587e5c655cbea042fc808639/packages/engine/tools/film-gate.mjs#L1-L84
[F9]: https://github.com/sitor-ai/frametell/actions/runs/42576923919

[P10]: https://github.com/yi1108/printfilm/blob/9c41796d8ae4ee0a903c44a0911f29d57f69836b/backend/app/services/pipeline.py#L1499-L1532

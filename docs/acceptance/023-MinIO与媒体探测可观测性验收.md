# MinIO 与媒体探测可观测性验收

- 状态：accepted（仅 PT-OBS-002 当前 MinIO/ffprobe 遥测增量；PT-OBS-002 整体保持 in_progress，PT-OBS-003 不在本次范围）
- 日期：2026-08-04
- 验收基线：`main@024e069` 工作树；本实现与证据在同一后续提交进入 `main`
- 对应需求：[日志与可观测性需求](../requirement/012-日志与可观测性需求.md)
- 对应设计：[日志与可观测性详细设计](../design/模块设计/013-日志与可观测性详细设计.md)
- 对应产品任务：[PT-OBS-002 当前 MinIO/ffprobe 增量](../prd/009-剪辑交付与平台保障PRD任务.md)
- 对应工程计划：[DEV-S6-03 第四个无 Key 增量](../plan/000-MVP全栈实施总计划.md)
- 验收边界：接受已实现 MinIO 八项端口和 ffprobe 探测的 CLIENT 子 span、登记集成日志及五项低基数指标；不接受 FFmpeg render、Provider、业务 MediaVersion/Task 完整关联、OTLP、生产 `/metrics` 边界、告警、PT-OBS-002/003 整体或 S6

## 1. 验收结论

1. 新增语义化 `backend/app/modules/media/metrics.py`，固定 `default/unregistered` storage profile、八项 operation、四类 media kind 与有限 result；未知或攻击者控制的值统一为 `unregistered`，不会制造 Prometheus label。
2. 五项指标固定为 `lanverse_storage_operations_total`、`lanverse_storage_operation_duration_seconds`、`lanverse_storage_bytes_total`、`lanverse_media_probe_results_total` 和 `lanverse_media_probe_duration_seconds`。只有成功 put 与完整消费的 stream 记录精确 bytes；提前关闭流记录 cancelled，既不记 succeeded，也不增加 bytes。
3. MinIO 八项操作统一建立 `storage.minio` CLIENT span，只含 system/profile/operation/result；ffprobe 建立 `media.ffprobe` CLIENT span，只含 tool/kind/result。实际媒体链验证 RabbitMQ consumer → ffprobe → MinIO stream 保持同一 trace 且父子 span 不复用。
4. 实测发现通用 `start_span` 在未给显式 `traceparent` 时错误传空 Context，使内部集成阶段成为新根。实现改为“有显式远端父上下文时提取，否则继承当前 span”，既有 HTTP→Outbox→RabbitMQ 传播回归保持通过。
5. 日志事件固定为 `storage.operation.completed/failed` 与 `media.probe.completed/failed`，只允许 trace/span、固定类别、耗时、成功 bytes 或稳定 error code。对象 key、bucket、endpoint、预签名 URL、临时路径、MIME、命令实参、stdout/stderr、凭据和异常正文均未进入 span、日志或 label。
6. 指标和日志观测均为 best-effort：指标对象或 logger 主动抛错时，存储仍返回原 ValueError，ffprobe 仍返回原 `probe_tool_unavailable`，遥测失败不会替换业务结果。

## 2. Red → Green 证据

| 阶段 | 命令与真实结果 |
| --- | --- |
| 指标契约 Red | 首次运行新增单元/真实契约集合时在收集阶段失败：`ImportError: cannot import name 'metrics' from 'app.modules.media'`，证明原系统没有 storage/media 指标与 label 白名单。 |
| 指标与日志 Green | `test_media_integration_observability.py` 与 `test_logging.py` 合计 8 passed；未知 label、五项指标、四个事件 allowlist、指标/logger 抛错隔离均通过。 |
| Trace Red | 首次把真实媒体栈加入父子断言后失败：`media.ffprobe` 有有效 SpanContext 但 `parent is None`，证明原 `start_span` 把本地子阶段错误切成新根。 |
| Trace Green | 修正当前上下文继承后，真实媒体栈 4/4；同 trace 的 consumer → ffprobe → storage stream 父子关系及既有 W3C 传播定向回归 5/5 通过。 |

## 3. 真实基础设施与故障证据

- `make contract-minio` 直接复用本机已配置私有 MinIO，为 7 passed：八项成功操作、匿名拒绝、预签名传输、multipart、hash、错误凭据、不可达 endpoint、网络前参数拒绝，以及提前关闭 stream 不伪报 bytes 均通过。
- `make contract-ffprobe` 调用本机真实 ffprobe 并注入不存在的工具路径，为 2 passed；真实 PNG 得到 1×1/png/png_pipe，工具缺失保持稳定业务错误和 failed span/log/metric。
- `make contract-media-stack` 使用真实 PostgreSQL、隔离 RabbitMQ、本机 MinIO 和 ffprobe，为 4 passed；上传探测、到期清理、位置迁移/回滚/退役的业务事实不变，且 Worker 子阶段 trace 连续。
- 测试只创建随机测试对象、无效测试身份和临时媒体；没有读取、打印或写回本地 `.env`，也没有输出本机真实凭据、对象 key、endpoint、URL 或媒体内容。

## 4. 全量发布门禁

- `make check` 全绿：Ruff、Pyright 0 errors/0 warnings、ESLint、TypeScript、后端 248 passed/19 个显式外部或性能开关 skipped、前端 17 文件/56 tests、`pip check`、Next.js 16.2.12 生产构建和 development/production Compose config 均通过。MinIO、ffprobe 与媒体栈的默认 skip 已由本 Acceptance 上述显式真实命令补证。
- `DEEPSEEK_API_KEY='' ARK_API_KEY='' LANVERSE_E2E_BACKEND_PORT=8002 LANVERSE_E2E_FRONTEND_PORT=3001 make e2e` 首轮 8/9 暴露既有分镜竞态：切换 current spec 的完成提示早于 Shot order 刷新，紧接复制使用旧 revision。页面现与保存规格一致，在 mutation 后等待 order/versions refetch 再显示完成；失败用例单独复跑 1/1，完整空 Key 顺序回归最终 9/9。
- 无 Provider Key 时 DeepSeek、Seedream/Seedance 继续按原设计 fail closed；本增量没有 Ollama、Ark SDK、模拟 Provider 成功、伪 Candidate 或伪费用。

## 5. 未接受项与残余风险

PT-OBS-002 整体仍为 `in_progress`，PT-OBS-003 仍未接受。MinIO/ffprobe 适配器不拥有 MediaVersion、Task 或 Render 事实，因此当前日志依靠父 trace 串联，不能替代业务编排层的 media_version_id/task_id 事件；FFmpeg render 输入/输出、退出码和 stderr digest 也尚无真实实现。

当前 Counter/Histogram 仍是进程内指标，独立扩容 Worker 时需要受控 scrape 或正式 multiprocess 方案。OTLP exporter、Collector 故障/丢弃计数、生产 `/metrics` 网络与鉴权、Task/Provider/数据库/缓存其余指标、告警阈值和 runbook 都必须在后续 PT-OBS-002/003 取得独立证据，不能由本 Acceptance 外推。

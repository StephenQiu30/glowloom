# HTTP 结构化日志与低基数指标验收

- 状态：accepted（PT-OBS-001 当前 HTTP 持续增量；PT-OBS-002/003 不在本次范围）
- 日期：2026-08-04
- 对应需求：[日志与可观测性需求](../requirement/012-日志与可观测性需求.md)
- 对应设计：[日志与可观测性详细设计](../design/模块设计/013-日志与可观测性详细设计.md)
- 对应产品任务：[PT-OBS-001/002/003](../prd/009-剪辑交付与平台保障PRD任务.md)
- 对应工程计划：[DEV-S6-03 当前 HTTP 增量](../plan/000-MVP全栈实施总计划.md)
- 验收边界：只接受 HTTP request ID、access event、字段处理和 Prometheus HTTP label 基线；不接受 W3C span、消息/Worker/Provider/FFmpeg trace、OTLP exporter、告警、运行手册、`/metrics` 生产访问控制、PT-OBS-002/003 或 S6

## 1. 验收结论

1. `X-Request-ID` 只在可解析、版本为 7 且等于规范字符串时继续使用；缺失、非法、UUID 非 v7 或非规范形式都由服务端生成新 UUIDv7，响应和 ApiError 不回显原输入。
2. HTTP 日志使用登记的 `http.request.completed` / `http.request.failed`，公共 JSON 字段包含 UTC timestamp、level、service、component、environment、event_name 和稳定 message。请求上下文只允许 request_id、method、route、status_class、duration_ms；失败事件另允许 error_type。
3. access log 在响应完成后读取 FastAPI route，因此动态资源只出现 `/api/v1/tasks/{task_id}`，未匹配请求统一为 `unmatched`；实际资源 ID、query、header 和 body 不进入事件。
4. 日志字段先按 event allowlist 丢弃，再按敏感键递归脱敏；控制字符转为空格，字符串最长 512 个 Unicode 字符。被拒绝字段不记录原值，只递增 `lanverse_telemetry_redaction_drops_total{signal="log",reason="attribute_not_allowed"}`。
5. HTTP 指标固定为 `lanverse_http_requests_total(method,route,status_class)` 与 `lanverse_http_request_duration_seconds(method,route)`；没有 user/workspace/task/resource ID、URL 实参、query 或错误正文 label。

## 2. Red → Green

| 阶段 | 命令与真实结果 |
| --- | --- |
| 日志 Red | 首次运行定向测试在收集期因 `MAX_LOG_STRING_LENGTH`/`log_event` 不存在失败，证明原实现没有登记事件、字段白名单、丢弃计数和字符串边界。 |
| HTTP Red | request/metrics 定向测试为 1 passed、2 failed：非法文本 ID 被原样回显；access log 没有 event_name，因此无法证明模板路由和 status_class。 |
| Green | `test_logging.py`、`test_health.py`、`test_observability.py` 合计 6 passed；Ruff 通过，Pyright 0 errors/0 warnings/0 informations。 |
| 回归修正 | 首轮完整后端为 217 passed、1 failed、12 skipped；唯一失败是防枚举测试复用非法文本 ID并比较整包动态 request_id。测试改为复用同一合法 UUIDv7 后，安全语义不变且相关定向 10 passed。 |

## 3. 性能与全量证据

旧固定性能脚本使用 `perf-*` 文本作为 request ID，严格 UUIDv7 规则生效后无法回收日志耗时并按预期暴露 KeyError。脚本改为每个 list/readiness 请求生成真实 UUIDv7，保持 12/36/120 fixture、10 次预热、50 次样本、单进程和 120 镜头重排 30 次不变。`RUN_STORYBOARD_PERFORMANCE=1 backend/.venv/bin/pytest backend/tests/performance/test_storyboard_profile.py -q -s` 为 1 passed；36/120 镜头服务端 P95 为 19.04/67.93 ms，低于 800/2000 ms 门限。

最终 `make check` 通过 Ruff、Pyright、ESLint、TypeScript、后端 218 passed/12 个显式外部或性能开关 skipped、前端 17 文件/54 tests、pip check、Next.js 16.2.12 生产构建和 development/production Compose config。

`LANVERSE_E2E_BACKEND_PORT=8002 LANVERSE_E2E_FRONTEND_PORT=3001 npm run e2e` 为 8 passed；账号、媒体资产、项目、剧本无 Key fail-closed、分镜和上传到期调度页面闭环均正常。备用端口用于保留本机已占用 3000 的未知进程，没有终止或复用它。

## 4. 环境、命名与安全边界

本增量不需要 DeepSeek 或 Ark API Key，不安装或调用 Ollama/Ark SDK，也不伪造 Seedream/Seedance 成功。`.env.example` 与 `.env.production.example` 继续保留 `DEEPSEEK_API_KEY=`、`ARK_API_KEY=` 空占位。

新增测试文件使用语义化名称 `test_observability.py`，性能测试只把采样键改为规范 UUIDv7；没有增加切片代号式文件、第二套指标框架、日志数据库、搜索 UI、APM 或 Collector 容器。

## 5. 未接受项与残余风险

PT-OBS-002/003 仍为 proposed。当前 Outbox 的 trace_id 仍来自 request_id，尚未创建或传播 W3C traceparent；RabbitMQ header、Consumer remote context、Worker/DeepSeek/MinIO/ffprobe/FFmpeg span、采样、OTLP exporter 降级计数和 Collector 故障均未实现，不能把相关依赖或业务事实称为“端到端 trace 已打通”。

`/metrics` 当前仍随 API 路由公开，尚未落实生产内网绑定或独立鉴权；在生产发布前必须随 PT-OBS-002 固定访问边界并做未授权拒绝验收。数据库池、Outbox backlog、RabbitMQ inflight、Task/provider/storage/media 指标和告警阈值也未全量实现。

日志事件 allowlist 当前只登记 HTTP 两类事件；Scheduler、Publisher、Worker 和集成日志仍须在各自 trace 增量中迁移并补敏感字段/格式化故障测试。运行日志不替代 AuditEvent、Task、Inbox/Outbox 或 ScheduleFire，业务恢复仍只依赖 PostgreSQL 事实。

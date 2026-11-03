# DeepSeek分时结算验证（2026-11-04）

来源：需求04 SRC-AI-39→设计34→PRD21 CASE-BIL-13～16→Plan30。用户仅授权配置、实现和隔离验证；本轮未执行真实收费调用，也未激活新模型。此前SRC-AI-38分时待补齐的历史边界，由本记录说明本轮新增接受范围。

## 规则与来源

- [DeepSeek官方价表](https://api-docs.deepseek.com/zh-cn/quick_start/pricing/)于本轮复核：北京时间周一至周五非中国法定节假日09:00–12:00、14:00–18:00高峰，其余空闲半价。
- [国务院2026放假通知政府转载](https://www.beijing.gov.cn/zhengce/zhengcefagui/202511/t20251104_4258873.html)：元旦1/1–3、春节2/15–23、清明4/4–6、劳动节5/1–5、端午6/19–21、中秋9/25–27、国庆10/1–7。周末包括调休工作日仍空闲；不外推2027。
- [Chat Completion接口](https://api-docs.deepseek.com/api/create-chat-completion/)的created只说明completion创建时刻，未提供可直接采用的跨界扣费时刻合同。本轮不把它冒充供应商账单时间。

原per_1k_tokens规则中追加time_pricing=deepseek_cn_2026，基础价为高峰，空闲系数1/2。报价按高峰且缓存未命中估算，费用明细冻结reservation_period=peak、审核来源和原价格ID，积分沿原1元=10积分。结算使用原dispatch_started_at到Worker收到供应商结果后、归档前持久化的received_at；区间不超过5分钟且两端同费率才能确定。5分钟内不存在两个相邻费率切换，故不会仅凭相同端点漏掉中间高峰。缺失、倒置、跨费率、超过上限或未审核年份进入原provider_cost_unknown，不取恢复时钟、不猜零费用、不重发请求。

| 模型 | 高峰输入未命中/缓存/输出（元/百万Token） | 空闲 |
| --- | --- | --- |
| DeepSeek Flash | 2 / 0.04 / 8 | 1 / 0.02 / 4 |
| DeepSeek V4 Pro | 9 / 0.30 / 27 | 4.5 / 0.15 / 13.5 |

## 工程与配置证据

- 使用原发布应用命令追加两款模型价格v4与draft，配置ID与模型v2均复用；[配置快照](configurations.json)不含凭据，也不是新的运行配置来源。旧版本不改写，未启用、未确认试运行。
- 原pricevalidation和价格发布事务校验规则单位、缓存单价、CNY及DeepSeek文本归属；派发前检查年度支持。未加新表/迁移、定时任务、日历服务、依赖包或积分钱包，未修改.env或供应商Key。
- 时段测试先出现invalid pricing rule；补齐后时段、时区、七组节假日、周末/调休、一次取整、Pro费率及未知年份通过。PG结算先复现缺时间参数错误；接入原证据后高峰85/空闲43微元，单笔结算并分别释放15/57微元。
- 旧活动代码overlay使received_at测试失败；当前Worker通过并且恢复后时间相同、假传输仅1次。错误类型测试先复现invalid pricing input不能进入原待核验；映射后由原Activity输出不可重试provider_cost_unknown。
- 供应商归属测试先复现其他供应商可发布DeepSeek折扣；补齐事务校验后拒绝，DeepSeek文本正常追加。未审核规则派发测试先错误准入；补齐后派发时间和凭据绑定均未写入。
- 集成测试全部使用临时PG18及假传输/私有存储fixture；不是供应商真实收费、真实COS归档或上线验收。

## 命令与结果

最终版本受影响命令退出0：

```sh
go -C backend test -race -count=1 ./tests/operation ./tests/catalog ./tests/billing ./internal/operation/adapter/managed ./internal/operation/adapter/providerhttp
```

使用隔离PG18：operation/billing接glowloom_reference，catalog接glowloom_catalog；均加载仓库schema及agent_checkpoint，测试凭据只为fixture。结果分别104.125s、30.699s、12.924s、1.710s、1.399s。未配置的外部Temporal/Kafka/Codex/真实供应商场景可能跳过，不算真实接受。

- gofmt/goimports：本轮13个Go文件无待格式化项。
- `go -C backend vet ./internal/catalog/... ./internal/operation/... ./tests/catalog/... ./tests/operation/...`：退出0。
- `go -C backend run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.11.3 run --new-from-rev=HEAD ./internal/catalog/... ./internal/operation/... ./tests/catalog/... ./tests/operation/...`：退出0，0 issues。
- `GOTOOLCHAIN=go1.26.10 go -C backend run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./internal/catalog/... ./internal/operation/...`：退出0，0可达漏洞；1项仅在所需模块中、当前代码不可达。
- `git diff --check`及本轮Markdown相对链接检查通过。本轮无前端变更，未重新执行前端全量/build或浏览器视觉验收。

首次`go -C backend test -race -count=1 ./...`退出1，10项失败及原文保留在[失败清单](full-suite-first-failures.txt)，未跳过或改测试来掩盖：

1. TestCinemaKitPostgresYjsSaveEditReadAndDetach：与operation撤销outbox INSERT权限的测试共享数据库，出现42501；原权限恢复后定向Race复跑通过（4.986s）。
2. TestAgentQuoteAndConfirmShareSessionTransaction：全仓负载下3秒上下文超时；最终operation整包Race通过，包括该用例。
3. prompt的8项测试：同一个OPERATION_STORE变量被该包要求指向glowloom_receipt，而operation的引用保护测试要求glowloom_reference。使用独立receipt库复跑prompt整包Race退出0（6.290s）。未改代码或放宽数据库保护。

全仓首次失败仍如实保留，不将分包复跑描述为同一条全仓命令通过。提交前主干ca56fa06的[CI 41736921522](https://github.com/StephenQiu30/glowloom/actions/runs/41736921522)已失败于TestPostgresCheckpointerSaveAndResume：CI库缺agent.graph_checkpoint；本轮不扩范围修改Agent/CI。新提交CI以交付时实际状态为准。

## 运行边界

本轮供应商派发前后均为3条，新增0条；原Flash发布和项目预算不变，原manual的0.12元继续保留。DeepSeek两款均disabled/draft，后续真实收费验证必须另获授权。2026年度规则到期须审核后追加新版本；跨费率边界需要供应商扣费证据，当前按未知保留，不承诺自动结清。

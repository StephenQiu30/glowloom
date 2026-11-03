# 其他模型配置与计费补齐验证

日期：2026-11-03；代码基线`eb590ce0`。来源：[需求04 SRC-AI-38](../../../requirement/04-AI平台接入与预设管理需求.md#src-ai-38)；设计34→PRD21→Plan30。用户选择“先完成配置和代码，暂不收费调用”，本轮未确认任何生成、未增加预算、未激活新增模型。状态及后续条件以[BACKLOG](../../../../BACKLOG.md#beeftv-adoption)为准。

## 实际保存的配置

通过既有PublishModelVersion、PublishPriceRule及CreateRelease命令与当前有效管理员会话保存，不直接改业务配置表，不创建供应商连接、凭据或存储桶。9个模型各追加配置v2，共追加11份有官方来源与审核时间的价格、7份待验证发布；旧版本和旧零价草稿保留。安全配置快照见[JSON证据](remaining-model-configurations.json)，不是新增运行配置入口。

以下文本单价依次为未命中输入/缓存命中输入/输出，单位元/百万Token；视频按输出Token，不把示例秒价写成固定秒费率。

| 模型 | 已审核价格/范围 | 保存结果 |
| --- | --- | --- |
| Seedream 5.0 Pro | 单图≤261万像素0.30元，以上0.60元；现有1K/1.5K与2K分别使用基础价/2倍系数；输入首张免费，第2张起0.02元/张，每任务收取 | 配置v2、价格v2、待验证发布 |
| Doubao Seed 2.1 Pro | 6 / 1.2 / 30，常规在线文本 | 配置v2、价格v2、待验证发布 |
| Doubao Seed 2.1 Lite | 0.8 / 0.16 / 2.7，常规在线文本 | 配置v2、价格v2、待验证发布 |
| Seedance 2.5 | 不含输入视频时480p/720p为70，1080p为77 | 配置v2、价格v2、待验证发布 |
| Seedance 2.0 | 不含输入视频时480p/720p为46，1080p为51 | 配置v2、价格v2、待验证发布 |
| Seedance 2.0 Fast | 不含输入视频，480p/720p为37 | 配置v2、价格v2、待验证发布 |
| Seedance 2.0 Mini | 不含输入视频，480p/720p为23 | 配置v2、价格v2、待验证发布 |
| DeepSeek Flash | 高峰2 / 0.04 / 8；空闲1 / 0.02 / 4 | 配置v2；价格v2高峰、v3空闲；不创建发布 |
| DeepSeek V4 Pro | 高峰9 / 0.3 / 27；空闲4.5 / 0.15 / 13.5 | 配置v2；价格v2高峰、v3空闲；不创建发布 |

依据：[火山方舟模型价格](https://docs.volcengine.com/docs/ark/model-pricing)、[DeepSeek价格](https://api-docs.deepseek.com/zh-cn/quick_start/pricing/)。所有新增审核规则保留现有markup=1、ratio=10，不扩展为个人钱包或全平台商业承诺。DeepSeek按北京时间工作日高峰/空闲且涉及法定节假日，两个价格版本只审价留存，不把固定一档自动发布成全天准确结算；没有新增日历调价引擎。

两个OpenAI图片候选没有已保存凭据，本轮保持原状；没有复制其他平台Key、伪造验证或填写缺乏汇率依据的人民币价。

## 最小代码变更与反例

- 缓存：沿原per_1k_tokens增加可选`cached_input_micros_per_1k`。报价按全部未命中预留，实际从总输入扣除缓存计数后分别定价，最后一次向上取整；缓存计数缺失、负数或超过总输入拒绝结算。DeepSeek的prompt_cache_hit_tokens与方舟的prompt_tokens_details.cached_tokens统一成内部计数，冲突证据不转成0。
- 视频：复用per_1k_tokens，输入价为0，分辨率系数应用于输出Token。按官方尺寸、24fps、输出时长估算报价；实际只消费供应商completion_tokens。新增发布门禁仅对方舟Seedance 2系列放行此单位且要求输入单价0，不扩大其他视频协议。
- 视频边界：有输入视频的折扣/最低Token合同尚未接通，报价拒绝，不能套用不含视频价格；文本/全能模式的adaptive比例拒绝估算，2.5首帧模式必须adaptive时以已知最大尺寸面积估算。4K和其他未实现参数不纳入本次价格；原API适配能力不冒称所有变体已接受。尺寸与Token依据[官方视频API](https://docs.volcengine.com/docs/ark/create-video-generation-task-api?lang=zh)。
- Pro图片：沿既有per_image与参考附加计算，报价从冻结媒体条目计数，`size`参与分辨率价格；发布校验接受明确完整的参考收费字段及模型尺寸枚举。结算必须存在报价冻结参考数，并与原输入一致，缺失或矛盾时保持费用未知，不能用当前0项覆盖原消费。
- 显示：试运行面板把原operation.quote_detail传给现有费用组件，显示冻结积分及明细，不在浏览器重算费率；尚未结算的null金额不显示成“已结算”。

这些改动复用既有版本、operation、账本和页面，无新增钱包、服务、环境变量、API路由或业务库schema迁移。

## 验证事实

核心测试先观察失败：缓存规则被拒绝/计数丢失、视频无法报价、参考附加被报价门禁阻止、Token视频发布被拒绝；修复后通过。真实隔离PG反例证明：缓存回执从4个输入中扣除2个缓存后按fixture规则结算19微元；视频先受理再查询，供应商37Token而产物时长99999ms时按fixture价结算37微元；参考冻结数缺失或与输入冲突拒绝。fixture金额不是供应商实际费用。

| 检查 | 结果 |
| --- | --- |
| 受影响整包Race | 新建独立PostgreSQL18测试容器，使用提交的完整schema及checkpoint SQL；operation外部整包、catalog外部整包、managed/providerhttp内部包全部通过；97.486s / 29.509s / 1.807s / 1.547s |
| 前端定向 | provider-trial共7测试通过；新增费用明细传递断言先失败、补属性后通过；tsc、两文件ESLint、Prettier通过；实际页面发现null误标已结算，新增断言先Red、修正数字判定后最终7项回归覆盖 |
| 前端全量 | `pnpm exec vitest run --maxWorkers=2`：287文件通过、1文件失败；2069项通过、1项失败。失败为`tests/unit/business-storage-policy.test.ts`的`business modules avoid browser durable storage outside approved view and recovery stores`，检测到并行新增的`src/components/layout/explore-search.tsx`；保留该并行改动，不称全前端通过 |
| Go静态 | 受影响生产包go vet通过；goimports/gofmt；golangci增量检查0 issues |
| 漏洞检查 | 按项目Go1.26.10运行govulncheck v1.9.0：0可达、0导入包漏洞，另1项required模块非可达条目；未改依赖版本 |
| 首次全仓尝试 | 未通过：运行库结构克隆不适合作为所有测试基线，缺Canvas专库/Prompt固定receipt库，runtime权限及旧媒体位置约束亦不匹配；随后改用独立完整schema完成上述受影响整包接受。不能将其报告为全仓通过；失败名称保留在[环境尝试记录](remaining-models-initial-test-failures.txt) |

整包命令退出0不代表所有外部集成接受；跳过的真实服务、生产历史及其他依赖场景不计通过。全前端命令早于最后null误标修复，该增量由最终7项组件回归覆盖。前端未重复生产构建，仅按实际类型/组件检查与浏览器证据判断本次属性传递。

本轮原生Chrome实际打开已保存的Seedance 2.5配置，看到每千Token规则、官方来源及禁用的激活按钮。原项目可用0.64元，低分辨率视频样例估算高于该值，页面按既有预算门禁禁止准备报价；没有为核对页面提高预算。

Pro图片在同一项目通过实际HTTP免费报价：operation `c8e2365d-e932-4594-ac1c-fccddfca6b81`为quoted，2K、输出1张、参考0项，quote_micros=600000、estimated_credits=6、resolution_2K=2。页面实际显示“预计消耗6积分”、预留¥0.6及完整参考附加规则；未点击“确认并试运行”。截图为会话附件pro-free-quote.png，早于最后null结算提示修复；随后Mac锁屏，无法再次读取页面，该最后增量仅按已通过的组件回归/类型检查判断，不声称锁屏后再次视觉验收。

收费边界复核：供应商dispatch_started_at总数仍为3，Flash仍使用原active发布`5356577c-91a3-4742-b5b6-9fda815ac739`。原项目limit=1000000、reserved=120000、settled=240000、revision=7保持不变；首次manual请求0.12元预留继续保留，不改写旧账。新增9个模型均disabled，没有新的真实验证成功记录。

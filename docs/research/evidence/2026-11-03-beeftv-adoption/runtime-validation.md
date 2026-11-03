# 首个Seedream Flash真实运行验证

日期：2026-11-03；首轮代码基线`4ab20c1547a1babcc10fdb5df86fb46fcc75b5fd`。授权见[需求04 SRC-AI-36](../../../requirement/04-AI平台接入与预设管理需求.md#src-ai-36)，实施状态及剩余接受项以[BACKLOG](../../../../BACKLOG.md#beeftv-adoption)为准。本文记录本地真实观测，不扩展为其他模型或生产接受。

## 运行恢复与最小修复

初始12个候选模型均停用；业务库缺少价格来源的`source_url`、`reviewed_at`列，模型列表/详情返回503。核对目标为本机`glowloom`后执行已提交的`model_billing_evidence_incremental.sql`，补列和目标账务索引；未重放完整schema，未重写旧价格或账本。既有后端容器重启编译挂载代码，模型目录恢复。

发现并修复以下真实问题，各自保留Red→Green回归：

| 问题 | 最小修复与覆盖 |
| --- | --- |
| 详情503后一直加载 | `model-detail.tsx`显示持久失败状态和原ID重试；新增详情组件测试 |
| 试运行报价缺价格审核来源 | `provider_trial.go`查询包含价格来源及审核时间；真实隔离PG断言冻结费用明细 |
| Flash请求参数不兼容 | `driver_ark.go`省略非必填`sequential_image_generation`和`stream`，使用API默认单图/非流式；协议wire测试覆盖Pro/Flash/Lite/4.5，保持一次POST |
| Pro/Flash候选错误提供4K | 候选尺寸改为1K/1.5K/2K、默认2K；回归通过，目录版本2026-11-03.1，仅相应两型号更新审核来源 |
| 打开参考选择器后验证模式丢失 | `provider-trial.tsx`拒绝Select原生表单同步产生的非合法空值；真实Radix交互回归保留image2image及参考入口 |

参数依据为[火山方舟图片生成API](https://docs.volcengine.com/docs/ark/image-generation-api?lang=zh)：5.0 Pro/Flash不支持组图与流式字段，支持1K/1.5K/2K。此处修正协议及候选资料，不批量发布其他模型。

## 复用现有COS

复用`cos-main/env-v1`与已保存的既有桶、凭据；取消新桶创建，未创建CAM用户/密钥或数据库角色，统一`.env`内容未修改。旧MinIO使用已有`legacy-main`只读连接。

既有COS桶为公开读混合桶，不宣称整桶私有。仅为应用拥有的7个前缀增加禁止匿名GetObject的策略（前缀及例外见[设计08](../../../design/08-文件上传服务调研与架构设计.md)）；原有公开目录保持原状。真实探针观察：签名读写/删除自有探针成功，匿名GET403，版本控制状态为空；匿名HEAD仍200，本次未覆盖元数据保护，也未证明现有凭据最小权限。

沿既有维护命令使用本机独立管理员Unix socket连接，不回退为业务DSN：注册2个既有profile，绑定旧存储证据，242项已有素材/衍生对象经真实字节核验完成位置绑定，执行finalize及正常路由激活。没有删除旧业务对象。激活证据摘要`21469de30760f11986b56af3e078ee22a8f692964b3acbc61784808695b69279`；旧存储证据摘要`9f47a54eaf138653a8cd12880ad3ac8200101cf1fdcfee64b44149cd943081a7`。

激活时核对真实role=all后端容器及API/worker/relay健康端口，以短期操作员快照和正在执行的维护进程提供既有门禁所需运行证据；没有实现持久自动心跳上报。完成生成后实际1条路由、1条旧绑定、252条对象位置（包括新增生成产物）。操作员临时探针与辅助程序不进入产品代码。

## 发布与计费

只启用Seedream 5.0 Flash，模型ID`cb596b56-a463-42b1-9de6-3e2676f1353a`，供应商模型`doubao-seedream-5-0-flash-260915`。发布`5356577c-91a3-4742-b5b6-9fda815ac739`绑定模型配置v2、价格v2、连接配置v1；发布已active，模型已active且为`image.generate`平台默认。配置和历史请求采用不可变版本保留，未覆盖旧请求。

价格依据[火山方舟定价](https://docs.volcengine.com/docs/ark/model-pricing?lang=zh)，本次审核为0.12元/输出张、参考图片输入免费。价格规则`base_micros=120000`、`markup_multiplier=1`、`credit_ratio=10`，单位per_image；审核来源与时间已保存。单次输出1张，两个成功请求尺寸2K，图生图引用上一次真实生成的项目素材。

项目`167519c4-b910-4ad4-b821-c6bffaa01c87`的验证预算通过现有ChangeBudget命令与审计流程从0设为1元，未直接改账本。实际调用如下，每项持久provider_call派发数均为1：

| 原operation | 模式/结果 | 报价微元 | 已结算微元 | 费用说明 |
| --- | --- | --- | --- | --- |
| `827948cf-87fe-48ac-bbdf-486eaf6f84c1` | text2image；参数拒绝后manual | 120000 | NULL | 费用unknown，保留0.12元预留，不自动认定免费 |
| `d2b89dae-bd52-4683-9e5f-3ca91a187d22` | text2image；completed | 120000 | 120000 | 产物归档COS并就绪，记录实扣0.12元 |
| `2bc8ac6c-12b6-4c6a-86c2-dac47900eaee` | image2image；completed | 120000 | 120000 | 使用前项实际图片，产物归档COS并就绪，记录实扣0.12元 |

前两种模式在新发布下的model_validation均为succeeded；旧参数错误发布仍未验证，不启用。最终项目账本limit=1000000、reserved=120000、settled=240000、revision=7；可用640000微元。本地结算记录不替代供应商账户最终账单，首项仍需原请求可信费用证据。

首页通过实际UI选择项目与Flash、填写提示词并点击“检查费用”，免费报价成功显示“预留费用¥0.12”“预估积分2（向上取整）”，明细为价格版本2、per_image、单价120000、数量/输出1、CNY、加价倍率1，可用项目预算¥0.64。没有点击“确认费用并生成”。免费报价新增1个未确认报价operation；数据库共4个operation、仍仅3次实际派发，本次获准调用额度已全部使用。

积分2是现有整数展示：ceil(0.12×10)，金额账本仍为0.12元。个人积分余额、充值与会员页面仍未开放；不能把项目CNY账本描述为已完成个人积分钱包。其他11个模型仍停用，不把候选目录称为真实接入。

## 本轮验证

| 检查 | 实际结果 |
| --- | --- |
| 前端定向Vitest | model-detail、provider-trial、models-page共3文件10测试通过，核心新增反例先Red后Green |
| 前端类型与静态 | `pnpm exec tsc --noEmit`及4个修改TSX的ESLint通过；修改TSX的Prettier检查通过 |
| 真实PG回归 | 专用隔离容器内新空库加载schema；`go test -race ./tests/operation -run '^TestProviderTrialQuoteRequiresAdminAndBudgetAndCanConfirmDraft$' -count=1`通过 |
| 协议/候选Race | `go test -race ./tests/catalog ./internal/operation/adapter/providerhttp -run 'TestCloud|TestArk|TestSeedream|TestVolcengine|TestProtocol|TestDriver' -count=1`通过，目录版本补充后再次通过 |
| Go静态与格式 | 受影响3个生产包go vet通过；7个修改Go文件gofmt/goimports通过；受影响生产及测试包golangci `--new-from-rev=HEAD`为0 issues |
| 漏洞检查 | 受影响生产包govulncheck通过：0可达、0导入包漏洞；另1项required模块条目不可达，不宣称全依赖无漏洞 |
| 运行接受 | 本地桌面真实两模式成功、COS归档、发布及首页免费报价；历史工程全量测试见[原工程记录](engineering-validation.md)，未重复冒称本轮全量 |

本轮截图保存在本机会话附件：`text2image-settled.jpg`、`image2image-settled.jpg`、`homepage-flash-quote.jpg`。未提交凭据、签名URL、个人云账户截图或运行日志。完整生产构建、全量测试、窄屏/读屏与生产在途重放不在本轮新增验证结论中。

CI基线核对：[4ab20c15的CI](https://github.com/StephenQiu30/glowloom/actions/runs/41600987178)前端成功，后端失败于Agent的`TestPostgresCheckpointerSaveAndResume`，原因是测试库缺`agent.graph_checkpoint`。[父提交b03f9dee的CI](https://github.com/StephenQiu30/glowloom/actions/runs/41581981609)已有同一失败。相关Agent实现/测试及CI工作流不在本次改动内；本地定向通过不宣称主干CI全绿。

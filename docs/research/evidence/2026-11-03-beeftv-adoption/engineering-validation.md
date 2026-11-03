# BeefTV采用工程验证记录

日期：2026-11-03；代码基线：`b03f9dee14651ddfa44fc7f5ad4471cd5e90df72`。验证对象为该基线后的本轮实现，以下保留提交前共享工作区的实际验证记录；本次Git交付另见需求04 SRC-AI-35的后续授权和实际提交记录。状态、调度和未决条件以[BACKLOG](../../../../BACKLOG.md#beeftv-adoption)为准。

授权来源：[需求04 SRC-AI-35](../../../requirement/04-AI平台接入与预设管理需求.md#src-ai-35)；合同：[PRD20](../../../prd/20-模型与供应商接入扩展.md)、[PRD21](../../../prd/21-模型计价透明与结算恢复.md)。本记录只记实际观测，不把fixture当成供应商接受。

本次提交范围内文件逐项见[文件清单](worktree-files.md)。

## 环境与边界

使用既有专用PostgreSQL测试容器内新建的独立空库（access、billing，以及既有测试guard固定要求的reference和receipt库）。分别一次执行空库schema，并在对应测试环境变量指向这些隔离库后验证；未重放业务库、未启动/重启应用、未执行真实收费请求。终端日志保存在本机临时目录，不包含于产品代码或提交。

不传DB环境变量的最初catalog/operation/billing基线为775个测试/子测试动作通过、219个跳过；这些跳过不计入数据库接受。

## 模型接入（Plan29）

| 检查 | 实际结果 |
| --- | --- |
| 严格声明解析 Red | 新声明/注册边界回归先失败；独立审阅另复现直接保存父级重复键绕过 |
| 重复键修复 Red→Green | Create/Version原先返回201而断言422；修复PresetDocument整体解析后，重复声明/schema/config均拒绝且无保存调用，合法两入口仍201 |
| 隔离catalog全量 | `GLOWLOOM_TEST_CATALOG_DB_DSN=<isolated> go test ./tests/catalog -count=1` 通过（修复后11.608s） |
| 隔离catalog复核Race | 同隔离环境 `go test -race ./tests/catalog -count=1` 通过（18.891s） |
| operation/providerhttp Race | `go test -race ./tests/operation ./internal/operation/adapter/providerhttp -count=1` 通过；不等同全部operation数据库接受 |
| 生成API/同站代理 | `pnpm api:generate`、`pnpm api:check`通过；代理合同70 tests通过；精确GET-only新路径 |
| 前端全量 | `pnpm exec vitest run --maxWorkers=2` 279 files /2016 tests通过（156.41s） |
| ESLint/类型/构建 | `pnpm exec eslint .`、`next typegen`、`tsc --noEmit`、`next build`通过；typegen/build显式指定仅构建用loopback后端origin，未启动服务 |
| Go静态/格式 | scoped `go vet`、本次文件goimports通过；golangci `--new-from-rev=HEAD` 0 issues |
| 漏洞检查 | scoped `govulncheck@v1.9.0` 0 reachable /0 imported package漏洞，另1项未调用模块条目；不宣称全依赖无漏洞 |
| 独立复核 | 初审1项Important/P2，修复后round1关闭；原独立overlay回归由FAIL变为PASS，无未解决Critical/Important |

协议fixture验证现有文本协议路径、鉴权、载荷、成功和401/402/429，以及原异步查询和断流没有transport自动POST重试；operation/原Key/恢复联合验证由Plan30专门测试承接。

原有全scope静态检查问题与18个无关前端格式文件仍保留；本次修改前端文件Prettier通过。未为清理既有问题扩大修改范围。为通过原有组件/theme合同做了最小兼容调整：providers复用现有Button/ToggleGroup；审批卡复用Input、可访问名称与语义颜色，审批业务行为不变。

## 计价与结算恢复（Plan30）

以下为实际Red→Green观测，最终质量门禁和完整命令另列于本记录后续验证表。

| 场景 | 观测证据 |
| --- | --- |
| 参考附加数学 | `TestReferencePricingFrozenFactorsAndMissingUsage`：per_task=500000 /per_output=600000微元，基础系数、独立附加后仅一次markup/FX/ceil；最终参考计量缺失拒绝。该fixture不授权生产规则发布 |
| 来源一致性 | 无source_url/reviewed_at直接发布、预设Connect创建价格分别先复现绕过再拒绝；旧文档仍可解析，已有冻结价不回填 |
| 独立费用投影 | 缺失/空串/显式零，已实扣+上游pending/refunded冲突，超期、交付未知和可恢复状态均有表驱动；不产生退款流水，不从取消/失败推断免费 |
| 同目标并发 | `TestBillingBarrierSerializesCanvasTargetDespiteInputChanges`：同canvas/node并发确认原先2份reserve，修复后仅1份；模型、输入、revision或请求键不能改变目标身份 |
| 恢复HTTP | `TestRecoverHTTPPreservesOriginalOperationAndReceipt`：无证据409；原身份202及同幂等键回放，Outbox一次；跨组织404，查询不泄露供应商task |
| 单次生成联动 | 真实隔离PG + Temporal测试环境 + managed activity + HTTP驱动的synthetic transport：上游受理后活动回执丢失，GET刷新、恢复Outbox消费、Key draining/replacement及当前release改变仍POST=1；禁用原Key不借新Key请求 |
| 终态原交付 | `TestTerminalPaidManifestRestoresOriginalDeliveryWithoutNewGeneration`：真实成功manifest与可信cost，撤销权限后实际收尾failed并结算；恢复当前授权后专用原交付归档。GET保持failed而delivery=ready，重复归档无新POST、attempt或账本；staging缺失必须拒绝 |
| 元数据真实权限 | 真实PG Store+HTTP验证同session撤销、用户credential epoch、platform epoch、账号/session不匹配及跨组织普通成员失败；全平台元数据对有效平台管理员开放 |
| 错误说明 | 同402/429不同安全结构码反例；裸HTTP不证明用户余额不足，分类不作为零收费或退款依据 |
| 增量结构 | `model_billing_evidence_incremental.sql`只在独立billing库执行首次与重复升级；均完成，无业务库执行 |

原产物恢复仅沿固定`operation-delivery/<原UUID>`交付工作流和原暂存manifest，不重新进入生成工作流；operation终态、价格、预留和账本不修改。交付就绪独立于执行状态。准入和归档重新检查当前权限、冻结身份、已结算事实及审核状态。

## 独立审查与修复回归

Plan29初审的父级重复JSON键问题按Red→Green修复，round1复核关闭。

Plan30初审4项Important/0 Critical：DirectUse自动无来源零价、账务冲突投影与实际屏障不一致、多产物部分归档过早ready、manual/终态恢复接纳后停止轮询。原实现者统一修复，回归至少包含：

- DirectUse真实PG先观察新增价格1行，再修复为0行；候选与Key仍能保存并读取，缺价不能自动创建正式release。新价格共同写边界要求审核来源。
- GET与quote/单批确认/dispatch共用持久费用事实，覆盖上游pending/released/refunded/settled与本地状态及账本、操作、预留金额矛盾。
- 真实两个PNG、私有staging、FFprobe/FFmpeg、进程内S3 SDK协议wire及PG归档/审核：第一项passed、第二项403中断时应recoverable/blocked；恢复固定原交付身份后才ready；重复恢复无新生成或新账本。实际集合来自成功manifest，不用计划产物数替代。
- failed/manual恢复202后继续读取原operation，结果ready后展示；保持项目切换、组件离开及迟到响应清理。

修复前稳定快照的全仓Go Race JSON：3260个测试/子测试pass动作、907个skip、31个包通过、0失败。严格库名guard的prompt38项及media-reference3项以专库另跑Race；私有协作包105项通过。以上不冒称修复后所有外部场景接受；修复后覆盖门禁及复核结论以随后补充的最终验证表为准。

## 修复后验证汇总

| 检查 | 实际记录 |
| --- | --- |
| catalog完整Race | 371测试/子测试通过，0跳过、0失败 |
| operation完整Race | 最终稳定代码663通过、26跳过、0失败（76.005s）；包含compatibility与严格manifest缺失反例 |
| 最新核心Race | 48通过、0跳过，含真实reuse/legacy、完整产物、冲突屏障、合法overage |
| Go合并统计 | 原全仓、专库及最新受影响family按package+test去重，3345 pass、882 skip、0 fail；全仓命令早于部分修复，相应模块以最新family覆盖，不宣称所有外部集成接受 |
| 前端全量 | 284文件/2035测试通过（124.42s）；仍出现之前全量已有的Vite/jsdom navigation/TimeoutNaNWarning，无测试失败 |
| 生成与类型 | API一致性、Next typegen、tsc --noEmit、ESLint通过，ESLint0错误/0警告 |
| 最终生产构建 | `GLOWLOOM_BACKEND_ORIGIN=http://127.0.0.1:1123 pnpm exec next build`通过，编译、类型、35静态页及路由表完成；没有应用服务启动 |
| Go质量 | go vet ./...、本次文件goimports及diff --check通过；govulncheck0可达，另1项required module非可达漏洞，未升级无关依赖 |
| 既有静态基线 | 新改文件无新增静态问题；相关scope仍有既有2项operation测试import与catalog既有staticcheck等，全仓原始日志亦有旧项，不宣称全仓lint通过 |
| 既有格式基线 | 本次前端TS/TSX文件Prettier通过；全前端仍有18个无关旧格式文件，未扩范围格式化 |
| 其他模块 | 私有canvas-collab105项通过；专库prompt38项Race和media-reference3项Race通过 |

独立复核：Plan30原I-01～I-04全部关闭；round1新增R1-01（legacy0-based/shared asset reuse误判）在round2关闭，原真实PG overlay由FAIL转PASS。保留strict managed分支，已证明managed但manifest缺失不降级legacy。合法复用的独立output行、冻结关系、共享asset及历史NULL/skipped合同得到正反例覆盖，源task隐藏/旧目录停用不清空新结果。

## 跨模块终审修复与最终结论

独立全局审查初次发现1项Critical、6项Important及字幕表现问题，均有真实隔离反例；集中修复后又查出两项残余写入/真实SDK差异，继续以相同反例收口。最终独立复核全部关闭，**Plan29/30整体工程规格与代码质量通过，无未关闭Critical/Important**。不将上述结论扩展为生产、供应商或浏览器接受。

| 路径 | 最终观测 |
| --- | --- |
| 已发送HTTP拒绝 | 裸402/429、仅分类码或内容拒绝不能证明零费用。一次POST后manual/held、cost=NULL、同目标blocked；本地确未发POST=0仍沿原合同闭合。安全错误说明与费用事实分开 |
| 原观察不可被覆盖 | 真实429持久unknown后，迟到同attempt not_submitted拒绝；强制零结算也拒绝，原预留/流水不变。相同观察仍幂等，可沿原任务可信accepted/completed补证 |
| 历史错误零结算 | 旧rejected/0不作可信费用；已闭账不重写历史，读投影核查并拒绝同目标quote/single/batch/dispatch；未新增补偿或退款政策 |
| 终态账务与权限 | HTTP/relay/worker/实际媒体和文本写事务用同一费用事实及最新上游观察。当前已接纳请求的session/epoch/actor/project/目标逐阶段重检；原确认人禁用时合法团队成员仍可恢复，当前请求人失权则拒绝；原付费执行和Agent私有范围不放宽 |
| 实际媒体写边界 | 第一产物完成、第二对象传输期间制造账务冲突，第二写事务仍拒绝。绑定位置unknown/错误身份/键不可ready；未绑定历史归档与合法共享复用保留 |
| 固定恢复工作流接续 | 使用实际SDK v1.49.0与进程内gRPC、真实PG/outbox/inbox证明：占用冲突不消费新事件，旧请求失权失败后同一新事件能发起新run；重复事件不增加RPC，已ready新请求no-op，生成POST保持1 |
| 只查原任务 | Codex manual真调用原receipt查证，mock只按原请求键query；无合法查询能力拒绝。旧nil/DefaultVersion命令顺序保留，新固定check_original仍可查证，真实finalizer不对旧0豁免 |
| 页面与明细 | 202后manual/终态持续GET；丢回执沿用原键，已接纳后的下一次主动查证用新键；跨scope/in-flight清理。正式面板显示冻结mode/resolution系数，不另算价格；真实MP3原字幕有/无对照 |

最终覆盖：operation外部整包Race676 pass/26 skip，连同operation/media内部包733 pass/26 skip；catalog两组隔离运行按测试去重371 pass；专库reference3、prompt38通过。最后R1/R2定向Race105 pass/0 skip/0 fail（42.447s），vet、goimports、变更行lint0问题、diff检查通过。前端全量284文件/2038通过，随后key重挂载收尾由最终相关两文件49项通过覆盖；最终ESLint、typegen、tsc、API一致性及完整生产build退出0。上述各批次时间与代码范围区分保留，未把历史全仓命令冒称所有最终外部集成通过。

独立复核原样重跑了迟到回调、真实SDK/inbox两个反例：分别观察到拒绝覆盖/拒绝零结算/held，以及固定ID冲突事件可重投。真实Temporal服务响应由进程内wire提供，未启动Temporal服务。捕获的生产SDK历史目录未提供，相应重放测试跳过；testsuite marker对照不替代线上在途升级。

## 实施裁决与影响

1. 复用已有AdapterSpecification/ProtocolDriver、不可变预设、operation价格算法与billing账本，不建立第二注册表、钱包或计价引擎。代价是新协议仍需编译安装并补适配测试；受限声明不会变成脚本插件。
2. 工程验证时沿共享main工作区保留修改，不擅自扩大当时仅文档的推送授权；用户后续明确授权本次实现代码、测试、文档与BACKLOG提交main并推送。仅交付本次冻结范围，其他并行改动保留，CI与产品接受分别记录。
3. 全部新增价格版本统一要求审核来源；DirectUse无价候选可保存，价格审核前不创建零价或正式release。代价是无真实价格的新模型需要管理员补审；旧连接与冻结价格不自动失效。
4. 已付费终态原成功产物采用固定专用交付工作流恢复，保持原执行终态和账本。代价是新增一条严格受限的交付路径及相应worker部署兼容要求；不能借此重买。
5. 参考附加只实现严格数学与证据校验，未知逐模型计量合同拒绝正式发布；迟到原币账单/退款补偿等未确认合同保持关闭。代价是这些商业能力仍不可用，避免编造费率与退款流水。
6. 保留既有测试安全guard，确认目标库不存在后新建隔离空库，分开执行固定库名测试；未清理业务数据、未重放业务schema。代价是本机保留独立测试库和fixture记录。
7. 中文任务标题无法由技能脚本自动提取，实施brief逐字保留原Plan与实际接缝预检，审阅以拥有来源设计/PRD为准。代价是需要人工核对任务边界，实际已通过独立审阅检查。

8. 终审后的迟到矛盾回调和真实SDK冲突两项是可复现的关键缺口，继续限定在两个产品文件内收口并独立重跑原反例，没有作为已知风险留给发布。代价是增加定向账本/SDK/inbox验证；没有扩大商业政策或执行环境。

## 接受边界

没有本轮浏览器桌面/320/390、真实屏幕阅读器、真实供应商账号调用或生产迁移证据。组件测试、编译和隔离数据库均不能代替这些接受。未确认的价格政策、参考图计量合同、迟到原币账单和退款补偿保持相应门禁，未发布生产价表或钱包。


后续限定运行接受：用户补充授权后执行的首个Flash模型真实验证与必要修复独立记录于[运行验证](runtime-validation.md)。本文件的“未执行”均描述首轮工程验证阶段，不否认后续已留证的本地接受。

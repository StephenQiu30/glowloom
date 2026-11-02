# SRC-AI-33 实现与验收

2026-11-01，工作区基线`65ba1e64200a5541498e1e6aaf63c82d55347a58`。本轮用户明确“落实我们的设计和实现”。设计依据为[13场景Figma](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3342-67847)、[设计](../../../design/2026-11-01-AI服务测试与验证交互设计.md)与[实施Plan](../../../plan/2026-11-01-AI服务测试与验证实施.md)。此文记录工程证据，不代替真实供应商账号接受。

## 交付行为

服务列表名称进入概览，独立“测试”进入同一520px Sheet的测试页签；手机全宽。详情固定概览/测试/凭据/高级配置，内容内部滚动。管理主导航仍由共享顶栏呈现，与账号工具栏同排。采用现有语义颜色、Geist和shadcn组件。

三层检查独立：网络检测不带Key或生成参数，任意HTTP响应仅表示网络可达；凭据只读鉴权校验真实JSON协议；模型真实验证须显式准备并确认费用。语音/音乐缺少独立只读鉴权时显示中性“不支持”，正式媒体试运行保留项目、预算、参数和授权素材门禁。

检查持久化到provider_check，复用Idempotency/Outbox/Temporal/worker；无Key检测不读取凭据。仅受控地址GET、不跟随跳转，8秒超时、仅超时最多重试一次；只读鉴权15秒。检查共用供应商并发与速率计数。HTML、异常JSON不被标为鉴权通过，额度/限流/权限/地域/IP等安全分类不展示上游原文。

accepted和未结束unknown合并原请求，不能重复生成。异步查询绑定原TaskID、配置、模型版本与原Key，draining Key仅允许读取原任务，disabled仍禁止；历史结果不成为当前证据。同步未知可记录核查说明并结束追踪，结果仍unknown、不释放预算；下一次新验证重新确认费用并关联旧check。请求标识可复制，控制台链接仅使用公开根地址，不附带Key或任务私有URL。

正式release试运行复用operation：草稿和当前有效active release可验证；刷新恢复服务端本人原任务和稳定确认键；确定性事务拒绝允许修正表单，未知网络结果仍保留原请求。确认摘要来自冻结模型/模式/参数/输入与准备的费用上限。

## 审查与先失败后修复

使用一次独立完整改动代码审查，收到0个Critical、7个Important及1个Minor；费用摘要项升级为Important处理。修复后未再发起审查循环。

| 问题                            | 回归证据                                                               |
| ------------------------------- | ---------------------------------------------------------------------- |
| 刷新丢失正式试运行              | 服务端恢复+稳定confirm key；UI不创建第二次报价                         |
| 配置变化后排队网络检查挂起      | 旧pending安全关闭，不占用永久并发                                      |
| 已发布模型被草稿限定拒绝        | 当前active release报价、确认及实际dispatch admission通过（模拟供应商） |
| 旧模型版本沿用通过              | 服务端onboarding和前端按模型version匹配                                |
| 只读请求绕过速率                | 派发原子记录dispatch_started_at，第二请求被共享限流拒绝                |
| accepted产生第二次生成          | 返回原check，异步只查询原任务                                          |
| 确定性拒绝锁死报价表单          | 明确失败解除等待；不确定结果保留恢复                                   |
| 费用摘要不足                    | 冻结输入/模式/参数及原上限回显                                         |
| 轮换后无法查询旧异步任务        | 使用原配置与draining Key，恰好一次GET，零POST；结果仍历史              |
| 已结束unknown误报失败、丢失关联 | 保持待确认/核查说明；新请求关联旧check                                 |
| 鼠标关闭丢失焦点                | 费用确认返回准备按钮；Sheet在DataTable重绘后仍回原行入口               |
| 复制入口依赖画布上下文          | 浏览器复现错误后改用全站Toast；补无画布Provider渲染回归                |

定向回归、最终全量结果与命令摘录见[test-results.txt](test-results.txt)。原始本机日志位于`/tmp/glowloom-service-check-*.log`，仅用于定位，不包含用户Key。

## 自动化与隔离数据库

使用本轮独立PostgreSQL18容器`glowloom-service-check-pg18`、端口55439，catalog与operation分离数据库，workspace参考约束在专用`glowloom_reference`库运行。测试只用合成密钥与HTTP transport fixture。

- catalog、operation与providerhttp完整Race通过，使用各包要求的真实测试DSN环境变量，非因未配置数据库跳过。
- 项目预算归属的workspace Race定向通过，验证本人项目及跨组织拒绝。
- workspace完整Race仍有`TestListProjectsScopesStatusesRecycleBinAndLiteralSearchOnPostgres`失败：固定2026-09-27样例的回收期已经过期，回收列表为空。未改动该业务源码或断言。
- 先前完整Go运行仍有`TestCanvasSocketAuthorityDoesNotUpgradeSharedProjectLocks`失败（未等到项目写锁）。其他包若缺少专用DSN会跳过，本轮不把该次运行当数据库全量接受。
- `go vet ./...`及变更范围`golangci-lint run --new-from-rev=HEAD ./...`通过；全仓lint仍有原有媒体/适配器等检查项，不宣称全仓通过。
- govulncheck没有可达漏洞；存在一个未调用依赖模块提示。未改变依赖。
- 前端API一致性、相关UI回归、TypeScript及生产构建结果见命令摘录。完整前端原有4项失败为canvas-yjs-pending-journal持久化策略及advanced-source-combinations的3项文案断言；未放宽这些断言或改动相应源码。

并发运行完整测试/构建曾出现前端5秒超时及一项generation-dialog间歇失败，后续限定2个worker复验。期间还修正了自身TypeScript空值引用、测试exact参数与异步复制返回类型；只以最后一次结果为依据。

## 实际浏览器验证

Chrome，localhost:1122，使用现有管理员会话。没有创建或修改真实Key，没有点击真实生成确认，也没有执行真实凭据鉴权。

- OpenAI未配置Key时可以网络检测，鉴权与模型按钮禁用。
- 实际一次无Key网络请求经完整Outbox→Temporal→worker→结果链路完成：HTTP404，956ms，1次尝试，本地时间2026-11-01 01:00:11。仅证明地址可达，页面明确未配置凭据。
- 打开DeepSeek测试、准备真实验证、取消：CDP记录全部为GET，零生成/模型POST；不记录请求头、正文或Key。
- 费用确认取消后焦点回“准备验证”；同一视口关闭Sheet，焦点回`测试 OpenAI · ff7c557b`，包括DataTable重绘后的当前按钮。
- 管理导航与账号工具栏DOM纵坐标均22px。390和320px的documentWidth、panelWidth均等于视口宽度，未出现整页横向溢出；最后恢复默认窗口。
- 页面运行时错误已实际复现并修正，最终重新加载正常显示三层测试，不以只有单测通过代替浏览器验收。

截图：[页面/统一顶栏](browser-page.png)、[桌面测试](browser-desktop.png)、[390px](browser-mobile-390.png)、[320px](browser-mobile-320.png)、[真实费用确认弹窗](browser-fee-confirmation.png)。设计截图仍保留在同目录，不能当作浏览器运行截图。

## 本地结构变更与接受边界

开发库仅事务执行新增`provider_testing_incremental.sql`，未重放schema、重置数据或写入Key；隔离测试库连续重复两次通过。此增量包含nullable credential与安全诊断字段、类型约束，不含用户业务数据。新环境继续用schema.sql。其他环境需要按自身部署流程安装增量，不表示已部署生产。

真实Key有效性仍须管理员在“测试→检查凭据”验证只读权限，再选择模型、确认费用后进行真实调用。只读通过不能代表所有模型权限、费用额度或实际生成；语音/音乐按各自正式试运行合同验证。轻量测试仅代表固定短模板，不是所有模式/任意参数接受。

Git：本轮保留未提交可审查diff；早先main推送`65ba1e64`不包含本轮测试实现。当前共享工作区同时存在其他任务的证据清理、研究文档和导演素材脚本改动，均保留，未恢复、打包或推送。上一轮CI失败不被本轮本机验证覆盖。

## 执行裁决

- 沿用已授权main工作区；本轮不自动提交或推送。代价是远端尚无新增测试功能。
- 真实账号/收费接受不由fixture代替；本轮只运行一次不带Key的GET。代价是真实Key、额度与生成仍需按显式费用确认验证。
- 审查者未执行的浏览器和迁移由主任务实际验证；保留全量失败，不以局部通过宣称全仓绿色。代价是交付记录需要区分已观察与未接受范围。

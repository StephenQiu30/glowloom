# PRD-011 AI 提供方配置与启用

- 状态：accepted（2026-08-12 用户要求按项目设计规范开始开发；接受的是执行基线，PT-AIP-001～007 仍须分别以真实 Acceptance 证据接受）
- 日期：2026-08-12
- 输入：[REQ-014 AI 提供方配置与启用需求](../requirement/014-AI提供方配置与启用需求.md)、[DES-009 AI 提供方配置与启用设计](../design/009-AI提供方配置与启用设计.md)、[生产模块需求](../requirement/009-生产模块需求.md)
- 输出：7 个可独立接受的 `PT-AIP-*` 产品任务、量化验收标准和完整功能退出门禁
- 下游：[PRD-010 需求、设计与产品任务追踪矩阵](./010-需求设计与产品任务追踪矩阵.md)、[PLAN-011 AI 提供方配置与启用执行计划](../plan/011-AI提供方配置与启用执行计划.md)

## 1. 产品结果

Workspace owner 可以在不修改部署环境的情况下，从主流平台预设创建 AI 提供方配置，安全写入 API Key，检查连接与模型候选，并把一条已经通过真实契约的能力启用到明确业务用途。刷新页面后配置、状态和绑定仍来自 PostgreSQL；切换只影响新任务，已有任务沿用创建时快照。

产品完成必须同时避免两类误导：

1. 平台出现在预设中不代表能执行；catalog-only 必须明确显示“可配置、未准入”。
2. 凭据或健康检查成功不代表模型参数、价格、权限和副作用契约已验收；`ModelCapability.status != active` 时不能启用。

本 PRD 的首个真实闭环仅为 DeepSeek `script_structure`。火山方舟 Seedream/Seedance 可以完成目录和配置管理，但 D-004 未关闭前不能 active，也不计入图片/视频生成验收。

## 2. 用户与主路径

| 角色 | 能力 | 不允许 |
| --- | --- | --- |
| owner | 查看预设；创建/编辑/归档 Connection；写入/轮换/撤销 Key；模型发现；健康检查；启用/切换/停用用途；查看安全审计 | 回显或导出 Key；绕过 Capability/D-004；让切换改写运行中任务 |
| editor | 在 Studio 查看业务用途是否可用以及 preflight blocker | 读取 Provider 管理详情、端点、凭据状态或健康错误；执行任何管理写操作 |
| viewer | 查看被授权的业务任务/能力最小状态 | 进入 Provider 管理数据面或执行写操作 |

owner 主路径：

1. 进入 Workspace 设置的 AI 提供方页面，选择一个预设。
2. 系统填充平台、协议、官方端点/地域和认证字段；owner 输入显示名与 write-only API Key。
3. 保存只产生 Connection 和加密 CredentialVersion，不自动启用。
4. owner 显式执行模型发现/健康检查；系统展示候选、健康和受控 blocker，不展示上游正文。
5. 只有兼容的 active ModelCapability 可被启用到一个 usage_type；切换 Dialog 明确旧绑定和“仅影响新任务”。
6. owner 可轮换 Key、停用用途或归档无引用 Connection；历史任务和审计保持可追踪。

## 3. 产品基线与量化标准

| 项目 | 固定基线 |
| --- | --- |
| 首批预设 | 11 个稳定 ID：`deepseek`、`volcengine_ark`、`openai`、`anthropic`、`google_gemini`、`alibaba_bailian`、`moonshot_kimi`、`zhipu_glm`、`minimax`、`openrouter`、`custom_openai_compatible` |
| 业务用途 | `script_structure`、`image_generation`、`video_generation`；同 Workspace/用途最多一个 active Binding |
| catalog-only | 除已真实回归的 DeepSeek 文本能力外，首批其他平台默认 catalog-only；逐能力准入后才能改变 |
| 健康检查 | connect timeout ≤3 s、total timeout ≤15 s、响应体上限 4 MiB、同 Connection 最多 1 个在途检查、10 s 冷却、成功结果 10 min 内可用于启用 |
| 模型发现 | 同健康检查的网络上限；最多返回 500 个规范化候选，超出时截断并标记；不得自动创建 active Capability |
| 普通 API | 固定基准环境、20 并发、100 样本下，预设/Connection 查询保存和 Binding 命令 P95 ≤500 ms，不含外部 Provider 时间 |
| 并发 | 两个 owner 同时切换同一用途只允许一个 revision 成功；数据库任意时刻只存在一个 active Binding |
| 凭据输入 | write-only、非空、最大 16 KiB；成功提交后响应、DOM、前端状态和浏览器持久化均不包含原值 |
| 可访问性 | 键盘可完成添加、保存、检查、启用、切换、轮换与停用；定向 axe 扫描 0 serious/critical violations |

外部 Provider 完成耗时不属于平台 SLA；本产品只承诺自身超时、状态持久化和可解释错误。健康检查不得使用会产生模型输出或计费副作用的生成请求；平台没有无副作用探针时必须返回 `probe_unsupported`。

## 4. 产品任务

| ID | 产品任务与用户结果 | 交付边界 | 可执行验收 | 依赖 | 追踪 |
| --- | --- | --- | --- | --- | --- |
| PT-AIP-001 | 浏览主流平台预设：成员知道哪些平台可配置、哪些能力尚未准入 | 11 个版本化 ProviderPreset；协议、地域/端点模板、官方资料、能力提示、model discovery 策略；catalog-only/available 明确 | 稳定 ID 唯一；目录顺序和 schema 确定；无 Key/模型硬编码泄漏；全部官方链接有复核日期；除 DeepSeek 外默认不能被 UI/服务端误报为 active | DES-009 accepted | REQ-014 AIP-ENT-001、AIP-FR-001、AIP-IF-001、AIP-NFR-011 |
| PT-AIP-002 | 安全保存 Connection 与凭据：owner 刷新后仍能管理配置但看不到 Key | Workspace Connection CRUD/归档前置校验；write-only CredentialVersion；应用层认证加密；owner RBAC；审计 | 明文 sentinel 不出现在数据库可读列、备份样例、API、OpenAPI、日志、trace、Audit 或浏览器；密文篡改/跨 Workspace 互换拒绝；editor/viewer/跨空间写入拒绝；并发更新不覆盖 | PT-AIP-001；主密钥可用 | AIP-ENT-002/003、AIP-FR-002/003、AIP-IF-002/003/007、AIP-NFR-001–005/007/010 |
| PT-AIP-003 | 发现模型并检查连接：owner 能区分配置、认证和模型准入问题 | 受控 model discovery；无副作用 health probe；安全错误归一化；检查历史与有效期；SSRF 防护 | 正确/错误 Key、无权限模型、429、超时、不可达、非法 JSON、超大响应和 probe 不支持均得到稳定状态；§3 网络上限生效；loopback/私网/metadata/DNS rebinding/重定向被拒绝；失败不改配置/Binding | PT-AIP-002；目标 Provider 允许测试 | AIP-ENT-005、AIP-FR-004/005、AIP-IF-004、AIP-NFR-006/007/009/011 |
| PT-AIP-004 | 启用、切换和停用用途：owner 得到唯一且可解释的当前 Provider | Binding 门禁；Capability/credential/health/adapter 版本校验；事务切换；runtime resolution；审计 | 不健康、过期检查、catalog-only、D-004、版本不匹配均零写入；并发切换唯一 winner；切换前创建的请求保持旧快照，切换后请求使用新快照；停用阻断新提交但不改旧任务 | PT-AIP-002/003；存在 active Capability | AIP-ENT-004、AIP-FR-006–008、AIP-IF-005–007、AIP-NFR-004/005/008/011 |
| PT-AIP-005 | 轮换、撤销和归档：owner 能安全维护凭据生命周期 | 新 current/旧 retiring；引用任务保护；立即撤销确认；Connection 归档；主密钥轮换/恢复演练入口 | 正常轮换后新请求用新版本、非终态旧请求仍可恢复；有引用时撤销/归档被阻断；立即撤销产生明确受影响清单且不自动重发；备份无明文，密钥轮换前后可解密既有密文 | PT-AIP-002/004 | AIP-FR-003/010、AIP-NFR-003–005/008/010 |
| PT-AIP-006 | DeepSeek 数据库切换：现有真实剧本提取不再依赖 Provider Key 环境直读 | 一次性安全迁移命令；DeepSeek Connection/credential/binding；运行时快照；旧 env 单向切换 | 命令不打印 Key且重复运行不重复建事实；错误 Key 不激活；显式真实 DeepSeek 结构提取通过；切换后删除 Provider Key 环境仍可由数据库绑定执行；旧 Batch/Task 恢复不漂移；失败时不进入长期双读 | PT-AIP-004/005；真实 DeepSeek 账号/额度 | AIP-FR-009、AIP-IF-006、AIP-NFR-004/008/011 |
| PT-AIP-007 | Provider 管理工作台：owner 在真实页面完成全部控制面操作 | `/workspaces/providers`；卡片/Drawer/Dialog；三状态展示；权限态；OpenAPI 生成 client；响应式/键盘/脱敏 | owner 完成添加→保存→检查→启用→切换/停用→轮换/归档；editor/viewer 无管理详情；Key 保存后从 DOM/state/storage 清除；刷新状态不丢；并发冲突显示服务端当前值；`umi-openapi` 重生无漂移，定向 axe 达标 | PT-AIP-001–006 | AIP-FR-011、AIP-IF-008、AIP-NFR-012；其余 FR 的页面闭环 |

## 5. 横向验收矩阵

| 验收面 | 必须证明 | 明确不能外推 |
| --- | --- | --- |
| 预设 | 11 个 ID、官方来源、版本、schema、catalog-only 状态稳定 | 不证明这些平台的 API、模型或费用可执行 |
| 数据 | 四类 Provider 事实与现有 Workspace/Capability 使用复合租户引用；唯一 active Binding | 不证明存量生产数据库已支持在线升级 |
| 凭据 | 加密、AAD、篡改拒绝、轮换、撤销、备份/日志/前端无明文 | 不证明部署主密钥/KMS 的生产运维已选型 |
| 网络 | 官方 host 规则、自定义 endpoint SSRF 矩阵、超时/大小/重定向/限流 | 不证明所有 Provider 都支持无副作用探针或模型列表 |
| 状态 | 保存、健康、Capability、Binding 分离；切换事务和旧任务快照稳定 | 不证明自动 failover、安全重试或跨 Provider 结果等价 |
| DeepSeek | 真实账号下迁移、健康、绑定、结构化提取和恢复成功 | 不证明 OpenAI/Claude/Gemini/国内平台或 Ark 能执行 |
| Ark | Key 可安全保存，目录/状态可查询，D-004 blocker 准确 | 不接受 Seedream/Seedance 图片/视频生成、价格或取消对账 |
| UI/OpenAPI | 生成客户端、owner 闭环、无权限态、Key 清理、键盘/axe、刷新恢复 | 不允许页面直连 Provider 或手写平行 DTO |

## 6. 完整功能退出标准

PRD-011 的执行基线已经产品接受；只有以下证据全部存在时，才能声称完整功能完成并把 PT-AIP-001～007 全部 accepted：

1. PT-AIP-001～007 均有对应 Acceptance 回链；局部 PT 可先 accepted，但不能提前声称完整功能完成。
2. 固定 sentinel Key 的数据库、API、日志、trace、Audit、OpenAPI 产物、DOM 和 browser storage 全面扫描为 0 个明文命中；数据库中对应密文可正确解密，任意 bit flip 和跨记录互换均失败。
3. 真实 PostgreSQL 证明租户复合外键、凭据只追加、唯一 current credential、唯一 active Binding、并发 revision 和审计原子性。
4. 网络测试覆盖 IPv4/IPv6 loopback、RFC1918/ULA、link-local、云 metadata、DNS rebinding、跨 host redirect、超时、4 MiB 上限和有限重试。
5. 普通 API 在固定环境满足 §3 性能；Provider I/O 达到固定 timeout/cooldown/inflight 上限，不报告无法控制的上游完成 SLA。
6. 显式开启真实 DeepSeek 测试完成“数据库 Binding → 剧本结构提取 → Pydantic 校验 → Task/Batch 终态”，并证明错误 Key、限流、超时和未知结果仍沿现有恢复契约。
7. 既有无 Key E2E、剧本/资产/分镜联合链、生成 fail-closed、消息/费用/媒体回归通过；Ark Key 是否存在都不能越过 `provider_contract_unverified`。
8. 浏览器以 owner、editor、viewer 三角色完成权限和主路径；定向 axe 0 serious/critical，Key 不出现在 DOM、localStorage、sessionStorage、Redux/Query cache 快照或生成 client。
9. `umi-openapi` 生成两次无漂移，后端 Ruff/Pyright/Pytest、前端 lint/type/Vitest/Playwright 与仓库 secret/产物卫生门禁全部真实通过。
10. 对存量生产数据库仍保持明确 blocker，除非另有已接受并演练的数据迁移方案；空库测试不能满足这一条发布门禁。

## 7. 部分接受与发布语义

- PT-AIP-001 可以独立接受为“预设目录完成”，但所有 Provider 仍可能不可执行。
- PT-AIP-002/003 可以接受为“配置和检查控制面完成”，但没有 PT-AIP-004 就不能启用。
- PT-AIP-004/005 可以使用隔离测试 Capability 证明事务和版本算法，但不能以此接受真实 Provider。
- PT-AIP-006 只接受 DeepSeek `script_structure` 的数据库切换。
- PT-AIP-007 只有在页面调用真实后端并通过生成 client 时才能接受；Mock/静态卡片不算完成。
- S4、Seedream、Seedance、Candidate、媒体结果、费用结算和 Provider unknown/reconcile 仍由原 production PT 与 D-004 验收，不因 PRD-011 accepted 自动完成。

## 8. 外部条件与残余风险

- 本地/测试主密钥可以使用受控部署 Secret；生产 KMS/Secret Manager 具体产品和轮换责任人在发布前必须关闭，不阻塞空库本地实现。
- 真实 DeepSeek 测试需要用户授权的账号、额度和显式测试开关；缺少时 PT-AIP-006 保持 blocked，不以 mock 接受。
- Provider 官方 endpoint、认证和模型目录会变化；预设带 `verified_at` 和 catalog version，目录升级必须人工审阅，不能静默覆盖用户 Connection。
- 当前无存量 schema migration；若部署环境已有不可丢失数据，完整功能可在新环境 accepted，但该环境发布仍 blocked。

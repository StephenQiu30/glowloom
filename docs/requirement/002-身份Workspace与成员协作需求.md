# CUR-IAM 身份、Workspace 与成员协作需求

## 0. 文档元数据

| 项目 | 内容 |
| --- | --- |
| 文档 ID | CUR-IAM |
| 文档序号 | 002 |
| 版本 | 0.1 |
| 状态 | proposed |
| 当前产品阶段 | 为“从剧本到逐镜视频素材包”提供最小可闭合身份、空间和协作边界 |
| 负责人 | 产品负责人（待指定） |
| 评审人 | 产品、设计、安全、研发、QA（待指定） |
| 创建日期 | 2026-08-14 |
| 上游输入 | CUR-00 当前核心产品需求总览、最小团队协作与跨 Workspace 隔离场景 |
| 下游需求 | CUR-PRJ～CUR-PLT、CUR-SEC |

### 0.1 状态与设计边界

- 本文件从零定义当前产品真正需要的身份与成员行为，不继承历史登录实现、账号表、认证供应商或旧角色状态。
- Requirement 只约束用户可观察结果、授权事实、状态和恢复，不决定认证协议、身份供应商、令牌格式、数据库或代码目录。
- 当前协作模型固定为 Workspace 级 `owner/editor/viewer`，不增加项目级 ACL、自定义角色、企业组织树或公开分享链接。
- 当前每个 Workspace 恰有一个 owner。增加共同所有者属于后续需求，不能通过实现默认值提前引入。

## 1. 模块定位

CUR-IAM 是所有业务模块的授权事实源，负责回答：当前操作者是谁、会话是否仍有效、当前 Workspace 是否可写、用户在该 Workspace 中具有什么角色，以及成员变化后哪些访问必须立即失效。

它不拥有 Project、剧本、资产、镜头、任务、媒体或导出事实；业务模块不得从前端提交的角色名称推断权限，也不得建立第二套成员关系。

### 1.1 用户结果

1. 新用户能通过至少一种已验证身份路径进入产品，并拥有一个可工作的个人 Workspace。
2. owner 能邀请 editor/viewer、修改其角色、移除成员并安全转移所有权。
3. 成员只能查看和操作其 Workspace 内被角色允许的资源，知道资源 ID 也不能越权探测。
4. 登出、会话撤销、成员移除和 Workspace 归档能及时阻止后续写入、生成和下载。
5. 用户输入不会因会话过期而无提示丢失，但恢复输入不能绕过最新权限检查。

### 1.2 产品 KPI

| KPI ID | 指标 | proposed 目标 |
| --- | --- | --- |
| CUR-IAM-KPI-001 | 首次完成身份验证并进入个人 Workspace 的任务完成率 | ≥ 95% |
| CUR-IAM-KPI-002 | 跨 Workspace 成功读取、引用、修改或下载 | 0 |
| CUR-IAM-KPI-003 | 已移除成员在授权失效窗口后仍成功执行业务动作 | 0 |
| CUR-IAM-KPI-004 | Workspace 同时存在零 owner 或多个 current owner | 0 |
| CUR-IAM-KPI-005 | 邀请重复接受造成重复 Membership | 0 |
| CUR-IAM-KPI-006 | 会话过期造成未提交创作输入无提示丢失 | 0 |

## 2. 范围与非范围

### 2.1 当前范围

- 一种或多种经过验证的身份登录路径及统一 UserAccount。
- 首次进入时建立个人 Workspace，owner 可创建新的 Workspace。
- 会话查看、续期、登出、撤销和高影响动作重新验证。
- Workspace owner/editor/viewer Membership。
- 邀请创建、撤销、过期、接受和重复接受恢复。
- 成员角色变更、移除和单 owner 原子转移。
- Workspace 切换、归档、恢复和空白 Workspace 受控删除。
- 业务读取、写入、生成、主选、导出和原始媒体下载的统一授权门禁。
- 当前用户可见的安全会话和成员变更记录。
- 账号停用/删除请求的业务前置检查与交接。

### 2.2 明确非范围

- 企业 SSO、SCIM、组织树、部门、用户组、自定义角色和属性策略编辑器。
- Project/Episode 级成员、逐资源 ACL、临时访客角色和外部审片门户。
- 匿名协作、公开链接、无需 Workspace 身份的媒体或素材包下载。
- 实时多人编辑、评论审批流和在线状态。
- 把内部受控支持人员加入用户 Workspace 或允许其代替用户创作。
- 商业席位、按成员收费、套餐限制和成员用量计费。
- 认证协议、身份供应商、令牌格式、密码哈希算法等技术选型。

## 3. 角色与权限

### 3.1 Workspace 权限矩阵

| 能力 | owner | editor | viewer |
| --- | :---: | :---: | :---: |
| 查看 Workspace 内业务事实和预览 | ✓ | ✓ | ✓ |
| 创建/修改 Project、剧本、资产、分镜 | ✓ | ✓ | — |
| 提交 AI 任务、取消或安全重试 | ✓ | ✓ | — |
| 星标、拒绝、设置主选和重新确认 | ✓ | ✓ | — |
| 创建导出、下载单个原始候选和素材包 | ✓ | ✓ | — |
| 查看不含原始媒体的历史导出摘要/Manifest 摘要 | ✓ | ✓ | ✓ |
| 邀请成员、撤销邀请和修改 editor/viewer 角色 | ✓ | — | — |
| 移除 editor/viewer | ✓ | — | — |
| 转移 Workspace 所有权 | ✓ | — | — |
| 归档/恢复或删除空白 Workspace | ✓ | — | — |

viewer 当前不具有原始候选或镜头素材包下载权限。若以后需要外部下载或 viewer 下载，必须新增可撤销、可到期、可审计的交付需求；当前不得通过一个永久 URL 或隐藏配置绕过本规则。

### 3.2 授权原则

1. 服务端以当前 Session、Membership、Workspace 状态和目标资源归属共同决定授权。
2. 前端传入的角色、Workspace、owner 标记或下载允许值均不可信。
3. 无权与不存在对跨 Workspace 调用采用不可探测的统一结果。
4. 查询允许不代表下载原始字节；预览、Manifest 摘要和原始媒体下载是不同权限动作。
5. 内部受控操作者只按 CUR-SEC 的最小必要支持规则查看脱敏诊断，不获得业务创作权限。

## 4. 核心用户场景

### CUR-IAM-US-001 首次进入

用户完成身份验证后首次进入产品，系统建立个人 Workspace，并以 owner 身份展示“创建项目”下一动作。

### CUR-IAM-US-002 邀请协作者

owner 邀请一名 editor 和一名 viewer。editor 可以继续制作，viewer 可以查看进度但不能生成、选择主选或下载原始素材。

### CUR-IAM-US-003 成员被移除

owner 在 editor 打开项目期间将其移除。editor 尚未提交的文本可复制保存，但下一次业务命令和媒体访问必须失败，不能继续使用旧页面权限。

### CUR-IAM-US-004 转移所有权

owner 把 Workspace 所有权转给现有 editor。转移原子完成：新 owner 生效，原 owner 同时降为 editor，不出现短暂无 owner 或双 owner。

### CUR-IAM-US-005 会话过期恢复

用户编辑分镜时会话过期。页面保留可恢复草稿，重新验证后按最新 Membership 恢复；若成员已被移除，只允许复制输入，不允许提交。

## 5. 功能需求清单

| FR ID | 名称 | 优先级 | 状态 |
| --- | --- | --- | --- |
| CUR-IAM-FR-001 | 验证身份并建立统一账号 | P0 | proposed |
| CUR-IAM-FR-002 | 创建和切换 Workspace | P0 | proposed |
| CUR-IAM-FR-003 | 管理会话、登出与重新验证 | P0 | proposed |
| CUR-IAM-FR-004 | 创建、撤销和接受成员邀请 | P0 | proposed |
| CUR-IAM-FR-005 | 修改角色、移除成员与转移所有权 | P0 | proposed |
| CUR-IAM-FR-006 | 对所有业务与媒体动作执行统一授权 | P0 | proposed |
| CUR-IAM-FR-007 | 归档、恢复和受控删除 Workspace | P0 | proposed |
| CUR-IAM-FR-008 | 查询安全会话与成员变更历史 | P1 | proposed |
| CUR-IAM-FR-009 | 停用或删除账号前完成归属与数据交接 | P0 | proposed |

## 6. 详细功能需求

### CUR-IAM-FR-001 验证身份并建立统一账号

| 项目 | 需求 |
| --- | --- |
| 前置条件 | 用户可使用当前产品支持的一种已验证身份路径。 |
| 主流程 | 用户完成身份验证；系统识别或建立稳定 UserAccount；首次成功进入时建立个人 Workspace 和 owner Membership；用户进入该 Workspace 的项目入口。 |
| 备选流程 | 同一人以后通过已确认可归并的身份再次进入时，必须先经过明确验证后关联既有账号；无法证明归属时保持独立，不自动合并。 |
| 失败流程 | 身份验证失败、身份未验证、账号 suspended/deletion_pending 或必要依赖不可用时拒绝进入业务区，并返回不泄露其他账号的恢复动作。 |
| 业务规则 | 邮箱、手机号或外部主体标识不是业务显示身份；UserAccount ID 稳定；首次进入重试不得建立多个个人 Workspace。 |
| 输入 | 验证结果、显示名称和必要同意记录。 |
| 输出 | UserAccount、个人 Workspace、owner Membership 或稳定拒绝结果。 |
| UI 要求 | 登录失败不透露账号是否存在；首次成功后只提供“创建项目”主动作；不得要求用户理解认证协议。 |

### CUR-IAM-FR-002 创建和切换 Workspace

| 项目 | 需求 |
| --- | --- |
| 前置条件 | 用户拥有有效会话。 |
| 主流程 | 用户查看自己拥有有效 Membership 的 Workspace；owner 可创建新 Workspace；用户显式切换当前 Workspace，之后所有业务导航和命令都带入该上下文。 |
| 备选流程 | 只有一个 Workspace 时直接进入；上次 Workspace 已归档或成员关系失效时进入可访问列表。 |
| 失败流程 | 创建重复提交不得产生多个 Workspace；切换到无 Membership、已删除或不可读取 Workspace 时统一拒绝。 |
| 业务规则 | Workspace 名称不是身份且允许重名；切换 Workspace 不复制、迁移或合并任何 Project；当前 Workspace 只是会话导航上下文，不能代替服务端目标归属校验。 |
| 输入 | 创建字段或目标 Workspace ID。 |
| 输出 | Workspace、Membership、允许动作和下一页面。 |
| UI 要求 | 始终清晰展示当前 Workspace；切换前有未提交输入时提示保存或复制。 |

### CUR-IAM-FR-003 管理会话、登出与重新验证

| 项目 | 需求 |
| --- | --- |
| 前置条件 | 用户存在有效或可恢复会话。 |
| 主流程 | 用户可查看当前会话、主动登出当前会话或全部会话；高影响动作在会话风险升高或超过策略窗口时要求重新验证；成功后重新读取最新 Membership。 |
| 备选流程 | 普通只读页面可在会话临近过期时提示续期；未提交文本可以本地临时保留，但不能成为服务端业务事实。 |
| 失败流程 | 会话撤销、账号停用或重新验证失败后所有新命令拒绝；不得继续使用旧下载能力或旧角色。 |
| 业务规则 | 客户端页面打开不代表会话永久有效；续期不得恢复已失效 Membership；登出不删除业务历史。 |
| 输入 | 会话、登出范围、重新验证结果。 |
| 输出 | 最新会话状态、允许 Workspace 或稳定拒绝。 |
| UI 要求 | 会话过期与业务校验失败分开说明；恢复后回到原对象或安全入口；输入只能复制/恢复，不能自动越权提交。 |

### CUR-IAM-FR-004 创建、撤销和接受成员邀请

| 项目 | 需求 |
| --- | --- |
| 前置条件 | Workspace active；邀请者为 owner；目标角色为 editor 或 viewer。 |
| 主流程 | owner 输入受邀身份和角色；系统展示权限摘要；owner 确认后形成可撤销、会过期的 Invitation；受邀者以匹配且已验证身份接受后形成唯一 Membership。 |
| 备选流程 | 对已有成员邀请时直接显示当前角色；同一有效邀请重复发送返回原邀请；owner 可在接受前撤销。 |
| 失败流程 | 邀请已过期/撤销、身份不匹配、Workspace 归档、角色非法或重复接受时不建立额外 Membership。 |
| 业务规则 | 邀请不能授予 owner；Invitation 不等于 Membership；转发邀请不能使其他身份接受；邀请内容不得包含项目或创作正文。 |
| 输入 | Workspace、受邀身份、editor/viewer、有效期、幂等意图。 |
| 输出 | Invitation、接受结果或具体可恢复错误。 |
| UI 要求 | 创建前展示 editor/viewer 差异；邀请列表区分 pending/accepted/expired/revoked。 |

### CUR-IAM-FR-005 修改角色、移除成员与转移所有权

| 项目 | 需求 |
| --- | --- |
| 前置条件 | 操作者是当前 owner；目标 Membership 和 Workspace 修订仍为当前。 |
| 主流程 | owner 修改 editor/viewer 角色或移除成员；转移所有权时选择现有 editor，查看影响并重新验证；确认后新 owner 和原 owner→editor 在同一原子结果中生效。 |
| 备选流程 | owner 可先把 viewer 升为 editor，再另行转移；被移除成员以后可通过新邀请重新加入。 |
| 失败流程 | owner 不能直接删除自身 Membership；目标已变化、转移对象无效或重新验证失败时零写入。 |
| 业务规则 | Workspace 始终恰有一个 owner；角色变化不改写历史 actor；移除后旧 Membership 不复活，重新加入形成新 Membership 生命周期。 |
| 输入 | 目标 Membership、目标角色/移除/转移意图、期望修订和重新验证。 |
| 输出 | 最新 Membership 集合、会话失效摘要和审计关联。 |
| UI 要求 | 降权、移除和所有权转移使用不同确认；转移说明原 owner 将降为 editor。 |

### CUR-IAM-FR-006 对所有业务与媒体动作执行统一授权

| 项目 | 需求 |
| --- | --- |
| 前置条件 | 请求可关联当前 UserAccount、Session、目标 Workspace 和具体动作。 |
| 主流程 | 每次业务读取/命令、异步执行、媒体预览/下载和导出下载均重新校验会话、Membership、Workspace 状态、目标归属和动作权限；通过后才返回最小必要结果。 |
| 备选流程 | viewer 可读取业务摘要和受控预览，但不能下载原始媒体或包；editor/owner 可在其他门禁通过时执行制作动作。 |
| 失败流程 | 任一事实未知、Membership 已失效、跨 Workspace、Workspace archived 或角色不允许时 fail closed；不返回资源是否存在、标题、媒体规格或任务详情。 |
| 业务规则 | 异步 Worker 不能沿用用户提交时的前端 role；任务执行使用已固定授权上下文并在产生新外部副作用前按规则复核；权限拒绝不改写目标。 |
| 输入 | actor、Session、Workspace、目标稳定 ID/版本、动作和用途。 |
| 输出 | allowed 与最小上下文，或统一拒绝和诊断关联。 |
| UI 要求 | 只读状态明确；按钮隐藏不能代替服务端授权；权限变化后页面及时刷新允许动作。 |

### CUR-IAM-FR-007 归档、恢复和受控删除 Workspace

| 项目 | 需求 |
| --- | --- |
| 前置条件 | 操作者为 owner，Workspace 影响事实可确认。 |
| 主流程 | owner 查看运行任务、项目、媒体和导出影响后归档 Workspace；归档后所有成员只能按权限读取历史，禁止新写入、生成和新导出；owner 可恢复。 |
| 备选流程 | owner 可先处理运行中/unknown 任务后重试归档；完全空白 Workspace 可请求不可恢复删除。 |
| 失败流程 | 存在 unknown 或无法安全处置的新外部副作用时阻止归档；存在 Project、任务、媒体、导出、额外成员或审计保留义务时阻止硬删除。 |
| 业务规则 | 归档不移除 Membership、不改写历史；只有无业务事实、无额外成员、无保留义务的空白 Workspace 可硬删除；删除后名称可复用但 ID 不复用。 |
| 输入 | Workspace、目标状态、影响预检、期望修订和确认。 |
| 输出 | 新状态或完整阻塞清单。 |
| UI 要求 | 归档、恢复和硬删除明确区分；删除说明不可恢复。 |

### CUR-IAM-FR-008 查询安全会话与成员变更历史

| 项目 | 需求 |
| --- | --- |
| 前置条件 | 用户查看自己的会话，或 owner 查看当前 Workspace 成员历史。 |
| 主流程 | 用户查看当前和近期会话摘要并撤销；owner 查看邀请、角色变化、移除和所有权转移历史及受控诊断关联。 |
| 备选流程 | editor/viewer 只能查看自己的 Membership 与会话；普通用户看不到其他成员的认证细节。 |
| 失败流程 | 跨 Workspace、超出保留范围或权限不足时统一拒绝；依赖不可用时显示 unavailable。 |
| 业务规则 | 展示设备/位置使用粗粒度安全摘要，不返回令牌、认证凭据或不必要个人数据；历史只追加。 |
| 输入 | actor、Workspace、筛选和分页。 |
| 输出 | 脱敏会话、Invitation/Membership 变化和允许动作。 |
| UI 要求 | 用户能一键撤销异常会话；成员事件使用可理解动作，不展示内部协议字段。 |

### CUR-IAM-FR-009 停用或删除账号前完成归属与数据交接

| 项目 | 需求 |
| --- | --- |
| 前置条件 | 用户已重新验证身份并发起停用或删除请求。 |
| 主流程 | 系统列出用户拥有的 Workspace、运行任务、未完成所有权转移和受 CUR-SEC 保留约束的事实；满足前置后进入 deletion_pending，撤销会话并阻止新动作；后续处理遵循 CUR-SEC 数据生命周期。 |
| 备选流程 | 用户可以取消尚未执行的请求；先把 Workspace 转移给现有 editor，或删除完全空白 Workspace。 |
| 失败流程 | 用户仍是非空 Workspace 唯一 owner、存在 unknown 外部副作用或保留决策不可用时阻止完成删除并列出动作。 |
| 业务规则 | 账号停用不改写历史 actor；不能为完成删除而把历史操作者改成其他成员；数据删除和匿名化边界由 CUR-SEC 唯一定义。 |
| 输入 | UserAccount、重新验证、删除意图和影响预检。 |
| 输出 | blocker、deletion_pending 或已完成状态及可解释摘要。 |
| UI 要求 | 明确区分停用、退出 Workspace 和删除账号；说明哪些历史因安全/权利义务暂时保留。 |

## 7. 领域实体与关系

| 实体 ID | 实体 | 产品含义 |
| --- | --- | --- |
| CUR-IAM-ENT-001 | UserAccount | 一个自然人/操作者在产品内的稳定账号身份。 |
| CUR-IAM-ENT-002 | VerifiedIdentity | 已验证且可关联账号的登录身份，不等于 Workspace 角色。 |
| CUR-IAM-ENT-003 | Session | 一次可撤销、会过期且需重新验证的访问上下文。 |
| CUR-IAM-ENT-004 | Workspace | Project 隔离、协作和授权的唯一上级范围。 |
| CUR-IAM-ENT-005 | Membership | UserAccount 在 Workspace 中的一段 owner/editor/viewer 角色生命周期。 |
| CUR-IAM-ENT-006 | Invitation | 加入 Workspace 的可撤销、会过期且绑定受邀身份的意图。 |
| CUR-IAM-ENT-007 | OwnershipTransfer | 单 owner 原子转移的高影响决定。 |
| CUR-IAM-ENT-008 | AccountDeletionRequest | 账号停用/删除的影响、阻塞和处理状态。 |

## 8. 状态机与不变式

### 8.1 状态

- UserAccount：`active → suspended | deletion_pending → deleted`；恢复策略由安全/支持规则约束。
- Session：`active → expired | revoked`；expired/revoked 不回到 active，重新验证建立新会话事实。
- Workspace：`active ↔ archived`；只有完全空白时可进入 deleted。
- Membership：`active → removed`；角色修改形成新修订，removed 不原地复活。
- Invitation：`pending → accepted | expired | revoked`。
- OwnershipTransfer：`prepared → confirmed | cancelled | conflicted`；只有 confirmed 改变 owner。

### 8.2 不变式

1. 每个 active/archived Workspace 恰有一个 current owner。
2. 一个 UserAccount 在同一 Workspace 同时最多一个 active Membership。
3. Invitation 接受、Membership 创建和 owner 转移均幂等且不得半成功。
4. 角色、Workspace 名称和登录标识均不得代替稳定 ID。
5. 跨 Workspace 资源不可探测；目标 ID 泄露不改变授权。
6. viewer 不得提交外部生成、改变主选、冻结导出或下载原始媒体/素材包。
7. Session、Membership 或 Workspace 任一不可用时，新写入和新外部副作用 fail closed。
8. 角色/成员变化不改写历史 actor 和业务决定。
9. 内部受控操作者不是 Workspace 成员，不能代替用户执行创作命令。

## 9. 产品接口与事件需求

| IF ID | 能力 | 必要输入 | 用户可观察输出 |
| --- | --- | --- | --- |
| CUR-IAM-IF-001 | 身份进入/退出 | 验证结果、会话意图 | 账号、会话、允许 Workspace 或稳定拒绝 |
| CUR-IAM-IF-002 | Workspace 列表/创建/切换 | actor、创建字段或目标 Workspace | 有权 Workspace、当前上下文和允许动作 |
| CUR-IAM-IF-003 | 邀请管理与接受 | Workspace、受邀身份、角色、Invitation | 邀请状态、Membership 或错误 |
| CUR-IAM-IF-004 | 成员角色/移除/owner 转移 | 目标 Membership、期望修订、重新验证 | 最新成员集合或零写入冲突 |
| CUR-IAM-IF-005 | 统一授权 | actor、Session、Workspace、目标、动作、用途 | allowed 最小上下文或不可探测拒绝 |
| CUR-IAM-IF-006 | Workspace 生命周期 | 目标状态、影响预检、确认 | 新状态或完整阻塞 |
| CUR-IAM-IF-007 | 会话/成员安全历史 | actor、范围、筛选 | 脱敏历史、撤销动作或 unavailable |
| CUR-IAM-IF-008 | 账号停用/删除交接 | 账号、重新验证、影响预检 | blocker、处理状态和数据生命周期交接 |

需要通知业务模块的确认事实包括：MembershipRoleChanged、MembershipRemoved、WorkspaceArchivedOrRestored、OwnershipTransferred、SessionRevoked、AccountSuspended。通知只带稳定 ID、修订、时间和最小路由信息；重复通知不得重复撤销业务事实。

## 10. 非功能需求

| NFR ID | 类别 | 可测要求 |
| --- | --- | --- |
| CUR-IAM-NFR-001 | 隔离 | 在用户读取、直接 ID、业务命令、异步执行、预览和下载六类测试中，跨 Workspace 成功访问数为 0。 |
| CUR-IAM-NFR-002 | 授权失效 | Session 撤销、Membership 移除/降权或 Workspace 归档事实提交后，所有业务列表/详情/CUR-CAN 画布在 ≤5 秒内反映；高风险下载和外部副作用提交必须在执行时同步复核并立即阻止，不等待页面刷新。 |
| CUR-IAM-NFR-003 | owner 一致性 | owner 转移在并发和故障注入测试中 100% 保持恰有一个 current owner，零中间可观察半状态。 |
| CUR-IAM-NFR-004 | 幂等 | 首次 Workspace 建立、邀请发送/接受、成员移除和 owner 转移在 100 次重复/超时重试中只产生一个业务结果。 |
| CUR-IAM-NFR-005 | 交互性能 | 在单用户 20 个 Workspace、单 Workspace 100 个 Membership 的基准数据集上，Workspace 列表及授权读取 P95 ≤ 1 秒；成员列表 P95 ≤ 1.5 秒。 |
| CUR-IAM-NFR-006 | 可恢复输入 | 会话过期/网络失败用例中，已明确保存或本地暂存的创作文本可恢复率 ≥ 99%，恢复后 100% 重新校验最新权限。 |
| CUR-IAM-NFR-007 | 隐私 | 登录失败、邀请查询和跨空间拒绝中泄露账号存在性、成员列表或资源摘要的自动化用例数为 0。 |
| CUR-IAM-NFR-008 | 审计 | 登录安全事件、会话撤销、邀请、角色变化、移除、owner 转移、Workspace 生命周期和账号删除请求 100% 具有 actor、目标、结果、时间和诊断关联。 |
| CUR-IAM-NFR-009 | 可访问性 | 登录、Workspace 切换、邀请、成员管理、owner 转移和会话撤销满足 WCAG 2.2 AA；所有 P0 动作可仅用键盘完成。 |
| CUR-IAM-NFR-010 | 可用性 | 身份依赖不可用时，新会话和高风险动作 fail closed；已加载页面不得把未知权限显示为可写。 |

## 11. 异常与恢复

| 异常 | 用户可观察行为 | 恢复要求 |
| --- | --- | --- |
| 首次进入请求重复 | 只出现一个个人 Workspace | 回读同一账号、Workspace 和 Membership |
| 邀请重复发送/接受 | 不出现重复成员 | 返回现有 Invitation/Membership |
| 成员编辑期间被移除 | 下一命令拒绝，页面转只读 | 允许复制未提交输入，不允许越权提交 |
| owner 转移并发冲突 | 原 owner 保持，零部分变化 | 展示最新成员状态并重新验证 |
| Workspace 归档时存在 unknown 任务 | 归档阻断并列出任务 | 先按 CUR-PLT 对账/处理，再重新预检 |
| 会话过期 | 不伪装业务保存成功 | 重新验证并按最新权限恢复上下文 |
| 下载期间权限撤销 | 新下载请求和续期拒绝 | 已冻结历史不改写，用户联系 owner |
| 身份依赖不可用 | 不建立弱验证会话 | 显示 unavailable 和安全重试入口 |

## 12. 验收条件

| AC ID | FR | Given | When | Then |
| --- | --- | --- | --- | --- |
| AC-CUR-IAM-001-A | CUR-IAM-FR-001 | 新用户通过受支持身份完成验证 | 首次进入 | 只建立一个 UserAccount、个人 Workspace 和 owner Membership，并显示创建项目 |
| AC-CUR-IAM-001-B | CUR-IAM-FR-001 | 同一首次进入意图超时并重试 100 次 | 请求恢复 | 始终回读同一 Workspace，不产生重复 owner/Membership |
| AC-CUR-IAM-002-A | CUR-IAM-FR-002 | 用户属于 W1、W2 | 从 W1 切换 W2 | 当前上下文清晰变为 W2，W1 项目不混入任何列表或命令 |
| AC-CUR-IAM-002-B | CUR-IAM-FR-002 | 用户不属于 W3 | 直接提交 W3 ID | 返回不可探测拒绝，不泄露 W3 名称或状态 |
| AC-CUR-IAM-003-A | CUR-IAM-FR-003 | 用户在两个设备有 active Session | 选择全部登出 | 两个会话在授权失效窗口内均不能继续读取或写入 |
| AC-CUR-IAM-003-B | CUR-IAM-FR-003 | 用户有未提交分镜文本且会话过期 | 重新验证但 Membership 已被移除 | 文本可复制，服务端提交被拒绝 |
| AC-CUR-IAM-004-A | CUR-IAM-FR-004 | owner 创建 editor 邀请 | 匹配的已验证身份接受 | 只形成一个 editor Membership，重复接受回读相同结果 |
| AC-CUR-IAM-004-B | CUR-IAM-FR-004 | Invitation 已撤销或过期 | 受邀者打开并接受 | 不建立 Membership，显示重新联系 owner 的动作 |
| AC-CUR-IAM-005-A | CUR-IAM-FR-005 | O1 为 owner、E1 为 editor | O1 转移给 E1 | E1 成为唯一 owner，O1 同时成为 editor，全程无零 owner/双 owner |
| AC-CUR-IAM-005-B | CUR-IAM-FR-005 | owner 在预检后成员修订变化 | 确认旧转移 | 零写入并展示最新成员状态 |
| AC-CUR-IAM-006-A | CUR-IAM-FR-006 | viewer 可查看某视频候选 | 请求生成、设置主选或下载原视频 | 三个动作均拒绝，候选和选择不变 |
| AC-CUR-IAM-006-B | CUR-IAM-FR-006 | editor 的 Membership 被移除 | 5 秒后使用旧页面和旧媒体 ID | 业务命令、预览续期和下载均拒绝且不泄露额外详情 |
| AC-CUR-IAM-007-A | CUR-IAM-FR-007 | Workspace 有历史项目和包但无 unknown 任务 | owner 归档 | 历史按权限只读，新写入、生成和新导出全部阻断；恢复后可继续 |
| AC-CUR-IAM-007-B | CUR-IAM-FR-007 | Workspace 存在任一 Project/媒体/额外成员/保留义务 | owner 请求硬删除 | 删除失败并返回完整阻塞类型 |
| AC-CUR-IAM-008-A | CUR-IAM-FR-008 | owner 查看成员历史 | 筛选所有权变化 | 显示 actor、前后角色、时间和结果，不显示认证凭据 |
| AC-CUR-IAM-008-B | CUR-IAM-FR-008 | editor 查询其他成员认证详情 | 发起查询 | 被拒绝且不能通过筛选探测记录存在性 |
| AC-CUR-IAM-009-A | CUR-IAM-FR-009 | 用户仍是非空 Workspace 唯一 owner | 请求删除账号 | 处理被阻断并要求转移所有权或清理空白 Workspace |
| AC-CUR-IAM-009-B | CUR-IAM-FR-009 | 用户已完成归属交接且重新验证 | 确认删除 | 账号进入 deletion_pending、会话撤销，历史 actor 不被改写，后续按 CUR-SEC 处理 |

## 13. 开放问题与已决策事项

### 13.1 开放问题

| ID | 问题 | 未决期间处理 |
| --- | --- | --- |
| CUR-IAM-OQ-001 | 首发采用哪一种验证身份入口，以及是否允许多个身份关联同一账号？ | 至少交付一种已验证入口；不自动合并无法证明归属的身份 |
| CUR-IAM-OQ-002 | 是否需要强制多因素验证？ | owner 转移、账号删除等高影响动作至少要求重新验证；MFA 由安全评审关闭 |
| CUR-IAM-OQ-003 | 单 Workspace 100 成员是否符合真实协作规模？ | 作为性能基准，不作为商业席位限制 |

### 13.2 已决策事项

- 当前 Workspace 恰有一个 owner；owner/editor/viewer 是唯一授权角色，不引入项目级 ACL。
- viewer 可查看业务摘要和受控预览，但不能下载原始候选或素材包，不能生成、改变主选或冻结导出。
- 当前不提供匿名公开链接或外部接收者账号；owner/editor 下载后在平台外完成交接。
- Membership、Session 或 Workspace 状态未知时，对新写入、下载和外部副作用 fail closed。
- 账号删除不得改写历史 actor，实际删除/匿名化和保留边界由 CUR-SEC 统一定义。

## 14. 术语与变更历史

| 术语 | 定义 |
| --- | --- |
| UserAccount | 操作者在产品中的稳定账号身份。 |
| VerifiedIdentity | 已验证且可用于进入账号的身份来源。 |
| Session | 可撤销、会过期、可能需要重新验证的访问上下文。 |
| Workspace | Project 的协作、权限和隔离上级范围。 |
| Membership | UserAccount 在 Workspace 中的一段角色生命周期。 |
| Invitation | 建立 Membership 前绑定受邀身份、可撤销且会过期的意图。 |

| 版本 | 日期 | 变更 | 状态 |
| --- | --- | --- | --- |
| 0.1 | 2026-08-14 | 从零建立账号、Workspace、会话和最小成员协作闭环，关闭业务模块此前隐含的身份前置 | proposed |

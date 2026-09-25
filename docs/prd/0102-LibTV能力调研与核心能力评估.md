# 0102 LibTV 能力调研与核心能力评估

- 状态：**已接受**（2026-09-25）
- 日期：2026-09-25
- 目的：弄清 LibTV 的能力范围与实现设计，评估 Lanverse 真正要做的核心能力，并为 0301 的顶层架构提供依据
- 关联：[0101 产品需求文档](0101-产品需求文档.md)、[0201 功能需求规格](../requirement/0201-功能需求规格.md)、[0301 系统架构设计](../design/0301-系统架构设计.md)

> 信息分级：**【事实】** 有可复查的一手来源；**【推断】** 根据一手材料做出的工程判断，不代表 LibTV 内部真实实现；**【未知】** 无法从公开材料确认。

## 1. 结论

1. **LibTV 是“画布文档 + 模型聚合 + 对话式 Agent”三件事的组合**：画布是创作的主容器，模型通过声明式注册表接入，Agent 通过自然语言驱动画布上的工作流【事实 + 推断】。
2. **它的画布基于 React Flow**，节点按“资源 / 生成 / 编辑”三类动作组织，生成结果由后台任务按字段白名单回写到节点【事实】。这印证了 0308 的画布选型。
3. **全能参考（含参考音频）是主流视频模型的标准能力**：LibTV 注册的 Seedance 2.x 类模型支持最多 9 张图、3 段视频、3 段音频作为参考；其自有 director-video 模型支持 30 / 10 / 10【事实】。0101 P10 的决策在供应侧可行。
4. **LibTV 的弱项正是我们的机会**：它以画布文档为唯一容器，镜头表只是画布里的一个节点，缺少“整部剧 × 逐集 × 版本依赖”的结构化生产管理【推断】。Lanverse 应坚持“结构化生产对象为事实源、画布为视图”，同时吸收它的节点模型、模型注册表和任务回写机制。
5. **顶层架构必须预留的扩展点**（第 6 节）：统一的“生成操作”抽象、声明式模型注册表、节点化的画布文档、面向 UI / Agent / 开放接口一致的命令层、可替换的执行引擎。这些在 MVP 就要定型，功能可以后置实现。

## 2. 调研方法与限制

| 来源 | 内容 | 访问日期 |
| --- | --- | --- |
| S1 [liblib.tv 首页](https://www.liblib.tv/) | 导航、功能入口、模型入口、模板分类 | 2026-09-25 |
| S2 [liblib.tv 产品介绍页](https://www.liblib.tv/wappro?sourceid=040004) | 产品定位与宣传数据 | 2026-09-25 |
| S3 首页与画布页的公开前端资源（HTML 内嵌数据、147 个公开 JS 文件与 16 个样式文件，解压后静态文本分析） | 模型注册表、节点类型、节点动作、任务可写字段、合规字段、前端技术栈、Agent 协议 | 2026-09-25 |
| S4 [libtv-labs/libtv-skills](https://github.com/libtv-labs/libtv-skills)（提交 `c609246`） | 公开 OpenAPI：会话、消息、上传；Agent 调用方式 | 2026-09-25 |

**限制：**

- 未登录、未创建项目、未调用任何生成接口；未读取需要登录的飞书帮助文档。
- S3 为对公开下发的前端代码做关键字与结构的静态阅读，**只能说明前端的数据形态和交互设计**，不能证明后端实现、质量、价格或内部架构。
- 前端版本随时变化，下述字段与模型清单只代表调研当日的快照。
- 本次联网搜索工具不可用，未检索第三方评测与新闻。

## 3. LibTV 能力地图

### 3.1 产品入口【事实，S1、S2】

| 类别 | 能力 |
| --- | --- |
| 创作容器 | 新建画布创作、项目、资产、插件与扩展 |
| 生成 | 视频生成、图片生成、音频生成、剧本生成、智能剪辑 |
| 独家功能 | 剧本原创与改编、导演执导、角色造型室、创意片头；宣传“20+ 独家专业功能” |
| Agent | LibTV Agent；宣传“100+ AI 导演 Skill，让 Agent 参与视频创作” |
| 3D | LibTV 3D-BOX（导演台：搭建 3D 场景、截图作为构图参考） |
| 模型 | 宣传“50+ 顶尖模型聚合”；首页置顶 Minimax H3 Max、Wan 3.0、Seedance 2.5 |
| 社区 | TV Show 作品展示；精选画布 / 模板分类（全网爆款、一场戏的诞生、短剧漫剧、商业广告等）；创作者挑战赛 |
| 商业 | 积分、会员；宣传“Seedance 2.5 最低 0.4 元/秒”（会员价，时效性信息） |
| 定位语 | “剧本 · 角色 · 世界观”“无限画布 · 自由编排 · 智能 Agent”“工作流 · 社区分享” |

### 3.2 画布节点类型【事实，S3】

| 节点 | 界面名称 | 说明（原文） |
| --- | --- | --- |
| `TEXT` | 文本 | 剧本、广告词、品牌文案 |
| `IMAGE` | 图片 | 海报、分镜、角色设计 |
| `VIDEO` | 视频 | 创意广告、动画、电影 |
| `AUDIO` | 音频 | 音效、配音、音乐 |
| `SCRIPT_V2` / `SCRIPT` | 脚本 | 创意脚本、生成故事板 |
| `VIDEO_CLIP` | 智能剪辑 | 多个视频片段合为一个 |
| `SHOT_BREAKDOWN` | 逐帧拉片 | 拆解视频的风格、运镜与音频 |
| `DIRECTOR_CONSOLE_3D` | 导演台 | 搭建 3D 场景，截图作为构图参考 |
| `REFERENCE` | 参考节点 | 备忘文本；连线仅为示意 |
| 其他 | — | `GROUP`、`VIDEO_GROUP`（一组视频任务）、`DIRECTOR_STUDIO`、`SPACE_SCENE_720`（全景空间）、`VIDEO_STORY`、`SCREENPLAY`、`MATERIAL_STYLE` / `MATERIAL_LENS`（风格 / 镜头素材，连到生成节点时自动带入模型或参数）、`CUSTOM` |

**节点动作**【事实，S3】：`TEXT_GENERATE`、`IMAGE_GENERATE`、`IMAGE_EDIT`、`VIDEO_GENERATE`、`VIDEO_EDIT`、`AUDIO_GENERATE`、`SCRIPT_GENERATE`，以及 `TEXT_RESOURCE`、`IMAGE_RESOURCE`、`VIDEO_RESOURCE`、`AUDIO_RESOURCE`、`VIDEO_CLIP_RESOURCE`、`SHOT_BREAKDOWN_RESOURCE`、`REFERENCE_NODE`、`DIRECTOR_CONSOLE_PANORAMA_INPUT`。即同一类媒体节点可以是“资源”（上传或已有结果）、“生成”或“编辑”。

### 3.3 脚本节点就是镜头表【事实，S3】

`SCRIPT_V2` 节点的可写字段：

- **节点级**：标题、镜头列定义、生图配置、资产、风格描述、原始故事文本、主题、总时长、提示词批量运行记录。
- **每行（一个镜头）**：镜号、时长、剧情描述（含实体引用）、角色、视频参考、镜头语言（cinematics）、景别、情绪、场景资产 ID、道具标签与道具资产 ID、光影氛围、音效、台词、逐句台词、旁白、BGM、音效、生图提示词、视频运动提示词、最终提示词中的实体引用、图生视频运动提示词（及用户改写版）、**图片版本列表、视频版本列表**、提示词状态、`textHash`、`payloadHash`。

### 3.4 模型注册表【事实，S3】

首页内嵌约 100 余个模型条目，每条包含 `modelKey`、`modelName`、`modelVendor`、`baseType`、`generateTypes`、`modeType`、参考上限（`imageMax`、`videoMax`、`audioMax`、`imageMaxWithVideo`）、`enableSound` 等，以及**声明式参数表单**（字段 `originalField`、`displayName`、`component`：`input` / `singleSelect` / `slider` / `switch` / `singleButton` / `voiceCard`，`enum`、`default`、`rules`、`forModeTypes`）。

| 类别 | 例子（按注册表原名） |
| --- | --- |
| 多模态文本 | 多模态文本模型、Qwen、DeepSeek、Seed 系列 |
| 生图 / 改图 | Seedream 4.x / 5.x（含分层）、Flux 2、Midjourney V7 / V8 / Niji、Qwen Image / Edit、Kontext 编辑、全能图片模型、director-image |
| 视频 | Seedance 1.5 / 2.0 / 2.5（含样片模式）、可灵 2.x / 3.0 / Omni / 动作控制、Wan 2.x / 3.0、Hailuo / MiniMax H3、Vidu Q2 / Q3、Pixverse、MJ Video、多镜头视频模型、director-video |
| 视频工具 | 高清放大（Topaz、智能超分）、字幕擦除、人像抠图、动作迁移、OmniHuman 1.5（数字人） |
| 音频 | MiniMax speech、音色设计、智能语音、Mureka 音乐 |

**视频生成模式**（`modeType`）：`text2video`、`singleImage2video`、`frames2video`（首尾帧）、`video2video`、`mixed2video`（多种参考混合输入）、`longVideo2video`。

**参考上限示例**：Seedance 2.0 / 2.5 与 MiniMax H3 类为 图 9 / 视频 3 / 音频 3；director-video 为 30 / 10 / 10；Wan 3.0 为图 10。

### 3.5 Agent 与开放接口【事实，S4】

- 公开 OpenAPI：`POST /openapi/session`（创建会话或发送消息）、`GET /openapi/session/:id?afterSeq=`（按序号增量拉取消息）、`POST /openapi/session/change-project`、`POST /openapi/file/upload`（图片 / 视频，≤ 200 MB）。
- 会话绑定项目；项目地址即画布地址；Agent 回复中带结果 URL。
- Skill 说明中的原则：**调用方只传原话，不拆任务、不写提示词**；拆分镜、选模型、写提示词、编排工作流都由后端 Agent 完成。
- 宣称的复杂能力：一句话生成短剧（剧本 → 分镜 → 成片）、复刻视频风格、音乐生成 MV、局部修改、元素替换、镜头调整、风格迁移、视频续写。

### 3.6 前端技术栈【事实，S3】

依据代码中的库特征与运行时标记识别（只能证明前端使用，不代表后端）：

| 类别 | 识别结果 |
| --- | --- |
| 框架与构建 | Next.js（Turbopack 构建产物）、React 19 |
| 样式与组件 | Tailwind CSS；Radix UI（`data-radix-*`）；同时存在 Ant Design v5（CSS-in-JS 变量），即两套组件体系混用 |
| 画布 | React Flow |
| 状态 | Zustand + Immer |
| 数据校验与请求 | Zod（大量使用）、axios |
| 富文本 | ProseMirror（Tiptap 相关代码），用于脚本 / 文本编辑与实体引用 |
| 长列表 | TanStack Virtual（`useVirtualizer`） |
| AI 输出渲染 | Streamdown（流式 Markdown） |
| 图标 | lucide |
| 未发现 | TanStack Query、Redux、i18next、Yjs、socket.io |

### 3.7 Agent 实现【事实，S3】

| 项 | 识别结果 |
| --- | --- |
| 前端框架 | **CopilotKit**（`useAgent`、`frontendTools`、`humanInTheLoop`、`useFrontendTool`、Agent 状态渲染） |
| 协议 | **AG-UI** 事件流：`RUN_STARTED`、`TEXT_MESSAGE_CONTENT`、`REASONING_MESSAGE_CONTENT`、`TOOL_CALL_START`、`TOOL_CALL_RESULT`、`STATE_SNAPSHOT`、`CUSTOM` |
| 传输 | 自建 WebSocket：`wss://<媒体 API 域名>/ws?token=…&project_id=…&agent_name=…&agent_version=…`，按项目建立连接；另有基于 SSE 的 AG-UI HTTP 通道代码 |
| 改画布 | 后端通过 `CUSTOM` 事件 `canvas_patch`（会话、序号、动作、节点列表、连线列表）下发画布补丁；前端校验节点属于当前项目后应用，并定位到新节点 |
| 发起生成 | 服务端工具 `create_generation_task`；结果以 `TOOL_CALL_RESULT` 返回，业务错误（如积分不足）带错误码 |
| 调试 | Agent 调试抽屉：Context、Events、Send、Local state |

### 3.8 合规与资产【事实，S3】

- 图片节点：`portraitAssetId`、`portraitCompliantExempt`、`portraitComplianceCheckedAtMs`、`protectionType`、`copyrightChain`。
- 视频节点：`assetVideoAssetId`、合规豁免与检查时间、`assetVideoCertifiedInPlace`、字幕地址。
- 存在用户级角色库接口（角色创建、解析、查询错误类型）。

## 4. 实现设计分析

| # | 设计点 | 证据 | 我们的判断 |
| --- | --- | --- | --- |
| D1 | 画布基于 React Flow | 【事实】前端含 `react-flow__node / edge / minimap / handle / controls / background` 等类名 | 与 0308 D8 一致；React Flow 足以承载同类体验 |
| D2 | 画布文档是主要数据容器 | 【事实】节点直接保存 URL、文本、镜头行、版本列表；`loadWorkflowCanvasData` 按节点与连线加载 | 【推断】对自由创作友好；对“整部剧逐集生产、跨集依赖、成本归集”不友好，需要大量约定在节点字段里维护 |
| D3 | 后台任务按白名单回写节点字段 | 【事实】`registerWritableByTask(节点类型, 可写字段)`，并有 `taskWhitelistEnforce` 开关 | 【推断】解决“人和任务同时改同一节点”的冲突：任务只能写结果字段。我们应采用同样的“结果字段归任务、配置字段归人”的划分 |
| D4 | 节点动作区分资源 / 生成 / 编辑 | 【事实】`*_RESOURCE`、`*_GENERATE`、`*_EDIT` | 同一媒体类型既可作为输入素材也可作为生成结果，统一了“上传”和“生成” |
| D5 | 连线表达输入 | 【事实】连线时根据上游节点类型自动填充下游参数（如风格素材节点连到图片生成节点时带入模型） | 连线 = 参考 / 输入；与 0301 “参考连线即参考组合”一致 |
| D6 | 声明式模型注册表 + 参数表单 schema | 【事实】`modelKey`、`modeType`、参考上限、`component` / `enum` / `rules` | 【推断】新增模型主要是配置工作，界面按 schema 渲染参数。我们应采用同样的“模型能力 + 参数 schema”登记方式 |
| D7 | 镜头表是一个节点 | 【事实】`SCRIPT_V2.rows[]` 含镜头全部字段、图片 / 视频版本 | 【推断】LibTV 的分镜是“画布里的一张表”；我们的镜头是一等业务对象，画布上的镜头表节点是它的视图 |
| D8 | 版本以列表挂在节点上 | 【事实】`imageVersions`、`videoVersions`、`textHash`、`payloadHash` | 与我们的“候选 / 选定 / 输入 hash”思路一致 |
| D9 | 对话式 Agent 驱动画布 | 【事实】会话、消息序号、结果写入项目画布 | 【推断】Agent 与人使用同一套画布操作；我们需要一层 UI / Agent / 开放接口共用的命令层 |
| D10 | 协作 | 【事实】存在协作开关（`createCollabFlag`）；未发现 Yjs 等 CRDT 库；WebSocket 主要用于 Agent 通道 | 【未知】多人协作协议与冲突处理方式 |
| D13 | Agent 通过工具与事件操作画布 | 【事实】CopilotKit + AG-UI；`canvas_patch` 自定义事件改画布；`create_generation_task` 服务端工具发起生成 | Agent 与人使用同一套画布操作语义；我们采用同样的协议，但画布修改与付费生成须经命令层与用户确认（见 0301 §7.4） |
| D14 | 前端技术栈 | 【事实】Next.js、Tailwind、Radix、React Flow、Zustand + Immer、Zod、ProseMirror、TanStack Virtual | 与 Lanverse 选型高度一致；技术对齐见 0308 §10 |
| D11 | 模板与社区 | 【事实】作品带 `projectUuid`、`templateUuid`、`snapshotId`、最终成片地址 | 【推断】项目有快照；模板是某个快照的可复制副本 |
| D12 | 合规字段进入数据模型 | 【事实】肖像合规、版权链、保护类型 | 合规状态应是媒体资产的属性，而不是事后检查 |

## 5. 核心能力评估

### 5.1 能力对照

优先级沿用 0201：MVP / V1 / V2 / 待定。“对照”列说明与 0201 功能需求的关系。

| 能力 | LibTV | Lanverse 定位 | 优先级 | 对照 0201 |
| --- | --- | --- | --- | --- |
| 整部剧导入、分集、结构解析、原文追溯 | 剧本生成 / 改编；脚本节点（单部内容） | **核心差异**：整部剧结构化 | MVP | SCR-01～05 |
| 全剧设定集（角色 / 造型 / 场景 / 道具）与锁定参考 | 角色造型室、资产库 | 核心 | MVP | BIB-01～07 |
| 分镜镜头表（镜头语言、台词、实体引用、版本） | 脚本节点 | 核心，且是一等业务对象 | MVP | SB-01～05 |
| 全能参考生视频（图 / 视频 / 音频参考） | `mixed2video` | 核心 | MVP | VID-04、SB-04 |
| 图生视频、文生图、图像编辑 | 有 | 核心 | MVP（编辑 V1） | IMG-01、VID-01、IMG-03 |
| 首尾帧 | `frames2video` | 增强 | V1 | VID-06 |
| 配音、音色、字幕 | 音频节点、语音模型、音色设计 | 核心 | MVP | AUD-01～03 |
| 智能剪辑（多片段合成、转场、字幕） | `VIDEO_CLIP` | 核心（MVP 为顺序合成） | MVP / 转场 V1 | EDT-01～03、DLV-01 |
| 声明式模型注册表与参数 schema | 有 | **顶层能力**，MVP 定型 | MVP | 新增，见 5.2 |
| 任务结果回写与可写字段划分 | 有 | **顶层能力** | MVP | 新增，见 5.2 |
| 通用画布（节点、连线、分组、生成节点） | 核心形态 | 与流水线并重 | V1 | CNV-01～06 |
| 风格 / 镜头素材节点（连线即套用预设） | `MATERIAL_STYLE` / `LENS` | 增强 | V1 | 新增，见 5.2 |
| 批量运行（整表出图 / 出视频） | `VIDEO_GROUP`、批量运行记录 | 核心 | MVP | GEN-03 |
| 对话式 Agent（一句话出片、自动编排） | LibTV Agent、100+ Skill | 与画布同步上线 | V1（0101 P20） | AGT-01、CNV-07 |
| 开放接口（会话、上传、结果查询） | OpenAPI + Skill | 顶层预留 | V2 | 新增 |
| 模板 / 工作流沉淀（项目快照） | 模板、精选画布 | 顶层预留 | V2 | 新增 |
| 逐帧拉片（风格 / 运镜 / 音频拆解） | `SHOT_BREAKDOWN` | 增强 | V2 | 新增 |
| 动作迁移 / 动作控制、数字人、超分、抠图、字幕擦除 | 独立模型 | 通过模型注册表接入，不单独开发 | 按需 | — |
| 3D 导演台、全景空间 | 3D-BOX | 不做 | — | — |
| 社区、挑战赛、积分会员 | 有 | 内部自用阶段不做 | — | — |
| 媒体合规状态（肖像、版权链） | 字段级 | 核心（合规） | MVP | BIB-07、CMP-02 |

### 5.2 需要补入 0201 的需求（建议）

| 建议编号 | 需求 | 优先级 | 理由 |
| --- | --- | --- | --- |
| GEN-09 | 模型注册表：每个模型登记能力、生成模式、参考上限、参数 schema；界面按 schema 渲染参数 | MVP | D6；新增模型只改配置 |
| GEN-10 | 生成结果只写入结果字段；人工配置字段不被任务覆盖 | MVP | D3；避免人与任务互相覆盖 |
| CNV-08 | 风格 / 镜头语言素材节点：连到生成节点即套用对应预设 | V1 | D5 |
| PRJ-05 | 项目快照与模板：从项目快照复制出新项目 | V2 | D11 |
| AGT-01 | 对话式 Agent：自然语言驱动与 UI 相同的命令 | V1（0101 P20） | D9 |
| API-01 | 开放接口：会话、上传、任务与结果查询 | V2 | S4 |
| SB-07 | 逐帧拉片：拆解参考视频的镜头、运镜、音频，生成参考镜头表 | V2 | 3.2 |

## 6. 对顶层架构的要求

MVP 可以只实现其中一部分，但以下抽象必须在 MVP 就定型，否则后续引入画布、Agent、开放接口、新模型时会返工：

1. **统一的“生成操作”（Operation）**：任何生成都表达为“能力 + 模式 + 模型 + 参数 + 带用途的输入 → 任务 → 版本化输出”。流水线、画布节点、批量运行、Agent、开放接口都只是创建 Operation 的不同入口。
2. **声明式模型注册表**：能力、模式、参考上限、参数 schema、价格、供应商适配器键；界面与报价校验都由它驱动。
3. **结构化生产对象为事实源，画布节点为视图或自由素材**：镜头、角色、造型等是一等业务对象；画布既能展示它们（如镜头表节点），也能容纳不属于任何镜头的自由生成节点。
4. **结果字段归任务、配置字段归人**：任务只写结果与状态；人只改配置与选定。
5. **统一命令层**：UI、Agent、开放接口调用同一套带权限、幂等、版本校验的命令。
6. **持久化的流程编排**：Operation 的调度、重试、等待、对账与人工确认由工作流引擎统一承担（按 0308 第 3 版确定为 Temporal），工作流只由一方（Go）编写，AI 执行作为独立的 Activity。
7. **合规与来源是媒体属性**：每个媒体资产带来源（上传 / 生成 / 哪个 Operation）、合规状态、授权记录。

这些要求在 [0301](../design/0301-系统架构设计.md) 中落地。

## 7. 已确认事项（2026-09-25）

| # | 问题 | 结论 |
| --- | --- | --- |
| R1 | 是否接受第 5.2 节补入 0201 的需求 | 全部接受（0101 P23） |
| R2 | 是否登录实测 LibTV | 由产品负责人在 P0 期间实测（画布交互、Agent 质量、全能参考效果与成本），结果补入本文 |
| R3 | Agent 的节奏 | 完整对话式 Agent 提前到 V1，与画布同步上线（0101 P20） |

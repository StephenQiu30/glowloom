# 创作对齐即梦：当前实现源码审计

- 日期：2026-10-29。
- 范围：第二阶段当前实现检查；对应 [70 项对齐矩阵](parity-matrix.csv)。第一阶段原始外部基线不因本次源码审查而改写。
- 快照：本次读取的工作区 HEAD 为 `6e86a650`，不是纯提交快照；已读取下列未提交改动。父任务报告第一阶段已由 `a92732f1` 提交推送，本报告不重复证明远端状态。
- 本审计只写本文件与同目录 `parity-matrix.csv`，没有修改实现、设计稿、导航或 BACKLOG。

## 证据边界

本报告的“已具备源码”只表示所读代码中存在对应结构或分支，不表示测试通过、真实页面可用、真实供应商支持或结算正确。“缺失/未见”限于已检查的创作主链路与列出的合同，不能作为全仓库绝无相关代码的证明。现有测试是否充分、是否通过，本次均未验证；没有执行单测、构建、浏览器、真实上传/生成/下载/发布或扣费操作。

功能、交互和命名分别记录；视觉样式全部保留为未实测，Figma 节点未读取、未填造。供应商目录与适配器的能力枚举不能代替真实账户和真实媒体验收。竞品资料中标为未知的上传后行为、草稿跨设备、排队运行、计费和退款合同，仍是未知，不由本项目代码反向推定即梦行为。

已读取的未提交文件包括：

- `frontend/src/components/generation/editor-capabilities.ts`
- `frontend/src/components/generation/generation-dialog.tsx`
- `frontend/src/components/generation/prompt-document.ts`
- `frontend/src/components/generation/reference-controls.tsx`
- `frontend/src/components/generation/reference-mention-menu.tsx`
- `frontend/src/components/generation/reference-mode.ts`
- `frontend/src/components/generation/rich-generation.tsx`
- `frontend/src/components/layout/product-header-actions.tsx`
- `frontend/src/components/layout/product-account-actions.tsx`（父任务并行新增，已重新读取）
- `frontend/src/app/assets/_components/personal-history-list.tsx`

同一工作区还有其他并行修改，本报告不归属、不覆盖它们。公共 Header 由父任务并行调整，复核时已读到新增 ProductAccountActions，CR-005/061 已按新源码更新；此调整尚未由本审计运行验收。相关文件继续变化后须重新定位行号并复核。

## 核心结论

七类创作名称已经出现在首页菜单，但实际进入生成工作区的只有图片和视频；Agent、音乐、音频、数字人、动作模仿打开未开放提示。首页默认图片，且同一 prompt/homeDraft 未按创作类型隔离。项目内图片/视频草稿已有项目与类型作用域、服务端 revision 保存、冻结报价与确认幂等机制，这些基础应继续复用。

引用模型已有媒体、主体和画布主体、角色与版本、上传中替代、失效验证、撤销和删除引用的边界。结构化输入节点目前没有技能类型，视频也没有完整五模式的独立输入结构。参数 schema 声明了 slider/voice，但渲染器主要落为 Select、Checkbox、Input；图片宽高联动、音乐时长、音色库等尚不能由这个通用表单自动获得。

生成历史已经有任务与结果关联、按媒体类型组织、预览、复用、下载和显式发布入口，以及未知结果核对/取消基础。但排队和运行的独立供应商状态、多输出不足/部分失败、最终计费和退还仍须专项闭环。用户可见内容混有内部 mode、capability、JSON 参数、编译版本和 revision，另有“积分”与 CNY 金额冲突，命名对齐必须同时覆盖输入、结果与费用。

普通页面原有 ProductHeader/AccountMenu 与工具栏插槽；初读时 assetLayout 导致账号组合不同，父任务并行修改后已提取 ProductAccountActions 统一组合，本审计已读回新源码。不能据此直接断言所有页面像素样式已一致。

## 可独立验收的变更边界

以下是依赖与验收拆分建议，不是排期、承诺或完成状态；实施跟踪仍归 BACKLOG。

| 变更边界 | 对应 CR | 可执行内容 | 需要的验收 |
| --- | --- | --- | --- |
| 公共账号与页面工具槽 | 005、061–063 | 在现有 Header 中固定账号组合；页面筛选与批量动作通过插槽注册；退出批量恢复 | 资产、创作、探索、主体、普通管理路由；登录/未登录；批量进退与回焦；长短视口 |
| 首页类型与草稿身份 | 009、014、016–017、027–028 | 类型作用域草稿；真实可用性与有效内容共同决定提交；类型文案/占位与正文分离 | Agent/图片往返；刷新、跨路由、账号/空间切换；引用和参数完整性；不支持能力不可假成功 |
| 图片/视频真实能力驱动参数 | 010–013、015、029–037 | 按模型 schema 渲染实际控件和限制；模式名称映射；首尾帧槽；秒/毫秒换算与数量 | 模型切换合法组合；参数保存与报价快照；引用迁移；实际供应商请求/结果 |
| 富文本与引用闭环 | 018、021–026、034–035 | 复用现有 structured draft；补槽位和错误/失效状态；保持只删除引用 | IME/剪贴板/撤销；上传失败和未知重试；替换与失效权限；服务端读回与任务映射 |
| 结果命名与状态呈现 | 054–060 | 用户可读参数；重试与再次创作区分；任务/输出关联；不足与部分失败 | 同任务多输出；双击/断网不重复确认；下载/发布回执；实际结算/退还 |
| 新类型与技能能力 | 003、006、018–020、038–053 | Agent/技能、音乐、音色、数字人、动作模仿各自建合同与真实能力；不以菜单完成代替业务 | 每类素材、授权、模型、参数、成本、输出、失败/恢复完整链路 |
| 全站视觉与适配 | 001–008、064–069 | 以认可头像账号样式及即梦基线建立 tokens/变体/页面映射 | Figma 节点读回；真实实例八态；200%/短视口/390移动/键盘焦点；管理合同不退化 |

其中前五组可利用现有能力形成较小闭环；新类型/技能不是单纯改文字与增加控件即可交付。任何供应商尚无协议支持的参数、时长或模式，应保持明确不可用并记录依赖。

## 70 项逐项证据

统计：59 项源码检查、10 项有限源码检查、1 项流程记录。70 项均无本轮运行验收。每项的实现功能、交互状态、视觉、命名、差异决定与精确路径同时保存在 CSV；以下以可读形式逐项展开。

### CR-001 · 主导航命名与层级一致，创作具有明确选中态

- 检查范围：源码已检查。功能：部分具备。
- 源码观察：四个主入口有 aria-current 选中态。命名：创作/探索/资产/主体；未见短剧工作室主入口。
- 证据：[frontend/src/components/layout/product-sider.tsx:44](../../../../frontend/src/components/layout/product-sider.tsx#L44)；[frontend/src/components/layout/product-sider.tsx:66](../../../../frontend/src/components/layout/product-sider.tsx#L66)。
- 差异与修正建议：按导航基线确认短剧工作室入口及层级；不要以现有四项视为完整一致。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-002 · 生成/画布为独立工作方式切换，不能丢失分支含义

- 检查范围：源码已检查。功能：已具备源码。
- 源码观察：generate/canvas 独立切换。命名：生成/画布一致。
- 证据：[frontend/src/app/_components/home-creation.tsx:114](../../../../frontend/src/app/_components/home-creation.tsx#L114)；[frontend/src/app/_components/home-creation.tsx:245](../../../../frontend/src/app/_components/home-creation.tsx#L245)；[frontend/src/app/_components/home-creation.tsx:442](../../../../frontend/src/app/_components/home-creation.tsx#L442)。
- 差异与修正建议：保留两种工作方式，分支草稿与真正提交行为另验。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-003 · 对话与画布历史分区，各有创建入口；空态可区分

- 检查范围：源码已检查。功能：部分具备。
- 源码观察：画布有历史；对话显示待开放。命名：对话区入口写新建创作，与新建对话语义不同。
- 证据：[frontend/src/components/layout/product-sider.tsx:153](../../../../frontend/src/components/layout/product-sider.tsx#L153)；[frontend/src/components/layout/product-sider.tsx:189](../../../../frontend/src/components/layout/product-sider.tsx#L189)。
- 差异与修正建议：建立独立对话实体与恢复合同后接入；不能把首页跳转当创建会话。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-004 · 画布分支显示最近项目、全部、新建画布和发送输入

- 检查范围：源码已检查。功能：部分具备。
- 源码观察：RecentCanvases/NewCanvasButton 存在；文字发送只开不可用弹窗。命名：画布入口基本可识别。
- 证据：[frontend/src/app/_components/home-creation.tsx:442](../../../../frontend/src/app/_components/home-creation.tsx#L442)；[frontend/src/app/_components/home-creation.tsx:464](../../../../frontend/src/app/_components/home-creation.tsx#L464)；[frontend/src/app/_components/home-creation.tsx:499](../../../../frontend/src/app/_components/home-creation.tsx#L499)。
- 差异与修正建议：最近画布链路可专项验收；从提示词创建画布仍须业务实现。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-005 · 公共账号栏与页面业务工具栏职责分离，跨普通路由复用

- 检查范围：源码已检查。功能：已具备共享结构，未运行。
- 源码观察：并行修改后已抽出 ProductAccountActions；assetLayout 只分支页面工具，账号组独立。命名：通知与账号统一；资产工具保留业务名称。
- 证据：[frontend/src/components/layout/product-header-actions.tsx:18](../../../../frontend/src/components/layout/product-header-actions.tsx#L18)；[frontend/src/components/layout/product-header-actions.tsx:47](../../../../frontend/src/components/layout/product-header-actions.tsx#L47)；[frontend/src/components/layout/product-account-actions.tsx:24](../../../../frontend/src/components/layout/product-account-actions.tsx#L24)；[frontend/src/components/layout/product-header.tsx:36](../../../../frontend/src/components/layout/product-header.tsx#L36)。
- 差异与修正建议：复核本次新增公共账号组件在各普通路由的实际组合与视觉；保留页面工具插槽和批量替换语义，不以源码抽出判定完成。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-006 · 首页包含推荐技能、最近上新、工具和作品内容，各自有边界

- 检查范围：源码已检查。功能：缺失主要区域。
- 源码观察：首页仅创作输入与 InspirationGallery。命名：创作精选不能覆盖技能/上新/工具。
- 证据：[frontend/src/app/_components/home-page.tsx:110](../../../../frontend/src/app/_components/home-page.tsx#L110)；[frontend/src/app/_components/home-page.tsx:132](../../../../frontend/src/app/_components/home-page.tsx#L132)；[frontend/src/components/inspiration/gallery.tsx:148](../../../../frontend/src/components/inspiration/gallery.tsx#L148)。
- 差异与修正建议：分别定义内容源与加载/空/错状态，再落推荐技能、最近上新、工具区域。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-007 · 首页内容频道为精选/短剧漫剧/影视作品/动画动漫/广告营销/平面设计/电商，不能直接套探索分类

- 检查范围：源码已检查。功能：有明确差异。
- 源码观察：静态频道筛选与分页。命名：六项频道缺短剧漫剧，顺序不同。
- 证据：[frontend/src/components/inspiration/gallery.tsx:31](../../../../frontend/src/components/inspiration/gallery.tsx#L31)；[frontend/src/components/inspiration/gallery.tsx:130](../../../../frontend/src/components/inspiration/gallery.tsx#L130)。
- 差异与修正建议：用独立首页频道合同按基线排序；确认内容归类，避免只添空标签。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-008 · 内容卡片的查看创作过程与创作操作分开定义

- 检查范围：源码有限检查。功能：待专项确认。
- 源码观察：主页卡片打开 InspirationPreview；本次未深入预览内部动作。命名：过程/创作操作尚未完整核验。
- 证据：[frontend/src/components/inspiration/gallery.tsx:203](../../../../frontend/src/components/inspiration/gallery.tsx#L203)。
- 差异与修正建议：补查预览详情路由与动作权限，分别验收查看创作过程和发起创作。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-009 · 创作类型菜单完整表达七类，有当前选中标记

- 检查范围：源码已检查。功能：菜单具备，功能仅两类。
- 源码观察：七项选中控件；五类走 UnavailableDialog。命名：七类名称一致但追加暂未开放；默认图片。
- 证据：[frontend/src/app/_components/home-creation.tsx:60](../../../../frontend/src/app/_components/home-creation.tsx#L60)；[frontend/src/app/_components/home-creation.tsx:115](../../../../frontend/src/app/_components/home-creation.tsx#L115)；[frontend/src/app/_components/home-creation.tsx:138](../../../../frontend/src/app/_components/home-creation.tsx#L138)；[frontend/src/app/_components/home-creation.tsx:214](../../../../frontend/src/app/_components/home-creation.tsx#L214)；[frontend/src/app/_components/home-creation.tsx:176](../../../../frontend/src/app/_components/home-creation.tsx#L176)。
- 差异与修正建议：记录为七个入口而非七类能力；按真实能力分阶段交付，默认类型按产品决定。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-010 · 模型集合随类型变化，名称、能力说明、标记、选择态分层

- 检查范围：源码已检查。功能：部分具备。
- 源码观察：图片/视频按 capability 拉目录，可搜索与选中。命名：名称+供应商+当前不可用，缺能力说明和细分标记。
- 证据：[frontend/src/components/generation/rich-generation.tsx:252](../../../../frontend/src/components/generation/rich-generation.tsx#L252)；[frontend/src/components/generation/model-picker.tsx:92](../../../../frontend/src/components/generation/model-picker.tsx#L92)；[frontend/src/components/generation/model-picker.tsx:100](../../../../frontend/src/components/generation/model-picker.tsx#L100)；[frontend/src/components/generation/model-picker.tsx:121](../../../../frontend/src/components/generation/model-picker.tsx#L121)。
- 差异与修正建议：扩目录展示合同并用真实状态解释不可用；五类尚无实际首页模型选择。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-011 · 模型能力决定参数集、引用角色/数量和可用模式

- 检查范围：源码已检查。功能：已具备主要基础。
- 源码观察：参数 schema、reference_roles 与 output_limits 校验；渲染未消费全部 component。命名：技术字段与产品称谓仍需统一。
- 证据：[frontend/src/components/generation/contracts.ts:95](../../../../frontend/src/components/generation/contracts.ts#L95)；[frontend/src/components/generation/editor-capabilities.ts:34](../../../../frontend/src/components/generation/editor-capabilities.ts#L34)；[frontend/src/components/generation/editor-capabilities.ts:73](../../../../frontend/src/components/generation/editor-capabilities.ts#L73)；[frontend/src/components/generation/generation-parameters.tsx:43](../../../../frontend/src/components/generation/generation-parameters.tsx#L43)；[frontend/src/components/generation/reference-mode.ts:57](../../../../frontend/src/components/generation/reference-mode.ts#L57)。
- 差异与修正建议：保留数据驱动验证；补 slider/voice 等渲染器及每模式输入结构，不硬编码虚假支持。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-012 · 权益受限展示与真实可用性一致；不能仅绘制VIP标签

- 检查范围：源码已检查。功能：部分具备。
- 源码观察：按 model/provider active 与 credential_present 禁用。命名：当前不可用未解释具体原因。
- 证据：[frontend/src/components/generation/model-picker.tsx:100](../../../../frontend/src/components/generation/model-picker.tsx#L100)；[frontend/src/components/generation/editor-capabilities.ts:8](../../../../frontend/src/components/generation/editor-capabilities.ts#L8)。
- 差异与修正建议：将权限/模型/服务/凭证不可用区分；没有即梦VIP权益等价合同，不能加标签冒充权益。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-013 · 模型加载中、无模型、失败、无权限需要独立可恢复状态

- 检查范围：源码已检查。功能：部分具备。
- 源码观察：目录失败可重试且保留输入；空目录/搜索无匹配复用通用空态。命名：暂无匹配的可用模型混合多种情况。
- 证据：[frontend/src/components/generation/rich-generation.tsx:488](../../../../frontend/src/components/generation/rich-generation.tsx#L488)；[frontend/src/components/generation/rich-generation.tsx:2020](../../../../frontend/src/components/generation/rich-generation.tsx#L2020)；[frontend/src/components/generation/model-picker.tsx:70](../../../../frontend/src/components/generation/model-picker.tsx#L70)；[frontend/src/components/generation/model-picker.tsx:95](../../../../frontend/src/components/generation/model-picker.tsx#L95)。
- 差异与修正建议：分离加载、零模型、搜索无匹配、权限错误与服务失败，给出对应可恢复动作。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-014 · Agent与图片草稿按类型隔离，本页往返恢复各自内容

- 检查范围：源码已检查。功能：有明确差异。
- 源码观察：首页一个 prompt/homeDraft；项目图片/视频草稿按 project/type 保存。命名：Agent 仍不可用。
- 证据：[frontend/src/app/_components/home-page.tsx:18](../../../../frontend/src/app/_components/home-page.tsx#L18)；[frontend/src/app/_components/home-creation.tsx:125](../../../../frontend/src/app/_components/home-creation.tsx#L125)；[frontend/src/app/_components/home-creation.tsx:127](../../../../frontend/src/app/_components/home-creation.tsx#L127)；[frontend/src/app/_components/home-creation.tsx:165](../../../../frontend/src/app/_components/home-creation.tsx#L165)；[frontend/src/app/_components/home-creation.tsx:338](../../../../frontend/src/app/_components/home-creation.tsx#L338)。
- 差异与修正建议：首页草稿身份加入创作类型；恢复正文、引用、模型、参数，不能只改变组件 key。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-015 · 视频模式切换自动匹配模型及参数，并给出提示；已有不兼容素材的保留、清空或确认须补证

- 检查范围：源码已检查。功能：部分具备。
- 源码观察：模式切换可选模型与转换参数；紧凑表单自动按引用推断模式。命名：完整表单裸露内部 mode 字符串。
- 证据：[frontend/src/components/generation/rich-generation.tsx:1005](../../../../frontend/src/components/generation/rich-generation.tsx#L1005)；[frontend/src/components/generation/rich-generation.tsx:1960](../../../../frontend/src/components/generation/rich-generation.tsx#L1960)；[frontend/src/components/generation/rich-generation.tsx:2107](../../../../frontend/src/components/generation/rich-generation.tsx#L2107)；[frontend/src/components/generation/reference-mode.ts:5](../../../../frontend/src/components/generation/reference-mode.ts#L5)；[frontend/src/components/generation/reference-mode.ts:57](../../../../frontend/src/components/generation/reference-mode.ts#L57)。
- 差异与修正建议：区分用户选视频模式与推断引用模式；补五模式及迁移确认，保留竞品素材迁移未知标注。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-016 · 页面刷新、跨路由、跨设备草稿恢复独立验收

- 检查范围：源码已检查。功能：部分具备。
- 源码观察：首页 branchJournal 仅恢复文字；项目草稿有服务端 revision/save。命名：首页明确提示参考素材与生成设置需重新选择。
- 证据：[frontend/src/app/_components/home-page.tsx:43](../../../../frontend/src/app/_components/home-page.tsx#L43)；[frontend/src/app/_components/home-page.tsx:65](../../../../frontend/src/app/_components/home-page.tsx#L65)；[frontend/src/app/_components/home-page.tsx:118](../../../../frontend/src/app/_components/home-page.tsx#L118)；[frontend/src/components/generation/rich-generation.tsx:579](../../../../frontend/src/components/generation/rich-generation.tsx#L579)；[frontend/src/components/generation/rich-generation.tsx:664](../../../../frontend/src/components/generation/rich-generation.tsx#L664)；[frontend/src/components/generation/rich-generation.tsx:680](../../../../frontend/src/components/generation/rich-generation.tsx#L680)。
- 差异与修正建议：分别验同页往返/刷新/跨路由/跨设备；当前恢复机制不能证明跨设备完整草稿。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-017 · 输入提示文案随类型/模式变化，空态与真正正文区分

- 检查范围：源码已检查。功能：部分具备。
- 源码观察：首页各类 placeholder；项目紧凑输入统一通用提示。命名：动作模仿仍提示描述动作，与禁用正文不一致。
- 证据：[frontend/src/app/_components/home-creation.tsx:60](../../../../frontend/src/app/_components/home-creation.tsx#L60)；[frontend/src/app/_components/home-creation.tsx:88](../../../../frontend/src/app/_components/home-creation.tsx#L88)；[frontend/src/components/generation/rich-generation.tsx:1174](../../../../frontend/src/components/generation/rich-generation.tsx#L1174)。
- 差异与修正建议：占位文案按类型/模式配置，保留 placeholder 与用户正文区别；动作模仿单独处理。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-018 · 富文本支持技能标签、@主体/参考及普通正文组合

- 检查范围：源码已检查。功能：部分具备。
- 源码观察：结构化正文+hardBreak+reference；@引用存在。命名：无 skill 类型标签。
- 证据：[frontend/src/components/generation/prompt-document.ts:8](../../../../frontend/src/components/generation/prompt-document.ts#L8)；[frontend/src/components/generation/prompt-document.ts:16](../../../../frontend/src/components/generation/prompt-document.ts#L16)；[frontend/src/components/generation/composer-session.ts:123](../../../../frontend/src/components/generation/composer-session.ts#L123)。
- 差异与修正建议：扩结构化技能节点及序列化/删除/复制语义；不能用彩色普通文字替代。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-019 · 技能标签为结构化对象，不仅是视觉上加颜色的字符串

- 检查范围：源码已检查。功能：缺失。
- 源码观察：PromptInline 联合仅 text/hardBreak/reference。命名：缺技能对象合同。
- 证据：[frontend/src/components/generation/prompt-document.ts:8](../../../../frontend/src/components/generation/prompt-document.ts#L8)；[frontend/src/components/generation/prompt-document.ts:158](../../../../frontend/src/components/generation/prompt-document.ts#L158)。
- 差异与修正建议：先定技能标识、版本、显示名与参数快照合同，再扩编辑器和后端编译。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-020 · 技能菜单含搜索、说明、官方标记、更多/新建/管理入口

- 检查范围：源码已检查。功能：未见对应实现。
- 源码观察：已查创作输入链路无技能菜单。命名：缺搜索/官方/更多/新建/管理的技能语义。
- 证据：[frontend/src/app/_components/home-creation.tsx:338](../../../../frontend/src/app/_components/home-creation.tsx#L338)；[frontend/src/components/generation/prompt-document.ts:8](../../../../frontend/src/components/generation/prompt-document.ts#L8)；[frontend/src/components/generation/reference-mention-menu.tsx:151](../../../../frontend/src/components/generation/reference-mention-menu.tsx#L151)。
- 差异与修正建议：建立技能目录与生命周期；主体@菜单不能直接视为技能菜单。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-021 · 输入@出现可能@的内容及创建主体入口

- 检查范围：源码已检查。功能：已具备源码。
- 源码观察：@触发候选；创建主体、加载、错误状态存在。命名：可能@的内容/创建主体接近基线。
- 证据：[frontend/src/components/generation/composer-session.ts:123](../../../../frontend/src/components/generation/composer-session.ts#L123)；[frontend/src/components/generation/composer-session.ts:139](../../../../frontend/src/components/generation/composer-session.ts#L139)；[frontend/src/components/generation/reference-mention-menu.tsx:151](../../../../frontend/src/components/generation/reference-mention-menu.tsx#L151)。
- 差异与修正建议：验证鼠标与键盘插入、失焦关闭、创建后回填；不可只凭菜单源码判验收。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-022 · 参考槽按角色区分并有移除动作，删除引用与删除源资产区分

- 检查范围：源码已检查。功能：部分具备。
- 源码观察：reference 有 role；删除确认只删引用与匹配标签，不删源资产。命名：普通列表与命名角色槽尚不等价。
- 证据：[frontend/src/components/generation/prompt-document.ts:36](../../../../frontend/src/components/generation/prompt-document.ts#L36)；[frontend/src/components/generation/reference-controls.tsx:758](../../../../frontend/src/components/generation/reference-controls.tsx#L758)；[frontend/src/components/generation/reference-controls.tsx:1871](../../../../frontend/src/components/generation/reference-controls.tsx#L1871)。
- 差异与修正建议：保留引用删除边界；按首尾帧/人物/动作等定义专用槽位视觉与空态。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-023 · 本地文件选择、上传、处理中、失败重试、类型/大小/数量校验单列

- 检查范围：源码已检查。功能：已具备主要基础。
- 源码观察：类型校验、pending upload、审核确认、原请求重试、同类型替换。命名：上传/重试原请求有技术合同含义。
- 证据：[frontend/src/components/generation/reference-controls.tsx:566](../../../../frontend/src/components/generation/reference-controls.tsx#L566)；[frontend/src/components/generation/reference-controls.tsx:656](../../../../frontend/src/components/generation/reference-controls.tsx#L656)；[frontend/src/components/generation/reference-controls.tsx:943](../../../../frontend/src/components/generation/reference-controls.tsx#L943)；[frontend/src/components/generation/reference-controls.tsx:1004](../../../../frontend/src/components/generation/reference-controls.tsx#L1004)；[frontend/src/components/generation/editor-capabilities.ts:34](../../../../frontend/src/components/generation/editor-capabilities.ts#L34)。
- 差异与修正建议：专项验证大小/数量/格式矩阵、处理中取消、未知结果重试；未执行真实上传。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-024 · 资产/主体/画布选择器与创作输入建立可追踪引用

- 检查范围：源码已检查。功能：已具备主要基础。
- 源码观察：source 联合含 media/subject/canvas_subject，版本和映射可追溯。命名：三类来源可辨识。
- 证据：[frontend/src/components/generation/prompt-document.ts:16](../../../../frontend/src/components/generation/prompt-document.ts#L16)；[frontend/src/components/generation/reference-controls.tsx:320](../../../../frontend/src/components/generation/reference-controls.tsx#L320)；[frontend/src/components/generation/rich-generation.tsx:594](../../../../frontend/src/components/generation/rich-generation.tsx#L594)。
- 差异与修正建议：验选择→插入→保存→读回→任务冻结映射；源码存在不证明权限与真实资料全链路。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-025 · 引用删除、重复、替换、失效权限和正文悬空引用有确定行为

- 检查范围：源码已检查。功能：部分具备。
- 源码观察：替换类型检查、失效验证、移除内联引用联动；重复细则需专项。命名：错误名称需按场景统一。
- 证据：[frontend/src/components/generation/reference-controls.tsx:566](../../../../frontend/src/components/generation/reference-controls.tsx#L566)；[frontend/src/components/generation/reference-controls.tsx:656](../../../../frontend/src/components/generation/reference-controls.tsx#L656)；[frontend/src/components/generation/reference-controls.tsx:1871](../../../../frontend/src/components/generation/reference-controls.tsx#L1871)；[frontend/src/components/generation/rich-generation.tsx:594](../../../../frontend/src/components/generation/rich-generation.tsx#L594)。
- 差异与修正建议：补重复、并发过期、撤销移除、悬空标签契约测试与真实页面；不要擅自删源素材。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-026 · 复制粘贴、撤销重做、中文输入法、换行/提交快捷键和长文本需验收

- 检查范围：源码已检查。功能：已具备主要基础。
- 源码观察：undo/redo、composition、快捷键、剪贴板文件与文本分支。命名：未发现本项特定命名问题。
- 证据：[frontend/src/components/generation/composer-session.ts:272](../../../../frontend/src/components/generation/composer-session.ts#L272)；[frontend/src/components/generation/composer-session.ts:606](../../../../frontend/src/components/generation/composer-session.ts#L606)；[frontend/src/components/generation/composer-session.ts:657](../../../../frontend/src/components/generation/composer-session.ts#L657)；[frontend/src/components/generation/composer-session.ts:725](../../../../frontend/src/components/generation/composer-session.ts#L725)。
- 差异与修正建议：组合验中文IME、撤销恢复引用、粘贴多媒体、长文本与焦点；本轮未执行。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-027 · 空内容生成禁用，普通Agent/图片输入后按钮可用

- 检查范围：源码已检查。功能：部分具备且有差异。
- 源码观察：supported && !prompt.trim() 禁用仅覆盖图片/视频；不支持类型空输入仍可点。命名：Agent 按钮只打开未开放提示。
- 证据：[frontend/src/app/_components/home-creation.tsx:138](../../../../frontend/src/app/_components/home-creation.tsx#L138)；[frontend/src/app/_components/home-creation.tsx:417](../../../../frontend/src/app/_components/home-creation.tsx#L417)；[frontend/src/app/_components/home-creation.tsx:424](../../../../frontend/src/app/_components/home-creation.tsx#L424)；[frontend/src/components/generation/editor-capabilities.ts:110](../../../../frontend/src/components/generation/editor-capabilities.ts#L110)。
- 差异与修正建议：统一按能力+有效内容+必需槽位判断；Agent未实现时不能视为满足输入后可生成。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-028 · 多槽位类型不按“有提示词”判断可提交；动作模仿文本禁用

- 检查范围：源码已检查。功能：缺失类型合同。
- 源码观察：通用校验可验引用数量，但动作模仿输入仍可编辑。命名：动作模仿占位描述动作不符合基线。
- 证据：[frontend/src/app/_components/home-creation.tsx:88](../../../../frontend/src/app/_components/home-creation.tsx#L88)；[frontend/src/app/_components/home-creation.tsx:338](../../../../frontend/src/app/_components/home-creation.tsx#L338)；[frontend/src/components/generation/editor-capabilities.ts:34](../../../../frontend/src/components/generation/editor-capabilities.ts#L34)；[frontend/src/components/generation/editor-capabilities.ts:110](../../../../frontend/src/components/generation/editor-capabilities.ts#L110)。
- 差异与修正建议：新增每类型正文/必填槽/允许来源规则，动作模仿禁用正文并校验角色和动作。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-029 · 图片比例菜单支持智能和固定比例，当前选择可见

- 检查范围：源码已检查。功能：部分具备。
- 源码观察：schema enum 可渲染比例；图片候选当前 size 仅2K/4K。命名：画面比例标签存在，不证明图片智能比例支持。
- 证据：[frontend/src/components/generation/generation-parameters.tsx:43](../../../../frontend/src/components/generation/generation-parameters.tsx#L43)；[frontend/src/components/generation/generation-parameters.tsx:47](../../../../frontend/src/components/generation/generation-parameters.tsx#L47)；[backend/internal/catalog/domain/model_candidates.go:151](../../../../backend/internal/catalog/domain/model_candidates.go#L151)。
- 差异与修正建议：先确认真实图片协议可表达比例/尺寸，再实现智能与固定比例，避免只展示无效参数。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-030 · 图片分辨率与像素W/H、约束比例共同呈现

- 检查范围：源码已检查。功能：缺失组合交互。
- 源码观察：参数只 enum/boolean/Input，没有 W/H 联动与比例锁。命名：清晰度已有，像素宽高/约束比例未见。
- 证据：[frontend/src/components/generation/generation-parameters.tsx:43](../../../../frontend/src/components/generation/generation-parameters.tsx#L43)；[frontend/src/components/generation/generation-parameters.tsx:109](../../../../frontend/src/components/generation/generation-parameters.tsx#L109)；[backend/internal/catalog/domain/model_candidates.go:152](../../../../backend/internal/catalog/domain/model_candidates.go#L152)。
- 差异与修正建议：为图片尺寸建立合法组合与模型范围；新增像素宽高和锁定规则后验实际请求。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-031 · 图片生成数量支持1/2/3/4，触发器显示当前值

- 检查范围：源码已检查。功能：部分具备。
- 源码观察：数量由 max_outputs 裁到1..8，不固定1..4。命名：紧凑生成数量/完整候选数量不统一。
- 证据：[frontend/src/components/generation/generation-parameters.tsx:35](../../../../frontend/src/components/generation/generation-parameters.tsx#L35)；[frontend/src/components/generation/generation-parameters.tsx:114](../../../../frontend/src/components/generation/generation-parameters.tsx#L114)；[backend/internal/catalog/domain/model_candidates.go:137](../../../../backend/internal/catalog/domain/model_candidates.go#L137)。
- 差异与修正建议：按具体模型限制呈现；基线仅观察当前模型1..4，不能对所有供应商强制四张。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-032 · 图片费用按张显示，数量变化后的报价/最终扣费需补证

- 检查范围：源码已检查。功能：部分具备。
- 源码观察：报价展示预计费用/余额，未见按张单价呈现。命名：CNY金额与界面提交前确认积分不一致。
- 证据：[frontend/src/components/generation/rich-generation.tsx:1318](../../../../frontend/src/components/generation/rich-generation.tsx#L1318)；[frontend/src/components/generation/contracts.ts:172](../../../../frontend/src/components/generation/contracts.ts#L172)；[frontend/src/app/_components/home-creation.tsx:417](../../../../frontend/src/app/_components/home-creation.tsx#L417)。
- 差异与修正建议：统一计价单位文案；另验张数→报价→实际输出/结算，不能从前端总价推断按张扣费。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-033 · 视频五模式使用不同输入结构：多帧的顺序帧槽、编辑视频和参考双槽、超长视频参考；模型联动

- 检查范围：源码已检查。功能：缺失五模式结构。
- 源码观察：现有视频 mode 主要 text2video/image2video/omni_reference。命名：内部mode直显；无完整五模式名称映射。
- 证据：[backend/internal/catalog/domain/preset_model.go:66](../../../../backend/internal/catalog/domain/preset_model.go#L66)；[frontend/src/components/generation/reference-mode.ts:5](../../../../frontend/src/components/generation/reference-mode.ts#L5)；[frontend/src/components/generation/rich-generation.tsx:1960](../../../../frontend/src/components/generation/rich-generation.tsx#L1960)。
- 差异与修正建议：定义五种产品模式与真实供应商能力映射；多帧/编辑/超长须新增合同及输入结构。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-034 · 全能参考表达多媒体组合与@引用，限制由具体模型决定

- 检查范围：源码已检查。功能：部分具备。
- 源码观察：多媒体 reference + @；Seedance2.5 有图片/视频/音频限制。命名：compact 自动推断 omni_reference，产品展示需统一。
- 证据：[frontend/src/components/generation/prompt-document.ts:6](../../../../frontend/src/components/generation/prompt-document.ts#L6)；[frontend/src/components/generation/reference-mode.ts:5](../../../../frontend/src/components/generation/reference-mode.ts#L5)；[backend/internal/catalog/domain/volcengine_models.go:59](../../../../backend/internal/catalog/domain/volcengine_models.go#L59)；[frontend/src/components/generation/editor-capabilities.ts:34](../../../../frontend/src/components/generation/editor-capabilities.ts#L34)。
- 差异与修正建议：可优先闭环全能参考；按实际模型限制验证混合输入与请求，不等同所有模型支持。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-035 · 首尾帧分两个命名槽位，不与普通参考列表混用

- 检查范围：源码已检查。功能：部分具备。
- 源码观察：first_frame/last_frame 角色及交换存在；紧凑添加默认普通引用。命名：缺稳定首帧/尾帧双槽空态。
- 证据：[backend/internal/catalog/domain/model_candidates.go:158](../../../../backend/internal/catalog/domain/model_candidates.go#L158)；[frontend/src/components/generation/reference-controls.tsx:582](../../../../frontend/src/components/generation/reference-controls.tsx#L582)；[frontend/src/components/generation/reference-controls.tsx:758](../../../../frontend/src/components/generation/reference-controls.tsx#L758)。
- 差异与修正建议：建立两个命名槽位、替换/清空/交换合同；普通引用列表不能代替。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-036 · 视频比例、分辨率、数量和时长是独立参数

- 检查范围：源码已检查。功能：部分具备。
- 源码观察：视频schema含 ratio/resolution/duration，数量独立。命名：时长内部 duration_ms；mode 有裸值。
- 证据：[backend/internal/catalog/domain/volcengine_models.go:59](../../../../backend/internal/catalog/domain/volcengine_models.go#L59)；[frontend/src/components/generation/generation-parameters.tsx:43](../../../../frontend/src/components/generation/generation-parameters.tsx#L43)；[frontend/src/components/generation/generation-parameters.tsx:114](../../../../frontend/src/components/generation/generation-parameters.tsx#L114)。
- 差异与修正建议：独立显示合法组合与单位转换，校验保存/重开/报价快照一致。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-037 · 视频时长支持滑块与数值输入；超长样本显示最长3分钟、刻度至180；刻度不等于可接受最小值

- 检查范围：源码已检查。功能：有明确差异。
- 源码观察：ParamField 允许 slider，但渲染数字 Input；现有2.5时长至30秒。命名：需秒与内部毫秒明确转换。
- 证据：[frontend/src/components/generation/contracts.ts:95](../../../../frontend/src/components/generation/contracts.ts#L95)；[frontend/src/components/generation/generation-parameters.tsx:43](../../../../frontend/src/components/generation/generation-parameters.tsx#L43)；[backend/internal/catalog/domain/volcengine_models.go:59](../../../../backend/internal/catalog/domain/volcengine_models.go#L59)。
- 差异与修正建议：补真正滑块+数值联动；180秒不能绕过模型30秒上限，超长能力另立合同。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-038 · 样片生成与高分辨率转换若采用，应定义两阶段状态与成本

- 检查范围：源码有限检查。功能：未见对应合同。
- 源码观察：已查普通生成任务链路无样片→高分辨率转换阶段。命名：不能将 succeeded/ingesting 当样片升级。
- 证据：[frontend/src/components/generation/contracts.ts:3](../../../../frontend/src/components/generation/contracts.ts#L3)；[backend/internal/catalog/domain/volcengine_models.go:59](../../../../backend/internal/catalog/domain/volcengine_models.go#L59)。
- 差异与修正建议：先确认供应商两阶段能力、输入关联、费用和失败恢复，再加入任务模型。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-039 · 音乐和音频独立类型、独立输入与参数，不合并成一个音频框

- 检查范围：源码已检查。功能：菜单区分但均未开放。
- 源码观察：music/audio 均不进入生成 workspace。命名：音乐生成/音频生成已有。
- 证据：[frontend/src/app/_components/home-creation.tsx:60](../../../../frontend/src/app/_components/home-creation.tsx#L60)；[frontend/src/app/_components/home-creation.tsx:138](../../../../frontend/src/app/_components/home-creation.tsx#L138)；[frontend/src/app/_components/home-creation.tsx:176](../../../../frontend/src/app/_components/home-creation.tsx#L176)；[backend/internal/operation/adapter/providerhttp/speech.go:24](../../../../backend/internal/operation/adapter/providerhttp/speech.go#L24)。
- 差异与修正建议：音频后端 text2audio 不等于音乐能力；分别设计和接入，禁止共用普通音频框冒充。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-040 · 音乐智能时长可切换，显式时长有滑块和数值输入

- 检查范围：源码已检查。功能：缺失。
- 源码观察：music 只有 placeholder 与未开放弹窗。命名：无智能时长/显式时长。
- 证据：[frontend/src/app/_components/home-creation.tsx:75](../../../../frontend/src/app/_components/home-creation.tsx#L75)；[frontend/src/app/_components/home-creation.tsx:138](../../../../frontend/src/app/_components/home-creation.tsx#L138)；[frontend/src/app/_components/home-creation.tsx:176](../../../../frontend/src/app/_components/home-creation.tsx#L176)。
- 差异与修正建议：先建立音乐能力和时长schema，再实现智能开关、滑块数值和范围校验。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-041 · 音乐示例提示与已输入文本区分；Tab接受建议须补测

- 检查范围：源码已检查。功能：缺失音乐建议合同。
- 源码观察：普通 placeholder 存在；未见音乐建议接受机制。命名：示例不能预填为用户正文。
- 证据：[frontend/src/app/_components/home-creation.tsx:75](../../../../frontend/src/app/_components/home-creation.tsx#L75)；[frontend/src/app/_components/home-prompt.tsx:20](../../../../frontend/src/app/_components/home-prompt.tsx#L20)；[frontend/src/components/generation/composer-session.ts:657](../../../../frontend/src/components/generation/composer-session.ts#L657)。
- 差异与修正建议：补音乐示例展示/接受行为；Tab行为竞品仍待证，不擅自设成已知要求。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-042 · 音频区分全能配音与文本朗读

- 检查范围：源码已检查。功能：缺失产品模式。
- 源码观察：音频首页不可用；后端目前 speech text2audio。命名：豆包语音合成2.0不能等同全能配音/文本朗读完整双模式。
- 证据：[frontend/src/app/_components/home-creation.tsx:138](../../../../frontend/src/app/_components/home-creation.tsx#L138)；[backend/internal/catalog/domain/volcengine_models.go:55](../../../../backend/internal/catalog/domain/volcengine_models.go#L55)；[backend/internal/operation/adapter/providerhttp/speech.go:24](../../../../backend/internal/operation/adapter/providerhttp/speech.go#L24)。
- 差异与修正建议：文本朗读可按真实能力先闭环；全能配音需独立能力合同与多媒体输入支持。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-043 · 音色库包含全部/我的/收藏与四类属性筛选

- 检查范围：源码已检查。功能：缺失。
- 源码观察：未见音色库；speaker 是文本参数。命名：无全部/我的/收藏与四类筛选。
- 证据：[backend/internal/catalog/domain/model_candidates.go:153](../../../../backend/internal/catalog/domain/model_candidates.go#L153)；[frontend/src/components/generation/generation-parameters.tsx:43](../../../../frontend/src/components/generation/generation-parameters.tsx#L43)；[backend/internal/operation/adapter/providerhttp/speech.go:24](../../../../backend/internal/operation/adapter/providerhttp/speech.go#L24)。
- 差异与修正建议：建立真实音色目录、属性/归属/收藏与筛选；文本speaker输入不能当音色选择器。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-044 · 选择音色加入可移除参考，切换类型返回仍保留

- 检查范围：源码已检查。功能：缺失音色引用合同。
- 源码观察：draft reference 仅媒体/主体；首页未按类型隔离。命名：voice 参数/引用未分层。
- 证据：[frontend/src/components/generation/prompt-document.ts:16](../../../../frontend/src/components/generation/prompt-document.ts#L16)；[frontend/src/app/_components/home-creation.tsx:127](../../../../frontend/src/app/_components/home-creation.tsx#L127)；[backend/internal/catalog/domain/model_candidates.go:153](../../../../backend/internal/catalog/domain/model_candidates.go#L153)。
- 差异与修正建议：建立音色引用结构、移除和跨类型恢复，先解决草稿作用域。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-045 · 多音色、时间戳、试听、音色创建及引用许可分别验收

- 检查范围：源码已检查。功能：缺失主要能力。
- 源码观察：当前speech adapter count1且不接参考素材；未见多音色/时间戳等闭环。命名：音色操作名称尚待定义。
- 证据：[backend/internal/operation/adapter/providerhttp/speech.go:24](../../../../backend/internal/operation/adapter/providerhttp/speech.go#L24)；[backend/internal/catalog/domain/model_candidates.go:153](../../../../backend/internal/catalog/domain/model_candidates.go#L153)。
- 差异与修正建议：试听、创建、许可、多音色、时间戳分别定义能力与授权，不从单说话人TTS推断支持。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-046 · 数字人提供角色与音色、说话内容和可选动作描述

- 检查范围：源码已检查。功能：缺失。
- 源码观察：数字人仅菜单/placeholder，未接角色和音色组合。命名：数字人入口名存在。
- 证据：[frontend/src/app/_components/home-creation.tsx:83](../../../../frontend/src/app/_components/home-creation.tsx#L83)；[frontend/src/app/_components/home-creation.tsx:138](../../../../frontend/src/app/_components/home-creation.tsx#L138)；[frontend/src/app/_components/home-creation.tsx:176](../../../../frontend/src/app/_components/home-creation.tsx#L176)。
- 差异与修正建议：新建数字人输入合同：角色、音色、讲话内容、动作；不直接套图片/视频表单。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-047 · 数字人支持上传音频入口与三类模式，不与普通视频表单等同

- 检查范围：源码已检查。功能：缺失。
- 源码观察：未见数字人上传音频与三模式可运行链路。命名：三模式命名未落实。
- 证据：[frontend/src/app/_components/home-creation.tsx:83](../../../../frontend/src/app/_components/home-creation.tsx#L83)；[frontend/src/app/_components/home-creation.tsx:94](../../../../frontend/src/app/_components/home-creation.tsx#L94)；[frontend/src/app/_components/home-creation.tsx:138](../../../../frontend/src/app/_components/home-creation.tsx#L138)。
- 差异与修正建议：明确音频驱动与文本驱动的互斥/迁移、上传限制、供应商支持后实现。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-048 · 动作模仿提供角色/动作槽位及三类模型，提示词禁用

- 检查范围：源码已检查。功能：缺失且有反向行为。
- 源码观察：动作模仿仅可编辑正文占位，未见角色/动作双槽与三模型。命名：入口名称有，内部结构不符。
- 证据：[frontend/src/app/_components/home-creation.tsx:88](../../../../frontend/src/app/_components/home-creation.tsx#L88)；[frontend/src/app/_components/home-creation.tsx:138](../../../../frontend/src/app/_components/home-creation.tsx#L138)；[frontend/src/app/_components/home-creation.tsx:338](../../../../frontend/src/app/_components/home-creation.tsx#L338)。
- 差异与修正建议：禁用正文，定义两个必填槽与真实动作模型目录，模型无法接入时保持诚实不可用。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-049 · 人物素材授权、格式和不满足条件时的反馈需单独定义

- 检查范围：源码有限检查。功能：待专门合同。
- 源码观察：通用素材上传有审核确认；不能证明人物授权全部规则。命名：授权与一般上传确认要分清。
- 证据：[frontend/src/components/generation/reference-controls.tsx:656](../../../../frontend/src/components/generation/reference-controls.tsx#L656)；[frontend/src/components/generation/editor-capabilities.ts:34](../../../../frontend/src/components/generation/editor-capabilities.ts#L34)。
- 差异与修正建议：定义人物素材许可来源、撤回、格式/质量限制及失败文案，另做用户素材验收。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-050 · Agent自动偏好与自定义明确区分，可选输出类型与比例

- 检查范围：源码已检查。功能：缺失。
- 源码观察：Agent只菜单入口；未见自动/自定义偏好。命名：Agent 模式名称存在。
- 证据：[frontend/src/app/_components/home-creation.tsx:60](../../../../frontend/src/app/_components/home-creation.tsx#L60)；[frontend/src/app/_components/home-creation.tsx:138](../../../../frontend/src/app/_components/home-creation.tsx#L138)；[frontend/src/app/_components/home-creation.tsx:176](../../../../frontend/src/app/_components/home-creation.tsx#L176)。
- 差异与修正建议：先建立Agent会话和偏好合同，区分自动默认与显式自定义，不用普通模型Select替代。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-051 · Agent模型配置受权限影响，提示不能被误当成成功保存

- 检查范围：源码已检查。功能：缺失。
- 源码观察：Agent未运行，无配置保存成功的完整链路。命名：无权限配置文案未定义。
- 证据：[frontend/src/app/_components/home-creation.tsx:138](../../../../frontend/src/app/_components/home-creation.tsx#L138)；[frontend/src/app/_components/home-creation.tsx:176](../../../../frontend/src/app/_components/home-creation.tsx#L176)。
- 差异与修正建议：建立配置权限、保存/失败/读回状态；普通模型凭证检查不能替代Agent配置权限。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-052 · 技能加入编辑器，生成按钮与内容状态联动

- 检查范围：源码已检查。功能：缺失。
- 源码观察：无skill结构；Agent按钮不执行。命名：技能与主体不能混称。
- 证据：[frontend/src/components/generation/prompt-document.ts:8](../../../../frontend/src/components/generation/prompt-document.ts#L8)；[frontend/src/app/_components/home-creation.tsx:176](../../../../frontend/src/app/_components/home-creation.tsx#L176)；[frontend/src/app/_components/home-creation.tsx:424](../../../../frontend/src/app/_components/home-creation.tsx#L424)。
- 差异与修正建议：技能入编辑器与有效内容提交联动，依赖技能对象和Agent能力实现。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-053 · 新建对话、历史会话、执行步骤、工具结果和恢复分别建状态

- 检查范围：源码已检查。功能：缺失主要闭环。
- 源码观察：侧栏对话区待开放，无完整历史会话/步骤/工具结果恢复。命名：新建创作应与新建对话区分。
- 证据：[frontend/src/components/layout/product-sider.tsx:153](../../../../frontend/src/components/layout/product-sider.tsx#L153)；[frontend/src/app/_components/home-creation.tsx:176](../../../../frontend/src/app/_components/home-creation.tsx#L176)。
- 差异与修正建议：建立会话/消息/执行步骤/工具结果及中断恢复；不要复用普通生成任务列表假装对话。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-054 · 生成历史归资产，分图片/视频/音频/文档；空态清晰

- 检查范围：源码已检查。功能：已具备主要基础。
- 源码观察：资产history类型与四类筛选，任务列表/空态有源码。命名：图片/视频/音频/文档已建类别。
- 证据：[frontend/src/app/assets/_components/asset-query.ts:5](../../../../frontend/src/app/assets/_components/asset-query.ts#L5)；[frontend/src/app/assets/_components/asset-query.ts:12](../../../../frontend/src/app/assets/_components/asset-query.ts#L12)；[frontend/src/components/generation/generation-dialog.tsx:208](../../../../frontend/src/components/generation/generation-dialog.tsx#L208)。
- 差异与修正建议：真实账户验证列表权限、过滤、分页、空/错态及路由返回；本轮不宣称运行通过。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-055 · 提交中、受理、排队、运行、成功、失败、取消和未知结果分别记录

- 检查范围：源码已检查。功能：部分具备。
- 源码观察：状态含提交/成功/入库/失败/未知/核对/取消等；轮询和取消存在。命名：排队/运行未作为所查公共状态独立枚举。
- 证据：[frontend/src/components/generation/contracts.ts:3](../../../../frontend/src/components/generation/contracts.ts#L3)；[frontend/src/components/generation/contracts.ts:20](../../../../frontend/src/components/generation/contracts.ts#L20)；[frontend/src/components/generation/generation-dialog.tsx:379](../../../../frontend/src/components/generation/generation-dialog.tsx#L379)；[frontend/src/components/generation/generation-dialog.tsx:426](../../../../frontend/src/components/generation/generation-dialog.tsx#L426)。
- 差异与修正建议：明确应用受理与供应商排队/运行映射、失败和未知恢复；不要把submitted视为已运行。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-056 · 重复点击/网络重试不重复生成；重试与重新生成语义分开

- 检查范围：源码已检查。功能：已具备主要基础。
- 源码观察：冻结quote输入/原confirm幂等键；未知确认重试与新报价分离。命名：重试原确认/复用参数重新报价偏技术表述。
- 证据：[frontend/src/components/generation/rich-generation.tsx:880](../../../../frontend/src/components/generation/rich-generation.tsx#L880)；[frontend/src/components/generation/rich-generation.tsx:936](../../../../frontend/src/components/generation/rich-generation.tsx#L936)；[frontend/src/components/generation/rich-generation.tsx:1395](../../../../frontend/src/components/generation/rich-generation.tsx#L1395)；[frontend/src/components/generation/generation-dialog.tsx:743](../../../../frontend/src/components/generation/generation-dialog.tsx#L743)。
- 差异与修正建议：保留原请求幂等语义，统一重试/再次创作名称；断网、双击、刷新需服务端验收。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-057 · 多结果归属同一任务还是多个任务，数量不足/部分失败如何展示须补证

- 检查范围：源码已检查。功能：部分具备。
- 源码观察：task.outputs数组；图片按output分卡，视频按task聚合；无可用结果提示。命名：结果分组有实现但数量不足语义未完整核验。
- 证据：[frontend/src/app/assets/_components/asset-query.ts:92](../../../../frontend/src/app/assets/_components/asset-query.ts#L92)；[frontend/src/app/assets/_components/history-results.ts:22](../../../../frontend/src/app/assets/_components/history-results.ts#L22)；[frontend/src/components/generation/generation-dialog.tsx:721](../../../../frontend/src/components/generation/generation-dialog.tsx#L721)。
- 差异与修正建议：明确同任务多输出、部分失败、数量不足与审核过滤的区别，再验分页与批量操作。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-058 · 结果预览、原参数、做同款、参考回用、下载和发布分别验收

- 检查范围：源码已检查。功能：部分具备。
- 源码观察：预览/参数/复用/下载/发布入口及部分处理存在。命名：再次创作与复用参数重新报价并存；原参数显示JSON/mode。
- 证据：[frontend/src/components/generation/generation-dialog.tsx:638](../../../../frontend/src/components/generation/generation-dialog.tsx#L638)；[frontend/src/components/generation/generation-dialog.tsx:673](../../../../frontend/src/components/generation/generation-dialog.tsx#L673)；[frontend/src/app/assets/_components/personal-history-list.tsx:350](../../../../frontend/src/app/assets/_components/personal-history-list.tsx#L350)；[frontend/src/app/assets/_components/history-result-card.tsx:154](../../../../frontend/src/app/assets/_components/history-result-card.tsx#L154)；[frontend/src/app/assets/_components/history-result-card.tsx:165](../../../../frontend/src/app/assets/_components/history-result-card.tsx#L165)；[frontend/src/app/assets/_components/history-result-card.tsx:173](../../../../frontend/src/app/assets/_components/history-result-card.tsx#L173)。
- 差异与修正建议：统一用户操作名与参数格式；逐项验真实媒体下载、引用回用、发布回执，不能以按钮存在结案。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-059 · 任务与资产之间有可追踪关联，生成成功不自动等于公开发布

- 检查范围：源码已检查。功能：已具备主要基础。
- 源码观察：operation_id/output_id/media_asset_id关联；publish独立显式动作。命名：生成历史与发布作品语义分离。
- 证据：[frontend/src/app/assets/_components/asset-query.ts:92](../../../../frontend/src/app/assets/_components/asset-query.ts#L92)；[frontend/src/app/assets/_components/history-results.ts:63](../../../../frontend/src/app/assets/_components/history-results.ts#L63)；[frontend/src/app/assets/_components/history-results.ts:72](../../../../frontend/src/app/assets/_components/history-results.ts#L72)；[frontend/src/app/assets/_components/history-publication-dialog.tsx:183](../../../../frontend/src/app/assets/_components/history-publication-dialog.tsx#L183)。
- 差异与修正建议：保持生成→资产→公开发布分层；验引用来源、权限和发布回执，不自动公开生成结果。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-060 · 费用预估、实际扣费、失败退还、余额不足、权限不足分别处理

- 检查范围：源码有限检查。功能：部分具备。
- 源码观察：quote/confirm/余额与权限错误映射存在；真实结算/退还本轮未深审。命名：积分提示与CNY金额有冲突。
- 证据：[frontend/src/components/generation/rich-generation.tsx:1318](../../../../frontend/src/components/generation/rich-generation.tsx#L1318)；[frontend/src/components/generation/contracts.ts:148](../../../../frontend/src/components/generation/contracts.ts#L148)；[frontend/src/components/generation/contracts.ts:172](../../../../frontend/src/components/generation/contracts.ts#L172)；[frontend/src/app/_components/home-creation.tsx:417](../../../../frontend/src/app/_components/home-creation.tsx#L417)。
- 差异与修正建议：单独审计余额保留/扣费/退款/未知结果对账；不得从错误文案推断结算正确。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-061 · 所有普通页面使用同一账号动作组合及头像/菜单样式

- 检查范围：源码已检查。功能：并行修改已收敛源码。
- 源码观察：同一 ProductAccountActions 渲染消息/额度/会员/AccountMenu；未登录包含主题和登录。命名：统一通知与账号组合；消息中心仍是未开放回调。
- 证据：[frontend/src/components/layout/product-header-actions.tsx:47](../../../../frontend/src/components/layout/product-header-actions.tsx#L47)；[frontend/src/components/layout/product-account-actions.tsx:30](../../../../frontend/src/components/layout/product-account-actions.tsx#L30)；[frontend/src/components/layout/product-account-actions.tsx:46](../../../../frontend/src/components/layout/product-account-actions.tsx#L46)；[frontend/src/components/layout/product-account-actions.tsx:51](../../../../frontend/src/components/layout/product-account-actions.tsx#L51)。
- 差异与修正建议：已重新读取父任务新增未提交组件；尚未真实跨路由、菜单、头像、移动端验收，不能宣称所有普通页面样式已一致。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-062 · 页面筛选/搜索/批量操作通过明确插槽接入，不复制账号组

- 检查范围：源码已检查。功能：已具备主要基础。
- 源码观察：navigation/actions插槽和PageToolbar注册存在。命名：asset-tools等标签可改中性名称。
- 证据：[frontend/src/components/layout/product-header.tsx:36](../../../../frontend/src/components/layout/product-header.tsx#L36)；[frontend/src/components/layout/product-header.tsx:39](../../../../frontend/src/components/layout/product-header.tsx#L39)；[frontend/src/components/layout/page-toolbar.tsx:18](../../../../frontend/src/components/layout/page-toolbar.tsx#L18)；[frontend/src/components/layout/page-toolbar.tsx:47](../../../../frontend/src/components/layout/page-toolbar.tsx#L47)。
- 差异与修正建议：复用现有插槽注入业务筛选/搜索/操作，公共账号组独立，勿新增平行Header体系。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-063 · 批量状态明确是否替换公共组，退出后恢复；悬浮菜单与焦点同步清理

- 检查范围：源码已检查。功能：部分具备。
- 源码观察：adjacent/replace-base，owner保护清理存在；焦点同步未验。命名：替换公共组语义需在设计固定。
- 证据：[frontend/src/components/layout/page-toolbar.tsx:18](../../../../frontend/src/components/layout/page-toolbar.tsx#L18)；[frontend/src/components/layout/page-toolbar.tsx:47](../../../../frontend/src/components/layout/page-toolbar.tsx#L47)；[frontend/src/components/layout/product-header.tsx:46](../../../../frontend/src/components/layout/product-header.tsx#L46)。
- 差异与修正建议：验进入/退出批选、路由切换、Popover关闭与回焦；注册清理不等于已完成焦点验收。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-064 · 全局tokens覆盖页面/侧栏/卡片/输入/菜单/弹窗表面与文字层级

- 检查范围：源码有限检查。功能：已有tokens但一致性待验。
- 源码观察：有light/dark和header/composer规则；全页未测量。命名：Token命名与跨页面映射待专项。
- 证据：[frontend/src/app/globals.css:58](../../../../frontend/src/app/globals.css#L58)；[frontend/src/app/globals.css:159](../../../../frontend/src/app/globals.css#L159)；[frontend/src/app/globals.css:318](../../../../frontend/src/app/globals.css#L318)；[frontend/src/app/globals.css:735](../../../../frontend/src/app/globals.css#L735)。
- 差异与修正建议：逐页对照设计token与computed style；不能仅看全局CSS宣称视觉统一。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-065 · 下拉、Popover、Dialog和菜单共享尺寸、圆角、间距、状态与锚定规则

- 检查范围：源码有限检查。功能：共享基础存在。
- 源码观察：ModelPicker复用Popover/Command并设碰撞和视口上限。命名：控件语义有公共实现；全部实例未审。
- 证据：[frontend/src/components/generation/model-picker.tsx:62](../../../../frontend/src/components/generation/model-picker.tsx#L62)；[frontend/src/components/generation/generation-parameters.tsx:43](../../../../frontend/src/components/generation/generation-parameters.tsx#L43)；[frontend/src/components/generation/generation-dialog.tsx:326](../../../../frontend/src/components/generation/generation-dialog.tsx#L326)。
- 差异与修正建议：统一组件变体与实例参数；检查锚点/边距/圆角/长内容，保留有业务必要的不同尺寸。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-066 · 默认/悬停/选中/聚焦/禁用/加载/错误/空态均纳入设计稿和实例检查

- 检查范围：源码有限检查。功能：尚未完成实例验收。
- 源码观察：源码可见禁用/加载/错误/空态，未逐实例核验八态。命名：设计稿节点本轮未读。
- 证据：[frontend/src/components/generation/model-picker.tsx:70](../../../../frontend/src/components/generation/model-picker.tsx#L70)；[frontend/src/components/generation/model-picker.tsx:95](../../../../frontend/src/components/generation/model-picker.tsx#L95)；[frontend/src/components/generation/rich-generation.tsx:2020](../../../../frontend/src/components/generation/rich-generation.tsx#L2020)；[frontend/src/components/generation/reference-mention-menu.tsx:162](../../../../frontend/src/components/generation/reference-mention-menu.tsx#L162)。
- 差异与修正建议：Figma每组件变体与真实实例建立对应并逐态截图；存在条件分支不等于设计覆盖。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-067 · 长名称、长菜单、短视口、200%缩放、键盘关闭/回焦不遮挡主操作

- 检查范围：源码有限检查。功能：部分防护。
- 源码观察：模型菜单视口约束、滚动与名称截断；缩放和回焦未运行。命名：长名称可截断，需可访问全名验证。
- 证据：[frontend/src/components/generation/model-picker.tsx:53](../../../../frontend/src/components/generation/model-picker.tsx#L53)；[frontend/src/components/generation/model-picker.tsx:63](../../../../frontend/src/components/generation/model-picker.tsx#L63)；[frontend/src/components/generation/model-picker.tsx:122](../../../../frontend/src/components/generation/model-picker.tsx#L122)。
- 差异与修正建议：真实测长名称/短高/200%/键盘关闭回焦；控件自身防护不能证明整页无遮挡。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-068 · 390桌面网页裁切不作为移动目标，手机布局须单独设计与验证

- 检查范围：源码有限检查。功能：存在响应式规则，未验。
- 源码观察：header/composer有media规则；本轮不运行移动页面。命名：独立移动设计待验证。
- 证据：[frontend/src/app/globals.css:1135](../../../../frontend/src/app/globals.css#L1135)；[frontend/src/app/globals.css:1234](../../../../frontend/src/app/globals.css#L1234)。
- 差异与修正建议：以390移动目标独立设计和验收，公开即梦桌面UA裁切仅作观察，不能复制缺陷。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-069 · 管理/服务连接等无直接竞品映射页面共享视觉基础，保留其业务合同

- 检查范围：源码有限检查。功能：待跨域专项检查。
- 源码观察：本轮未逐一读取管理/服务连接业务页面。命名：不得伪造即梦对应管理功能。
- 证据：[frontend/src/components/layout/basic-layout.tsx:150](../../../../frontend/src/components/layout/basic-layout.tsx#L150)；[frontend/src/components/layout/product-header-actions.tsx:47](../../../../frontend/src/components/layout/product-header-actions.tsx#L47)。
- 差异与修正建议：普通壳统一后复核管理页面业务权限/配置合同；不为竞品一致性删除BYOK或管理功能。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

### CR-070 · 修改顺序为外部基线→项目差异→设计稿与文档→代码→真实页面验收

- 检查范围：流程记录待验。功能：已遵循阶段进入源码审计。
- 源码观察：外部基线与需求已先提交；本报告只记录第二阶段源码快照。命名：后续设计/代码/浏览器证据仍应独立。
- 证据：[frontend/src/app/_components/home-creation.tsx:138](../../../../frontend/src/app/_components/home-creation.tsx#L138)。
- 差异与修正建议：该行属于过程证据；a92732f1为父任务已报告阶段一提交。本审计没有修改代码/Figma或执行测试。
- 验收边界：本项未执行测试、浏览器或真实供应商验收；视觉未实测，Figma 未检查。

## 本次文档检查

仅检查 CSV 保留原 12 列与 70 个顺序 ID、所有源码路径存在且行号未越界、两份文件的 diff 格式。此类结构检查不证明实现行为正确。并行代码改动后，行号和相应源码结论须重新核对。

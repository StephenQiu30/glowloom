# 即梦 V+A → 新音频菜单：静态规则核查

只读；未操作浏览器、未执行 bundle、未改产品代码。偏移为 Python 已解码字符串的 0-based 字符位置。

资源身份：
- core `/tmp/jimeng-calibration-37.js` SHA256 `8f194bf4096477bf048a451313b9c959da8f50e1887b6e306ebf406b3c75c0a6`。
- data `/tmp/jimeng-multi-canvas-data.js` SHA256 `9a34bf1e304ed3cc59a7cb82ef930e576b6c2e6bbf827ccf9fc682a370da7d2f`。

## 结论

没有找到“视频+音频混合就禁止生成音频”或“新音频只允许一个来源”的固定规则。已下载代码明确显示：两来源 V+A 不触发 baseline count；4秒视频、7秒音频不参与这个菜单 preview 分支的时长判定。新音频有独立、当前本地缺失的创建能力/生成草稿/支持素材类型门禁。仅凭此静态 bundle 不能把源当前 disabled 精确归因到某个 model，不能硬编码 V+A 禁用或音频7秒禁用。

## 确切调用链

| 位置 | 规则 |
| --- | --- |
| data @286164 | audio baseline allowedSourceTypes=[image,video,audio,subject]、maxIncomingConnections=7，没有按类型容量字段。V+A总数2，基础结构合法。 |
| core @486329 `class aQ.evaluateConnection` | 新目标先按baseline逐来源累计总数/每类型数量；合法后对audio调用evaluateProspectiveTarget。 |
| core @559572 `ld` | `audio && !creationPolicy.canCreateAudioNode()`直接experimentDisabled；普通创建菜单走preview。只有image/video且含tool来源会强制complete。音频菜单不走该complete特例。 |
| core @565164 `lu` | 新audio没有generationDraft时，调用`initialGenerationDrafts.tryCreateAudioDraft({scenario:"omni-dubbing"})`。不能创建该draft则仍无draft。 |
| core @524018 `evaluateProspectiveTarget` | audio先`df(...,"preview")`；再只查prospectiveTargetTypeViolation。 |
| core @534847 `df` | audio能力来自`dg(generationDraft,baseline,ports,"preview")`；逐来源使用过滤后的capability做type/结构验证。 |
| core @535472 `dg` | draft必须符合dv且有audioModel；否则null。若提供getAudioConstraintForModel但查不到约束，也返回null。存在约束时按acceptedTypes过滤：image→2、video→3、audio→4、subject仍保留。 |
| core @536067 `dv` | 明确要求activeGenerator=text-to-speech、有效audio-model草稿（tW.Ig）且audioModel.scenario=omni-dubbing。 |
| core @536279 `db` | preview去掉过滤能力的maxIncomingConnections，其余字段保留。模型maxTotalCount不是本次2来源菜单preview的新增总数门禁。先前baseline7仍由aQ检查。 |
| core @528184 `dt` | preview逐来源解析素材类型，只判断constraint.acceptedTypes.includes(type)；不检查duration、perType.maxCount、mutuallyExclusiveTypes。 |
| core @1686787 `class CW` | getAudioConstraintForModel(modelID)读取ready registry中`audio:modelID:unified_edit`，不是硬编码某模型。 |
| core @1998079 module68497 | constraint由registry.materials投影：acceptedTypes来自materials.types.media_type；maxTotalCount来自total_count.max；互斥组、perType counts、duration另有字段。类型编号image2/video3/audio4。 |

因此当前源disabled能归入的静态可达门禁是：①canCreateAudioNode=false；②没有可用omni-dubbing草稿/有效audioModel；③registry capability尚未ready/不存在；④当前约束acceptedTypes缺video或audio（包括subject经解析到某不支持类型）。不能从bundle确定当前运行命中哪条。

在已确认“audio单来源和video单来源各自均enabled”的前提下，这条preview静态链不能解释“仅V+A禁用”；届时应检查当前运行bundle身份、menu绑定的getState参数/草稿与可用性reason，而不是臆造类型互斥。虽然complete路径有互斥、类型数量和时长验证，但`ld`对audio menu preview未调用complete，不能混为菜单禁用证据。

## 最小修复建议（未实施）

1. 保留底层I/V/A/S→audio的结构能力与baseline7，不把本次观测误改成全局禁video或混合来源。
2. 新audio菜单availability和点击创建共用同一“可生成音频草稿+能力ready+acceptedTypes覆盖全部来源”条件。当前本地`supportsCanvasConnection`只有静态ports，不能代表这个更严格创建能力。
3. 在真实能力尚未接到创建流程前，缺失能力应禁用该入口，而不是因为基础port匹配就enable。应明确是未确认能力的fail-closed，不能宣称已复刻即梦当前具体model规则。
4. 提交阶段再按真实能力完整校验数量/互斥/时长；菜单preview与提交complete分层保持清晰。

现有测试位置：
- `frontend/tests/unit/components/canvas/connection-authoring.test.ts`：静态类型、audio总7、整批拒绝；应保留。
- `frontend/tests/unit/components/canvas/multi-connections.test.ts`：多源一次候选。
- `frontend/tests/unit/components/canvas/connection-create-focus.test.tsx` / `connection-state.test.tsx`：创建菜单行为入口。
- editor `createConnectedNode` @1377、菜单候选filter约@1530–1610：新audio能力门禁的接线位置。

建议新增独立能力测试：draft缺失/registry未ready禁用；acceptedTypes=[2,4]时V+A禁用而I+A允许；acceptedTypes=[2,3,4]时V+A基础preview允许；4s/7s值变化不改变仅类型preview（完整提交另测）。这些是规则fixture，不是当前源model数据的声明。

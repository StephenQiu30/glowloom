# director-ai 项目研究

- 编号：RES-006
- 调研日期：2026-08-14
- 分类：AI 分镜/素材包概念验证
- 固定快照：[freestylefly/director_ai@`dd812c756f0ee0533cd7d36042a16144ab1b1202`](https://github.com/freestylefly/director_ai/tree/dd812c756f0ee0533cd7d36042a16144ab1b1202)
- 快照提交时间：2026-05-01
- Stars 快照：1,705，仅作检索记录，不代表成熟度
- 许可证证据：固定提交没有根 LICENSE，GitHub API 未识别许可证
- 研究结论：只作为多源视觉槽位与工作视图概念样本，不作为任务或存储参考

## 1. 公开事实

仓库根 README 不能充分说明真实产品；固定提交的 [`pubspec.yaml`](https://github.com/freestylefly/director_ai/blob/dd812c756f0ee0533cd7d36042a16144ab1b1202/pubspec.yaml) 表明主体是 Flutter/Dart 应用。`web/ai_storyboard_pro_framework.md` 则是一份产品框架，定义角色、场景、道具、风格、叙事五类输入槽位，以及按叙事功能组织的镜头模板。[框架原文](https://github.com/freestylefly/director_ai/blob/dd812c756f0ee0533cd7d36042a16144ab1b1202/web/ai_storyboard_pro_framework.md)

实际 Flutter 模型 `AssetPack` 包含草稿剧本、正式剧本、角色设定表和 `SceneAsset`；单个 SceneAsset 只保存一个图片 URL 与一个视频 URL。持久化服务把整个素材包 JSON 写入 SharedPreferences，最多保存 10 个包，并支持 JSON 导入/导出。[`asset_pack.dart`](https://github.com/freestylefly/director_ai/blob/dd812c756f0ee0533cd7d36042a16144ab1b1202/lib/models/asset_pack.dart) 与 [`asset_pack_storage.dart`](https://github.com/freestylefly/director_ai/blob/dd812c756f0ee0533cd7d36042a16144ab1b1202/lib/services/asset_pack_storage.dart)

## 2. 产品工作流与模块观察

### 2.1 五类参考槽位

**事实**：框架将输入拆为角色、场景、道具、风格和叙事；每类有不同的参考数量、锁定项和权重建议。[产品框架](https://github.com/freestylefly/director_ai/blob/dd812c756f0ee0533cd7d36042a16144ab1b1202/web/ai_storyboard_pro_framework.md)

**推断**：不同参考输入的业务语义不同。即便底层供应商最终只接收有序图片数组，产品也必须知道“第 2 张图是哪个角色的哪个状态”，否则无法审查和失效。

**Lanverse 决策**：使用有类型的 ShotReferenceSlot；参考顺序显式保存，Prompt 中的引用由固定快照编译，不把所有 URL 混在一个数组。

### 2.2 镜头模板

**事实**：框架按建立、聚焦、动势三类列出九种镜头模板，并为镜距、角度、构图、焦距、槽位权重和典型用途提供结构化描述。

**推断**：镜头模板适合作为创作者起点和校验提示，不应成为固定镜头数或自动导演公式。

**Lanverse 决策**：允许镜头规格预设，但不采用“九宫格=九镜头”的规则；镜头数量由叙事覆盖和导演决议决定。

### 2.3 素材包聚合

**事实**：AssetPack 聚合草稿、正式剧本、角色表和场景资源，并以完成场景数计算整体进度。[`asset_pack.dart`](https://github.com/freestylefly/director_ai/blob/dd812c756f0ee0533cd7d36042a16144ab1b1202/lib/models/asset_pack.dart)

**风险**：`SceneAsset` 只有单一图片/视频 URL 和两个布尔完成标记，无法表达候选、生成任务、失败、来源、选择或 stale。包级状态混合编辑、就绪、生成和完成，难以处理部分结果。

**Lanverse 决策**：不采用一个大 AssetPack 作为业务真相；项目进度由镜头、任务、候选和主选事实计算。

## 3. 任务恢复边界

**事实**：固定证据中的 AssetPack 状态包含 drafting、ready、generating、partiallyCompleted、completed、failed，但没有持久任务、供应商 task ID、Attempt、错误详情或恢复租约。[`asset_pack.dart`](https://github.com/freestylefly/director_ai/blob/dd812c756f0ee0533cd7d36042a16144ab1b1202/lib/models/asset_pack.dart)

**推断**：这些状态只适合 Demo 进度展示，不足以承担高成本视频调用。

**待验证**：仓库其他实验脚本是否实现独立任务，但即使存在，也没有证据证明与 Flutter AssetPack 形成统一持久化闭环。

**Lanverse 决策**：不吸收其任务设计；生成必须通过持久化 GenerationTask/ProviderExecution。

## 4. 媒体与版本边界

**事实**：SceneAsset 直接保存 URL 与 Prompt，更新图片/视频会构造新的 SceneAsset，但素材包保存时会按 ID 替换整个包；SharedPreferences 最多保留 10 个包。[模型](https://github.com/freestylefly/director_ai/blob/dd812c756f0ee0533cd7d36042a16144ab1b1202/lib/models/asset_pack.dart) 与 [存储](https://github.com/freestylefly/director_ai/blob/dd812c756f0ee0533cd7d36042a16144ab1b1202/lib/services/asset_pack_storage.dart)

**风险**：

- URL 不等于可管理媒体；
- 更新后的旧结果没有候选历史；
- 自动截断到 10 个包可能导致不可预期数据丢失；
- SharedPreferences 不适合大量图片/视频、并发或事务；
- 没有输入版本与输出血缘。

**Lanverse 决策**：所有媒体独立持久化并有血缘；任何保留/清理策略可见且可解释，不按数量静默截断项目。

## 5. 导出边界

**事实**：存储服务支持把 AssetPack 序列化为 JSON 字符串并重新导入；仓库另有分镜 PDF/脚本导出实验文件，但没有证据证明其与主应用领域模型、候选选择和媒体校验形成稳定合同。[`asset_pack_storage.dart`](https://github.com/freestylefly/director_ai/blob/dd812c756f0ee0533cd7d36042a16144ab1b1202/lib/services/asset_pack_storage.dart)

**Lanverse 决策**：不把 JSON 备份等同镜头素材包；导出必须从显式主选构造固定 Manifest，并检查每个媒体可读。

## 6. 安全与许可边界

固定提交无明确许可证，公开可读不等于可复制。SharedPreferences 和 URL 字段也不能证明密钥加密、文件权限、上传验证或项目隔离。

**Lanverse 决策**：只记录抽象产品模式；不复制代码、模板正文、镜头参数表或页面素材。

## 7. 测试证据与边界

固定提交可见少量 API/Flutter widget 测试入口，例如 [`web/mobile/test/widget_test.dart`](https://github.com/freestylefly/director_ai/blob/dd812c756f0ee0533cd7d36042a16144ab1b1202/web/mobile/test/widget_test.dart)，但未找到与 AssetPack 的事务、恢复、多候选或导出闭包对应的系统性测试证据。

这意味着本文不把产品框架中的完整叙述当作已验证实现。

## 8. 可吸收模式

1. 角色、场景、道具、风格、叙事输入显式分槽；
2. 每个槽位保留用途、顺序和锁定语义；
3. 镜头模板作为起点而非硬约束；
4. 项目可以提供总览、镜头编排和资产工作台等不同视图，但应共享数据。

## 9. 明确拒绝点

- 不移植无明确许可证的代码或模板正文；
- 不使用 SharedPreferences 存项目生产事实；
- 不把单一 SceneAsset URL 作为候选模型；
- 不按最多 10 个包静默清理；
- 不把固定九宫格当镜头数量公式；
- 不把包级状态当任务恢复状态；
- 不把概念文档描述当已实现能力。

## 10. Lanverse 决策

director-ai 只影响 ShotReferenceSlot 与镜头规格预设两个产品概念，不影响技术架构。它是“输入如何让创作者理解”的参考，也是“不能用大 JSON 包和单 URL 承担生产状态”的反例。

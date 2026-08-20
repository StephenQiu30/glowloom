# 切片 A：手工事实主线 PRD

> PRD ID：PRD-A
> 上游模块：M01、M02、M03、M04（最小）、M05、M08（Operation）、M09（Fixture）、M11（上限）、M13（预演）、M14（来源）
> 状态：proposed

## 1. 用户问题与结果

用户需要在没有 Agent 和真实生成 Provider 的情况下，把一份真实脚本变成可追溯、可编辑、可播放的镜头预演。切片 A 用手工闭环验证领域对象、版本、权限和运行骨架是否正确。

## 2. P0 用户旅程

```text
创建 Workspace/项目/Brief
  → 上传 DOCX/Markdown/TXT
  → 查看完整性与原文锚点
  → 手工校对 Scene/Beat/Mention
  → 批准 NarrativeRevision
  → 建立最小人物/地点/道具事实
  → 手工创建并重排不少于 10 个 Shot
  → 用 Fixture 生成占位 Candidate
  → 人工选择分镜参考/视频占位
  → 生成顺序 Animatic
```

## 3. 范围

- 单 Workspace、项目和 ContentUnit，基础成员/项目职责；
- Project Brief 版本、来源保全、ParseReport/Anchor、结构表单；
- 人物身份/别名/稳定外观、地点身份、关键道具和最小状态；
- ShotPlan/Order、覆盖检查、谱系、Fixture Candidate/Selection；
- Operation/Outbox/Worker 恢复、项目资源硬上限、来源权利声明；
- 简单顺序预演，不含真实 Provider、Agent、正式审阅或交付。

## 4. 页面与关键决定

| 页面 | 用户决定 |
| --- | --- |
| 项目创建 | 目标、时长、画幅、资源上限 |
| 导入/结构校对 | AI/解析理解是否正确；首期可完全手工 |
| 生产知识 | 哪个身份/状态可以发布 |
| 镜头表 | 每个 Beat 如何覆盖、镜头顺序和锁定 |
| 候选/预演 | 哪个 Fixture 作为当前 purpose 选择 |
| Operation | 重试、恢复或接管 |

## 5. 明确延期

LangGraph、真实图像/视频、质量 AI、外部审片、正式时间线、完整画布、模板/API、PDF/OCR。切片 A 可以有确定性解析，不允许用假 Agent 冒充 B。

## 6. 验收与退出

1. 真实来源产生 approved NarrativeRevision，所有结构可回跳；
2. 用户手工建立至少 10 个有 Beat/实体绑定的 Shot，覆盖缺口可见；
3. Fixture 结果形成独立 Candidate，主选与预览高亮分离；
4. 重排不改变 Shot ID，旧 Animatic 可重读；
5. 重复命令/Outbox/Worker 重启不产生重复事实；
6. 跨 Workspace 对象/媒体/Worker 负向测试通过；
7. 关闭全部 AI/Provider 后用户仍完成上述旅程。

未满足任一项，不进入切片 B 的正式实现。

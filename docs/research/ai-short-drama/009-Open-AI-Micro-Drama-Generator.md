# Open-AI-Micro-Drama-Generator 项目研究

- 编号：RES-009
- 调研日期：2026-08-14
- 分类：一句话/剧本到成片的演示型管线
- 固定快照：[Anil-matcha/Open-AI-Micro-Drama-Generator@`94ef2a6611696f353a8fc319fbef81b248725e20`](https://github.com/Anil-matcha/Open-AI-Micro-Drama-Generator/tree/94ef2a6611696f353a8fc319fbef81b248725e20)
- 快照提交时间：2026-08-02
- Stars 快照：455，仅作检索记录，不代表成熟度
- 许可证证据：[README 徽标/文字声明 MIT](https://github.com/Anil-matcha/Open-AI-Micro-Drama-Generator/blob/94ef2a6611696f353a8fc319fbef81b248725e20/README.md)，固定提交无根 LICENSE
- 研究结论：适合说明最小 Demo 链路，也集中展示内存任务、静默跳过和黑盒成片的生产风险

## 1. 公开事实

README 描述 Idea-to-Video 和 Script-to-Video 两条管线：故事/人物/场景脚本、角色肖像、分镜、首帧、视频，最后由 MoviePy 拼接。[README](https://github.com/Anil-matcha/Open-AI-Micro-Drama-Generator/blob/94ef2a6611696f353a8fc319fbef81b248725e20/README.md)

FastAPI `api.py` 将所有 job、事件、SSE queue 和结果保存在进程内字典；SSE 重连只会重放同一进程内的事件。输出文件落在 job ID 对应目录。[`server/api.py`](https://github.com/Anil-matcha/Open-AI-Micro-Drama-Generator/blob/94ef2a6611696f353a8fc319fbef81b248725e20/server/api.py)

Shot 模型只有 idx、视觉/动作/音频描述、首帧 URL 和视频 URL。[`server/interfaces/shot.py`](https://github.com/Anil-matcha/Open-AI-Micro-Drama-Generator/blob/94ef2a6611696f353a8fc319fbef81b248725e20/server/interfaces/shot.py)

Idea2Video 对每个 Scene 执行子管线；单个 Scene 异常时打印 Warning 并继续，最后只拼接成功 Scene；只有全部失败才让 Job 失败。[`idea2video.py`](https://github.com/Anil-matcha/Open-AI-Micro-Drama-Generator/blob/94ef2a6611696f353a8fc319fbef81b248725e20/server/pipelines/idea2video.py)

## 2. 工作流与模块

```mermaid
flowchart LR
    A["Idea / Script"] --> B["Agent 生成故事与人物"]
    B --> C["Scene Scripts"]
    C --> D["Shots / First Frames"]
    D --> E["Scene Videos"]
    E --> F["跳过失败 Scene"]
    F --> G["拼接 final_video.mp4"]
```

**产品价值**：流程短、状态直观，适合演示模型组合如何从创意走到结果。

**产品缺口**：没有项目编辑、人工确认、资产版本、镜头规格审核、多候选、主选、影响分析或素材包交接；用户只有提交与等待。

**Lanverse 决策**：不采用黑盒“一次跑完”；每一阶段的候选必须可检查，视频以镜头为局部操作边界。

## 3. 任务恢复与状态反例

### 3.1 内存 Job

**事实**：`jobs` 是进程内 Dict，BackgroundTasks 执行管线，queue 是 asyncio.Queue。[`api.py`](https://github.com/Anil-matcha/Open-AI-Micro-Drama-Generator/blob/94ef2a6611696f353a8fc319fbef81b248725e20/server/api.py)

**后果**：

- 服务重启后 Job、事件、状态和结果索引全部丢失；
- 多进程/多实例请求可能命中没有该 job 的实例；
- SSE 回放只在原进程存活时成立；
- 没有租约、重试、取消、幂等或 provider task ID；
- 文件仍可能存在，但系统无法从文件重建任务真相。

**Lanverse 决策**：所有 Job/Attempt/事件游标持久化；SSE 只是订阅通道，不是状态存储。

### 3.2 失败 Scene 静默缺失

**事实**：某 Scene 失败会继续剩余 Scene，最后把成功部分拼接并将 Job 标 completed。[`idea2video.py`](https://github.com/Anil-matcha/Open-AI-Micro-Drama-Generator/blob/94ef2a6611696f353a8fc319fbef81b248725e20/server/pipelines/idea2video.py)

**风险**：这是语义数据丢失。用户看到“完成”，却不知道故事中间少了一个场景。

**Lanverse 决策**：镜头失败可局部继续，但项目/导出不得静默完整。导出 Gate 必须列出每个缺失、失败、无主选或 stale 镜头；用户不能用“成功的几个片段”冒充完整交付。

## 4. 媒体与版本边界

**事实**：Shot 只保存 URL；输出按 job 文件夹组织，最终以 MoviePy 拼接。[Shot 模型](https://github.com/Anil-matcha/Open-AI-Micro-Drama-Generator/blob/94ef2a6611696f353a8fc319fbef81b248725e20/server/interfaces/shot.py) 与 [API 输出](https://github.com/Anil-matcha/Open-AI-Micro-Drama-Generator/blob/94ef2a6611696f353a8fc319fbef81b248725e20/server/api.py)

**缺口**：

- URL/路径没有独立媒体身份、哈希和血缘；
- 同镜多次结果没有候选集合；
- 没有当前主选、stale 或保留策略；
- 文件系统与 Job 状态没有事务闭包；
- 无项目级长期恢复。

**Lanverse 决策**：不采用 URL-only Shot；MediaAsset、Candidate、Selection 与任务分开。

## 5. 导出边界

**事实**：管线的目标是 `final_video.mp4`，不是逐镜素材包。[README](https://github.com/Anil-matcha/Open-AI-Micro-Drama-Generator/blob/94ef2a6611696f353a8fc319fbef81b248725e20/README.md)

**边界冲突**：Lanverse 当前明确不做拼接和成片。因此该项目只说明“最终拼接不应侵入上游镜头对象”，不能作为导出设计正例。

**Lanverse 决策**：逐镜主选视频 + Manifest 即终点；不把 MoviePy/FFmpeg 纳入核心阶段。

## 6. 安全与许可边界

**事实**：API 使用 `allow_origins=["*"]` 且 `allow_credentials=True`，输出目录作为静态资源挂载；没有从该文件看到认证或项目授权。[`api.py`](https://github.com/Anil-matcha/Open-AI-Micro-Drama-Generator/blob/94ef2a6611696f353a8fc319fbef81b248725e20/server/api.py)

**风险**：这适合本地 Demo，不适合多用户服务；输出 URL、跨域、Job 枚举、资源隔离和输入限制都需重新设计。

README 声明 MIT 但无根 LICENSE，不能据此复制代码。

## 7. 测试证据与边界

固定树未发现覆盖 API Job 恢复、SSE 重连、失败 Scene 完整性、授权或恶意输入的系统测试。README Demo 视频和管线描述不能代替测试。

**待验证**：单次演示能否在其指定供应商账户下运行不影响本文结论，因为生产状态边界已经由代码直接证明不足。

## 8. 可吸收模式

1. Idea 与 Script 两种入口可共享下游镜头生产阶段；
2. 进度事件使用 stage/message/progress 结构，而非只有日志；
3. 输出目录以 job ID 隔离临时产物；
4. 单场景失败不必重跑全部已经成功的场景，但必须显式暴露不完整。

## 9. 明确拒绝点

- 不移植无根 LICENSE 代码；
- 不用内存 Dict/Queue 保存正式 Job；
- 不把 BackgroundTasks 当持久队列；
- 不把 SSE 回放当恢复机制；
- 不跳过失败镜头后仍标完整成功；
- 不只保存 URL/文件路径；
- 不采用一次性黑盒全流程；
- 不进入 MoviePy 拼接与成片范围。

## 10. Lanverse 决策

该项目是重要反例：快速 Demo 的最短路径恰好展示了 Lanverse 必须补齐的持久项目、阶段审核、任务恢复、候选选择和完整性 Gate。它不会影响技术选型，只用于验证“不能怎样做”。

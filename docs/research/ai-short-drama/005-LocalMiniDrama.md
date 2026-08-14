# LocalMiniDrama 项目研究

- 编号：RES-005
- 调研日期：2026-08-14
- 分类：本地优先 AI 短剧垂直工作台
- 固定快照：[xuanyustudio/LocalMiniDrama@`7b6c1a748e9e3013b88a902cfbfd31ec283da0d1`](https://github.com/xuanyustudio/LocalMiniDrama/tree/7b6c1a748e9e3013b88a902cfbfd31ec283da0d1)
- 快照提交时间：2026-08-13
- Stars 快照：1,273，仅作检索记录，不代表成熟度
- 许可证证据：[根 LICENSE 为 MIT](https://github.com/xuanyustudio/LocalMiniDrama/blob/7b6c1a748e9e3013b88a902cfbfd31ec283da0d1/LICENSE)
- 研究结论：本地创作闭环、同源双视图、供应商任务续查与工程导出的强参考

## 1. 公开事实

README 将产品定义为本地优先的短剧/漫剧工具，使用 Vue 3、Node.js、Electron 与 SQLite，并公开八步流程：故事、剧本、角色、场景、道具、分镜、逐镜图片/视频、合成。[README 固定快照](https://github.com/xuanyustudio/LocalMiniDrama/blob/7b6c1a748e9e3013b88a902cfbfd31ec283da0d1/README.md)

本轮与 Lanverse 最相关的事实不是“合成”，而是：

- 列表模式与画布模式使用同源数据；
- 支持补全缺失内容、逐步重试与项目 ZIP 导入/导出；
- `async_tasks` 持久化任务状态、进度、错误、资源 ID 和完成时间；
- 启动时将丢失执行上下文的遗留 `pending/processing` 任务标记为失败；
- `video_generations` 保存 provider task ID，失败后可继续轮询，不重新提交；
- 远程生成成功后尝试下载到项目本地目录；
- README 声明可导出分镜表 HTML。

任务与视频证据见 [`taskService.js`](https://github.com/xuanyustudio/LocalMiniDrama/blob/7b6c1a748e9e3013b88a902cfbfd31ec283da0d1/backend-node/src/services/taskService.js)、[`videoService.js`](https://github.com/xuanyustudio/LocalMiniDrama/blob/7b6c1a748e9e3013b88a902cfbfd31ec283da0d1/backend-node/src/services/videoService.js) 和 [`videos.js`](https://github.com/xuanyustudio/LocalMiniDrama/blob/7b6c1a748e9e3013b88a902cfbfd31ec283da0d1/backend-node/src/routes/videos.js)。

## 2. 工作流与产品模块

```mermaid
flowchart LR
    A["项目 / 多集剧本"] --> B["角色 / 场景 / 道具"]
    B --> C["分镜"]
    C --> D["首帧 / 尾帧 / 多图引用"]
    D --> E["视频生成记录"]
    E --> F["远程结果下载本地"]
    F --> G["工程 ZIP / 分镜表"]
```

### 2.1 列表与画布同源

**事实**：README 明确说明列表精细编辑与画布批量编排是同源数据，画布可编辑节点、生成、创建工作流组和重跑。[画布说明](https://github.com/xuanyustudio/LocalMiniDrama/blob/7b6c1a748e9e3013b88a902cfbfd31ec283da0d1/README.md)

**推断**：画布是视图和批量操作入口，不应成为另一套领域模型或任意执行 DSL。

**Lanverse 决策**：首版优先列表/镜头卡；未来画布只能投影同一 Shot/Task/Asset 数据，不能复制状态。

### 2.2 “补全并生成”

**事实**：README 声明流水线会跳过已有内容，并可只补缺失项。

**推断**：完整项目不应每次从头重跑。系统需要对每个阶段判断“已有、有效、过期、缺失、阻塞”。

**风险**：只检查“字段是否非空”会误用旧版本；真正可复用必须比较输入版本和状态。

**Lanverse 决策**：采用版本依赖和 stale 判断，不用 URL 是否存在代替有效性。

## 3. 任务恢复与未知状态

### 3.1 普通异步任务

`taskService` 将任务写入 SQLite；进程重启时，对进程内 `setImmediate` 已丢失但仍为 `pending/processing` 的任务统一标失败，以避免前端无限轮询。[任务服务](https://github.com/xuanyustudio/LocalMiniDrama/blob/7b6c1a748e9e3013b88a902cfbfd31ec283da0d1/backend-node/src/services/taskService.js)

**吸收判断**：当执行上下文确定已丢失且无法恢复时，“可解释失败”优于永远转圈；但这种策略不能覆盖已在供应商侧执行的任务。

### 3.2 视频供应商任务

`videoService` 持久化 `provider_task_id`，防止同一 video generation 重复 poll；失败记录可通过“继续查询”重新附着上游任务，不重新提交；缺少 provider task ID 时明确要求重新生成。[视频服务](https://github.com/xuanyustudio/LocalMiniDrama/blob/7b6c1a748e9e3013b88a902cfbfd31ec283da0d1/backend-node/src/services/videoService.js) 与 [路由](https://github.com/xuanyustudio/LocalMiniDrama/blob/7b6c1a748e9e3013b88a902cfbfd31ec283da0d1/backend-node/src/routes/videos.js)

这是比“所有遗留任务标失败”更精确的恢复分支：

| 已知事实 | 正确动作 |
| --- | --- |
| 有 provider task ID，供应商可查询 | 恢复轮询，不重提 |
| 无 provider task ID，确认未提交 | 用户可显式新建 Attempt |
| 提交请求超时，是否被接收未知 | 标 `unknown`，先查询/人工核对，绝不自动重提 |
| 本地执行上下文丢失、无外部执行 | 标失败并解释原因 |

`unknown` 是 Lanverse 在该项目基础上必须补充的状态。

### 3.3 取消边界

**事实**：普通任务取消实现会将任务标失败，注释承认无法中断已经执行的 AI 调用。[`taskService.js`](https://github.com/xuanyustudio/LocalMiniDrama/blob/7b6c1a748e9e3013b88a902cfbfd31ec283da0d1/backend-node/src/services/taskService.js)

**拒绝点**：Lanverse 不把“停止前端轮询”或“本地标失败”等同供应商已取消。取消请求、取消确认和供应商仍可能完成必须分开。

## 4. 媒体、候选与版本边界

**事实**：`video_generations` 记录 Prompt、模型、图片/首尾帧/参考图、状态、provider task ID、远程 URL 与本地路径；成功后下载远程视频，并把结果同步写回 storyboard 当前视频字段。[`videoService.js`](https://github.com/xuanyustudio/LocalMiniDrama/blob/7b6c1a748e9e3013b88a902cfbfd31ec283da0d1/backend-node/src/services/videoService.js)

**亮点**：

- 生成记录与本地物理副本同时存在；
- 供应商临时 URL 不被视作唯一长期来源；
- 列表保留近期完成/失败记录，避免轮询瞬间消失；
- 视频历史版本在 README 中被明确列为能力。

**风险**：

- 成功后又把 URL 写回 storyboard 的单一字段，可能让“当前显示”与“人工主选”混淆；
- 代码证据没有证明每镜唯一主选、星标或 stale；
- 本地下载后是否有内容哈希、MIME 校验、病毒扫描和原子落盘尚未证明；
- URL 与本地路径的优先级、丢失恢复和重复文件清理需要另行设计。

**Lanverse 决策**：远程结果先形成不可变 MediaAsset；VideoCandidate 引用媒体；ShotSelection 决定当前使用项。写入候选不得顺手改写主选。

## 5. 导出边界

**事实**：README 声明工程 ZIP 导入/导出和分镜表 HTML；仓库有 [`dramaExportService.js`](https://github.com/xuanyustudio/LocalMiniDrama/blob/7b6c1a748e9e3013b88a902cfbfd31ec283da0d1/backend-node/src/services/dramaExportService.js) 与前端 [`exportStoryboardSheet.js`](https://github.com/xuanyustudio/LocalMiniDrama/blob/7b6c1a748e9e3013b88a902cfbfd31ec283da0d1/frontweb/src/utils/exportStoryboardSheet.js)。

**产品价值**：

- 项目 ZIP 服务备份、迁移和交接；
- 分镜表服务人工检查和外部沟通；
- 两者与“拼接成片”是不同导出类型。

**Lanverse 决策**：当前实现“镜头素材包”，不承诺全项目可重新导入，也不导出成片。包内必须有固定 Manifest、按镜头排序的主选视频和缺失/过期校验结果。

## 6. 安全边界

**事实**：项目定位为本地 Electron/Node/SQLite，凭据写本地配置；固定提交有 [`SECURITY.md`](https://github.com/xuanyustudio/LocalMiniDrama/blob/7b6c1a748e9e3013b88a902cfbfd31ec283da0d1/SECURITY.md)。

**不可推断**：本地运行不等于没有安全风险。桌面应用仍涉及：

- API Key 静态存储与日志泄露；
- 用户提供上传 URL/代理 URL 的 SSRF；
- ZIP 导入的路径穿越和压缩炸弹；
- Electron 页面与本地文件系统的权限边界；
- 下载远程媒体的大小、类型和恶意内容校验。

**Lanverse 决策**：不沿用本地配置和桌面信任模型；SaaS 入口要对远程媒体抓取、压缩包生成和项目授权分别设边界。

## 7. 测试证据与边界

固定提交有任务服务测试 [`backend-node/test/taskService.test.js`](https://github.com/xuanyustudio/LocalMiniDrama/blob/7b6c1a748e9e3013b88a902cfbfd31ec283da0d1/backend-node/test/taskService.test.js)，可核验遗留任务与状态行为。

当前证据不足以证明：

- 真实供应商的重复回调、未知提交、长时间轮询和限流；
- ZIP 导入的完整恶意样本测试；
- 多用户并发和租户隔离；
- 多候选主选冲突、上游改动的失效传播；
- 大项目导出中断后的幂等恢复。

## 8. 可吸收模式

1. 列表与画布是同一数据的不同视图；
2. 按缺失/有效状态局部推进，不整集重跑；
3. 持久化普通任务，重启后清理无法恢复的假运行状态；
4. 有 provider task ID 时继续查询而不重新提交；
5. 生成远程媒体后建立受控长期副本；
6. 项目工程导出、检查表导出和成片导出是三种不同产品；
7. 每个失败向用户说明下一动作。

## 9. 明确拒绝点

- 不移植 LocalMiniDrama 代码、桌面封装或本地配置方式；
- 不用 `setImmediate`/进程内任务支撑正式异步队列；
- 不把用户取消写成 failed，也不假装供应商调用已停止；
- 不把成功结果直接写成 storyboard 当前主选；
- 不用“字段非空”判断阶段可复用；
- 不把项目 ZIP 等同 Lanverse 当前素材包契约；
- 不进入 FFmpeg 合成、配音和成片范围；
- 不因本地优先宣称推断安全或隐私已经解决。

## 10. Lanverse 决策

LocalMiniDrama 为 Lanverse 的恢复与导出提供第二优先证据：任务必须能在刷新和服务重启后恢复真实状态；已知 provider task ID 的任务继续轮询；未知提交停止自动重试；完成媒体进入可控存储。产品界面先以镜头列表完成闭环，未来画布只能成为同源视图。当前导出只做固定已选镜头素材包。

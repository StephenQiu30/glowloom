# ViMax 项目研究

- 编号：RES-011
- 调研日期：2026-08-14
- 分类：Agent 驱动的通用视频生产管线
- 固定快照：[HKUDS/ViMax@`05a48943878312d88fe5a016c12a9654940ecc43`](https://github.com/HKUDS/ViMax/tree/05a48943878312d88fe5a016c12a9654940ecc43)
- 快照提交时间：2026-07-29
- Stars 快照：11,920，仅作检索记录，不代表成熟度
- 许可证证据：[根 LICENSE 为 MIT](https://github.com/HKUDS/ViMax/blob/05a48943878312d88fe5a016c12a9654940ecc43/LICENSE)
- 研究结论：检查点、会话续做与并行依赖有参考价值；“文件存在即有效”不能成为 Lanverse 的恢复规则

## 1. 公开事实

ViMax 公开定义 Idea2Video、Script2Video 与 Novel2Video 三种入口，并把 Agent 对话、命名项目、产物/分镜预览、渲染检查点和供应商设置放入同一个 Web 工作区。[固定提交的中文 README](https://github.com/HKUDS/ViMax/blob/05a48943878312d88fe5a016c12a9654940ecc43/README_ZH.md) 同时声明会生成角色、剧本、分镜、首末帧、镜头视频和最终视频。

实现证据显示，它不仅有一条顺序脚本：

- `SessionIndex` 保存活动会话、阶段、摘要、工作目录和一组 `stale` 标记；
- 会话索引采用文件锁保护读改写，以临时文件替换完成保存；JSON 损坏时先改名保留损坏证据，再从空状态开始；
- Script2Video 使用 `asyncio.Event` 表达“镜头视频必须等待所需首/末帧”，同时并发处理可并行的机位或镜头；
- 多个阶段先检查预期文件是否存在，存在就跳过再次生成。

以上可分别在 [`agent_runtime/session_index.py`](https://github.com/HKUDS/ViMax/blob/05a48943878312d88fe5a016c12a9654940ecc43/agent_runtime/session_index.py) 与 [`pipelines/script2video_pipeline.py`](https://github.com/HKUDS/ViMax/blob/05a48943878312d88fe5a016c12a9654940ecc43/pipelines/script2video_pipeline.py) 复核。

## 2. 工作流与模块

```mermaid
flowchart LR
    A["Idea / Script / Novel"] --> B["会话与工作目录"]
    B --> C["故事 / 角色 / 剧本"]
    C --> D["Storyboard / Shot Description"]
    D --> E["Camera Tree"]
    E --> F["首帧 / 末帧"]
    F --> G["逐镜 Clip"]
    G --> H["Final Video"]
```

### 2.1 交互会话与生产产物并存

**事实**：Session 保存 Agent 最近对话、压缩摘要、当前阶段和工作目录；产物检查表独立扫描故事、角色、分镜、选择器输出、镜头视频与最终视频。[会话模型](https://github.com/HKUDS/ViMax/blob/05a48943878312d88fe5a016c12a9654940ecc43/agent_runtime/session_index.py)

**推断**：创作对话和生产事实可以在一个工作区呈现，但二者不是同一类状态。对话摘要不能代替剧本版本，Agent 说“完成”也不能代替产物验收。

**Lanverse 决策**：Agent 只发起命令、解释结果或提出修改；项目、版本、任务、候选与选择仍由领域对象保存。

### 2.2 机位树与镜头依赖

**事实**：Script2Video 构造 camera tree；视频任务等待本镜所需的 frame event，机位内外按依赖拆分可并行工作。[管线实现](https://github.com/HKUDS/ViMax/blob/05a48943878312d88fe5a016c12a9654940ecc43/pipelines/script2video_pipeline.py)

**推断**：短剧生产不等于“全项目串行”；最小依赖满足后，互不相关镜头可以并行，从而缩短等待时间。

**Lanverse 决策**：以 `shot_id + input_revision` 计算就绪性；调度器只并行相互独立的生成任务，不把 UI 阶段顺序当执行锁。

### 2.3 选择器输出

**事实**：管线为首帧/末帧保存 `*_selector_output.json`，其中记录已选择的参考输入与 Prompt；文件存在时可继续使用。[同一管线实现](https://github.com/HKUDS/ViMax/blob/05a48943878312d88fe5a016c12a9654940ecc43/pipelines/script2video_pipeline.py)

**推断**：选择本身也是需要恢复的创作决议，而不只是一次函数内部变量。

**Lanverse 决策**：参考资产选择、关键帧主选和视频主选都必须是可寻址决议，并记录其依据版本。

## 3. 任务、恢复与失败边界

### 3.1 已有机制

| 机制 | 固定提交事实 | 对 Lanverse 的含义 |
| --- | --- | --- |
| 会话保存 | 文件锁覆盖完整读改写，临时文件后 `os.replace` | 单文件本地模式也要防并发丢更新和半写 |
| 损坏处理 | 损坏的 `sessions.json` 被保留为备份 | 恢复不能以“清空再说”破坏调查证据 |
| 检查点 | 产物文件存在时跳过对应生成 | 有助于续做，但只证明路径存在 |
| 依赖等待 | `asyncio.Event` 控制帧就绪后生成视频 | 调度应基于输入就绪而非固定延时 |
| 网络防挂 | 测试约束下载超时、4xx 不重试、轮询次数上限 | 重试必须按错误类别和预算约束 |

会话锁、损坏备份和原子替换有直接实现与 [`tests/test_agent_session_index.py`](https://github.com/HKUDS/ViMax/blob/05a48943878312d88fe5a016c12a9654940ecc43/tests/test_agent_session_index.py) 佐证；超时/重试边界可见 [`tests/test_hang_guards.py`](https://github.com/HKUDS/ViMax/blob/05a48943878312d88fe5a016c12a9654940ecc43/tests/test_hang_guards.py)。

### 3.2 关键不足

**事实**：帧、选择器输出、镜头视频和最终视频的主要续做判断是 `os.path.exists`。[管线实现](https://github.com/HKUDS/ViMax/blob/05a48943878312d88fe5a016c12a9654940ecc43/pipelines/script2video_pipeline.py)

**推断**：旧输入留下的文件、零字节文件、写到一半的媒体或参数已变的文件，仍可能被误认成有效检查点。

**Lanverse 决策**：检查点有效性至少要求：产物状态成功、媒体可读、内容校验信息存在、输入版本/参数快照匹配且未 stale。路径存在只用于诊断，不能单独驱动跳过。

### 3.3 待验证

- 供应商请求是否持久化 request/task ID，以及进程重启后能否继续轮询；
- 高成本请求的幂等键、未知提交状态与重复扣费保护；
- 取消在依赖图中如何传播，已完成兄弟分支是否保留；
- 多用户或多进程同时更新同一会话时，除会话索引以外的产物是否有一致性保护；
- stale 标记由哪些输入变更触发，以及是否能精确到镜头和产物版本。

## 4. 媒体与版本边界

### 4.1 可吸收证据

`STALE_KEYS` 把 story、characters、script、storyboard、shot descriptions、camera tree、frames、clips 和 final video 分开记录，说明项目承认这些产物可以分别过期。[会话索引](https://github.com/HKUDS/ViMax/blob/05a48943878312d88fe5a016c12a9654940ecc43/agent_runtime/session_index.py)

工作目录把每个镜头的描述、选择器输出、首帧、末帧和视频分别落盘，说明中间产物可观察，而不是只剩最终文件。[管线实现](https://github.com/HKUDS/ViMax/blob/05a48943878312d88fe5a016c12a9654940ecc43/pipelines/script2video_pipeline.py)

### 4.2 风险

- stale 是会话内按类别的布尔值，无法从字段证明“哪个上游版本使哪个镜头结果过期”；
- 目录名和镜头索引承担身份，镜头插入、删除或重排可能改变语义；
- 没有从这些文件结构证明同镜头保留多个视频候选和唯一主选；
- 媒体是否包含哈希、MIME、时长、尺寸、来源、供应商响应和审核状态，不能由路径证明。

**Lanverse 决策**：沿用中间产物可检查的思想，但使用稳定 ID、不可变版本和显式 DependencyEdge；候选历史不以覆盖文件实现。

## 5. 导出边界

**事实**：ViMax 把逐镜 clips 拼接为 `final_video.mp4`，并在文件存在时跳过拼接。[管线实现](https://github.com/HKUDS/ViMax/blob/05a48943878312d88fe5a016c12a9654940ecc43/pipelines/script2video_pipeline.py)

**边界**：当前 Lanverse 不做拼接、时间线、音频或成片，因此不采纳 Final Video 模块。可保留的是“导出前对每个必要产物做完整性检查”的思想。

**Lanverse 决策**：导出只读取镜头顺序和每镜显式主选，生成不可变 ExportManifest 与独立视频文件；不得因某个旧 `final_video.mp4` 存在而宣告交付成功。

## 6. 安全边界

**事实**：Session 将工作目录解析到专用根目录下，并拒绝逃逸；测试覆盖恶意 `../../` 会话 ID 的归一化。[实现](https://github.com/HKUDS/ViMax/blob/05a48943878312d88fe5a016c12a9654940ecc43/agent_runtime/session_index.py) 与 [测试](https://github.com/HKUDS/ViMax/blob/05a48943878312d88fe5a016c12a9654940ecc43/tests/test_agent_session_index.py)

**不可推断**：这不能证明多租户鉴权、上传安全、SSRF、凭据加密、日志脱敏、对象存储签名或 Agent 工具权限安全。

**Lanverse 决策**：所有文件访问以受控媒体 ID 解析，不让客户端或 Agent 直接提交服务器路径；供应商密钥不进入会话、Prompt 或导出清单。

## 7. 测试证据与边界

固定提交至少提供以下与本研究相关的测试：

- [`tests/test_agent_session_index.py`](https://github.com/HKUDS/ViMax/blob/05a48943878312d88fe5a016c12a9654940ecc43/tests/test_agent_session_index.py)：会话、路径限制和产物清单；
- [`tests/test_crash_regressions.py`](https://github.com/HKUDS/ViMax/blob/05a48943878312d88fe5a016c12a9654940ecc43/tests/test_crash_regressions.py)：崩溃回归；
- [`tests/test_hang_guards.py`](https://github.com/HKUDS/ViMax/blob/05a48943878312d88fe5a016c12a9654940ecc43/tests/test_hang_guards.py)：网络超时与轮询上限；
- [`tests/test_script2video_pipeline_guards.py`](https://github.com/HKUDS/ViMax/blob/05a48943878312d88fe5a016c12a9654940ecc43/tests/test_script2video_pipeline_guards.py)：Script2Video 防护条件。

这些测试约束了固定提交的部分本地行为，不证明真实供应商幂等、跨进程队列、多租户安全、大项目负载或长期迁移。

## 8. 可吸收模式

1. 命名项目、会话与可检查产物集中在同一工作区；
2. 对话状态与生产产物状态分离；
3. 损坏状态先保留证据，再进入受限恢复；
4. 原子保存与并发读改写保护；
5. 以最小输入就绪条件驱动镜头并行；
6. 选择器输出本身持久化；
7. 中间产物分阶段可观察；
8. 超时、非重试错误和轮询上限进入可执行测试。

## 9. 明确拒绝点

- 不移植 ViMax 代码、Agent 或供应商适配器；
- 不把一句话到成片当作 Lanverse 当前目标；
- 不以文件存在判断检查点成功；
- 不用镜头序号或目录名作为永久身份；
- 不采用类别级布尔 stale 代替版本依赖；
- 不引入最终拼接、音频和成片模块；
- 不因 Stars 数量、论文或“Agentic”标签推断生产可靠性。

## 10. Lanverse 决策

ViMax 支持两个重要判断：生产流必须能看到并恢复中间产物，且镜头级工作可按真实依赖并行。Lanverse 将把这些能力落实为持久 WorkflowRun/Step/Attempt、不可变 ArtifactRevision、明确 DependencyEdge 和可验证 Checkpoint；不会采用文件存在检查、粗粒度 stale 或最终视频拼接。用户看到的“继续制作”必须说明复用了哪些有效产物、哪些已过期、哪些任务仍需重新确认成本。

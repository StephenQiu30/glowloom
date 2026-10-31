# 主体正式节点：后端持久化与复制验证

日期：2026-10-29。事实来源为设计27 §7.4.4、PRD14 AC-CJC-16、Plan23 T-CJC-08；本证据是后端合同验证，不代表JC-046～047或60项全量产品接受。

## 实现边界

- 正式 `subject/resource` 与 `config.subject={version:1,subject_id?:UUID,version_id?:UUID}`。两个ID必须同时缺省或均为非零UUID；拒绝显式null、残缺身份、未知嵌套字段、错误配置版本及其他节点类型携带主体配置。空节点不是可执行生成输入。
- `config.subject` 是整体CAS字段，进入既有Yjs投影差异、领域校验、原子提交及历史恢复。候选图和中间节点逐一通过原有统一主体所有者解析固定版本，验证全部主/辅/音色binding；保存时不把既往授权当成当前许可。
- 删除/错误版本/媒体状态失效拒绝整笔保存；清除失效pin允许恢复为空态。库头部版本推进不会改变画布已固定version_id。重复节点保存重新验证同一个pin。
- 项目复制通过专用 `NewCanvasCopySources` 消费源项目读取权限与未发布目标写权限；workspace所有者读回当前OwnerScopeID后要求源/目标同空间。无会话worker从当前项目权限恢复scope，复核主体和binding。Freeze、Copy及复制receipt重放均重新验证；节点UUID按复制任务重建，subject_id/version_id保持不变，主体媒体不转成普通项目资产。
- API通过 `pnpm run api:generate` 生成，只有 `frontend/src/api/typings.d.ts` 新增7行（SubjectConfig及NodeConfig.subject）。无需数据库schema改动；沿现有config JSON持久化。

## 隔离环境

本轮以本机PostgreSQL18 `initdb` 新建临时数据目录，监听仅 `127.0.0.1:55439`，初始化独立 `glowloom_subject_alignment_canvas_node` 数据库与仓库 `backend/db/schema.sql`。写入使用 `glowloom_app` 非超级用户角色；测试入口明确检查角色及subject不可变表权限。未读取.env，未连接或修改已有业务数据库。

主体和媒体由受控SQL测试夹具及真实subject服务生成，媒体元数据符合正式路径/归属/版本合同。画布编辑走Node/Yjs二进制与真实PostgreSQL事务，授权使用实际 `NewPromptSources`、workspace会话/worker权限和subject/media所有者，不是假授权仓储。该测试没有调用外部供应商或宣称生产对象字节验收。

临时实例在主体验证完成后借给同一任务AI助手子任务创建独立数据库；根代理负责双方结束后的实例停止与清理，不能宣称已销毁。

## 可执行结果

```sh
GLOWLOOM_TEST_SUBJECT_DB_DSN='postgres://postgres@127.0.0.1:55439/glowloom_subject_alignment_canvas_node?sslmode=disable' \
GLOWLOOM_TEST_PROJECT_COPY_DB_DSN='postgres://postgres@127.0.0.1:55439/glowloom_subject_alignment_canvas_node?sslmode=disable' \
go -C backend test -race ./tests/canvas ./tests/subject \
  -run '^(TestSubjectNode|TestCanvasSubject|TestProjectCopy)' -count=1 -json
```

结果：11个顶层用例、22个含子用例通过，0失败、0跳过。覆盖新主体配置/授权与已有项目复制回归。关键测试：

- `TestSubjectNodeClosedConfig`：有效空/固定pin往返及所有闭合配置反例。最初Red为subject未知节点拒绝，新增正式节点后Green。
- `TestSubjectNodeSourceProofRechecked`：复用pin仍逐媒体复核；不存在、删除、撤权、版本错配、scope/project错配、旧式无binding媒体和空媒体均拒绝。
- `TestSubjectNodeProjectCopyRetainsFixedVersion`：复制重建节点ID但保持固定主体/版本身份。
- `TestCanvasSubjectNodePostgresFixedVersionAndRevocation`：空态保存→同节点导入→新服务实例回读→复制→错误版本整笔回滚→库头推进仍固定旧版→媒体失效拒绝→主体删除后拒绝未改pin的布局保存→清除所有pin恢复。
- `TestCanvasSubjectProjectCopyPostgresWorkerProofAndReplay`：缺复制来源所有者拒绝→冻结→删除后worker拒绝且0目标画布→恢复后无会话/无预设scope worker复制→回执重放→回读固定身份→删除后重放拒绝→同物理组织跨scope目标拒绝。

附加检查：

- `go vet ./internal/canvas/... ./internal/app/subject ./internal/app/projectcopy ./internal/workspace/adapter/postgres ./internal/workspace/application ./tests/canvas ./tests/subject` 通过。
- `golangci-lint run` 同范围通过，0 issues；修改文件 `goimports -l` 无输出。
- `govulncheck ./internal/canvas/... ./internal/app/subject/... ./internal/app/projectcopy/...`：0条影响调用代码的漏洞，0条已导入包漏洞；所需模块另有1条未被调用漏洞，不能表述为整个依赖图零漏洞。
- `git diff --check` 通过；没有提交、推送或部署。

## 尚未由本证据证明

主体节点前端实际渲染、零/单/多选导入取消及撤销、完整主辅媒体/音色修改、两端浏览器保存与刷新、跨项目复制公开API的端到端UI流程、报价及真实供应商执行均需各自补证。现有报价/执行binding链未被此次节点改动替换；本轮没有重新执行供应商请求。

## 后续：画布局部主体与生成输入（2026-10-29）

本节取代上文仅固定库pin/API新增7行的实现边界。正式配置现增加 `local={description,primary?,references,voice?}`，仅接受带精确revision的当前项目媒体或原固定版本内真实binding。辅助上限120，主媒体图/视频、辅助图片、音色音频，重复资产及跨角色拒绝。局部编辑保留/移除库binding但从不写主体库。

- 主体新建、名称或内容变化写真实 `canvas.node_input` 不可变快照；位置不变更input_revision。删除撤销从已删除记录递增，历史恢复从当前/历史最高修订递增；复制写新节点revision1快照。
- 新来源 `canvas_subject` 按项目权限、唯一节点归属、当前input_revision、不可变快照及所有媒体重新验证。纯库主体仍用原subject来源。普通项目媒体复制通过media映射重建asset/revision；继承库binding保留并重新授权。
- 前端空态与编辑器使用真实项目上传/资产列表/preview。编辑名称、说明、主辅媒体、音色，明确局部保存，整体配置CAS及账号/scope/epoch保护；主媒体清除完整空态。各Dialog及节点阻断mouse/pointer手势冒泡。
- 从画布添加已改为现有referenceSelection蒙层直接点选，支持Escape返回；只允许不同节点、正确媒体类型、正式asset UUID，目标删除、冻结、epoch替换撤回选择；不创建连线。
- 智能配音复用正式统一语音options/job/unknown恢复。成功后“使用生成音色”通过既有永久幂等media transfer把个人音频复制到当前项目；轮询/失败原任务重试/未知转移核验均保留原身份，不重复合成。精确核对actor、source item、target project、job revision与成功asset，再请求项目preview后写局部音色。此处是接线与合同证据，尚不是本次真实供应商成功收据。

最终命令：

```sh
GLOWLOOM_TEST_SUBJECT_DB_DSN='postgres://postgres@127.0.0.1:55439/glowloom_subject_alignment_canvas_node?sslmode=disable' \
GLOWLOOM_TEST_PROJECT_COPY_DB_DSN='postgres://postgres@127.0.0.1:55439/glowloom_subject_alignment_canvas_node?sslmode=disable' \
go -C backend test -race ./tests/canvas ./tests/subject ./tests/prompt \
  -run '^(TestSubjectNode|TestSubjectLocal|TestCanvasSubject|TestProjectCopy|TestEditorDraftStrictAndHashes$|TestEditorDraftReferenceCompilation$)' -count=1 -json
```

结果：17个顶层、28个含子用例通过，0失败、0跳过。日志 `/tmp/glowloom-subject-final-race.jsonl`。新增关键用例：

- `TestCanvasSubjectLocalPostgresRevisionAndSources`：真实项目媒体→Yjs保存revision1→布局不递增→改名2/说明3→三份快照→旧source、跨project、伪造说明、已删除source拒绝→删除撤销revision4且旧3拒绝→媒体撤权后生成校验和新的布局保存均拒绝。
- `TestSubjectLocalClosedSources` / `TestSubjectLocalCopyRemapsOnlyProjectMedia`：局部闭合来源校验，普通资产重映射、库binding不变、新输入revision1与源对象不变。复制的局部混合媒体是domain证据；真实worker的固定库pin证明仍由此前PG测试提供。
- `TestCanvasSubjectDraftCompilation`：新来源JSON往返、主图/音频按真实role编译、完整8192字内多字节说明、旧subject描述上限不被放宽、混合来源身份拒绝。

前端：subject状态、画布候选、转移回执及固定source测试4文件13用例通过；`/tmp/glowloom-subject-final-local-vitest.log`。ESLint覆盖subject全部、context、canvas-editor和新增tests通过。Go vet通过；包含canvas/subject/prompt/projectcopy/workspace及测试的golangci-lint为0 issues。API已重新生成SubjectLocal/SubjectMedia/SubjectVoice（同次包含并行导演字段）。整仓tsc一轮通过；后续并行导演motion与yjs测试编辑产生外部类型问题，已通知对应owner，以根代理最终整仓结果为准。

测试过程如实保留：早期混选`TestDraft`导致2个缺CANVAS_DSN失败，新copy测试曾错误按指针地址比较binding导致1个失败；改为值比较并精确选择测试后，上述最终完整运行0失败0跳过。随后一轮泛选EditorDraft产生2个未配置prompt DB跳过，最终命令明确限定纯domain用例且未跳过。

尚待根代理补证：实际浏览器主/辅/voice编辑、直接点选、保存/刷新/另一端与真实媒体播放；真实voice供应商输出及media transfer worker端到端收据；历史版本恢复的专属主体场景（删除undo已PG通过）；混合局部媒体跨项目复制完整公开API路径。未以这些单元/PG通过声称60项全量一致。

## 2026-10-29 主体连线、派生输入与历史版本补证

本节取代上节“主体历史恢复尚无专属场景”的缺口；不取代浏览器和真实供应商验收。实现依据为设计27 §7.4.4、PRD14 AC-CJC-08/16 与 Plan23 T-CJC-08 连线补充。

- `subject/reference_subjects/REFERENCE_SUBJECT/NODE_VALUE` 是正式编辑边，仅接图片/视频/音频；前端复用端口与节点+边编辑，服务端拒绝非法目标、重复边和循环。高级节点在投影中保留原身份，不强行转成普通图片。
- 空主体可保存一条边，但领域生成输入不产生参考；前端连接摘要明确“待添加主媒体，不用于生成”，普通媒体提及列表不伪造主体条目。
- `GenerationInput` 组合主体与导演工具箱的派生输入，原composer保持不变。边ID生成稳定参考ID，按order/ID排序分配主体编号；局部主体冻结节点/input_revision和主辅音色媒体事实。库pin在node_input冻结subject_id/version_id，正式ReadInput在同事务内调用统一主体owner展开固定版本并重新核验，当前库head不能替换固定版本。
- 普通保存先全图推进主体修订，再冻结下游，防止目标节点排在源节点前导致版本混用；布局不推进，源内容改变、删除/恢复推进目标输入。用户composer不能伪造或占用派生边ID。
- 真实历史回归发现：删除undo后目标修订5，恢复早期图曾回退到3。修复为从数据库/历史最高输入修订推进，同时写完整GenerationInput快照；即使恢复到没有生成草稿的资源节点，也保留输入高水位，后来重新添加草稿不复用旧主键。

### 测试证据

`/tmp/glowloom-subject-edge-red.log` 保留最初两个领域测试拒绝未分类主体边的失败；实现后对应测试通过。`/tmp/glowloom-subject-edge-history-red.log` 为真实Yjs+PG历史回退红例，`/tmp/glowloom-subject-edge-history-green.log` 为修复后通过。PG采用前述隔离实例及真实glowloom_app权限、Yjs运行时、统一subject owner；媒体对象元数据仍是受控fixture，不代表真实对象字节或供应商生成成功。

最终组合命令（runtime以owner连接后在会话切换非登录角色，未变更角色权限）：

```sh
GLOWLOOM_TEST_CANVAS_DB_DSN='postgres://postgres@127.0.0.1:55439/glowloom_subject_alignment_canvas_node?sslmode=disable' \
GLOWLOOM_TEST_CANVAS_M1_RUNTIME_DSN='postgres://postgres@127.0.0.1:55439/glowloom_subject_alignment_canvas_node?sslmode=disable&options=-c%20role%3Dglowloom_app' \
GLOWLOOM_TEST_SUBJECT_DB_DSN='postgres://postgres@127.0.0.1:55439/glowloom_subject_alignment_canvas_node?sslmode=disable' \
go -C backend test -race ./tests/canvas ./tests/subject \
  -run 'Test(SubjectConnection|CanvasSubject|CinemaKit|ResourceInput|YjsHistory)' -count=1 -json
```

结果：20个顶层、39个含子用例通过，0失败、0跳过；`/tmp/glowloom-subject-edge-backend-final.jsonl`。包括空边无参考、全部合法/非法目标、目标先于源、双主体顺序与编号、工具箱共存、草稿不变、伪造ID拒绝、资源输入旧回归，以及专属PG测试 `TestCanvasSubjectConnectionPostgresFreezeReadbackAndUndo`：保存/新store读回、库head变化保持旧pin、跨project/旧输入拒绝、局部版本、不可变node_input、删除undo、真实历史恢复、旧确认输入拒绝、素材撤权拒绝。

前端4文件78用例通过（subject-connections、canvas-subject-source、service-graph、yjs-live-session），日志 `/tmp/glowloom-subject-edge-front-suite.log`。新增5用例覆盖三类目标正式边保存往返、空主体不造普通媒体参考、高级节点保留、重复连接幂等、非法目标拒绝，以及摘要随填充/删除/图恢复/无文档变更。scoped ESLint和完整tsc通过，分别见 `/tmp/glowloom-subject-edge-eslint.log`、`/tmp/glowloom-subject-edge-tsc-final.log`。共享领域/PG/测试范围golangci本次主体变更无问题；并行新增cinema-kit测试的单一import分组问题已通知其所有者修正，以集成检查为准。

过程失败未跳过：首次扩展资源回归漏CANVAS_DSN，后续用glowloom_app直接登录被角色NOLOGIN拒绝；最终使用仓库既定session role方式重跑全部选中用例。首次PG用例的操作名误写create_edge，按正式connect修正后通过。以上均不是产品成功证据。

### 仍未接受的边界

- 根代理尚需真实浏览器验证主体拖线/新建目标、保存刷新、删除撤销和生成费用确认；当前测试不能代表外部供应商生成成功。
- 项目整体复制领域现有合同拒绝所有 `IsDraft` 生成节点；本次未擅自放宽该范围。固定pin/局部主体既有worker复制结果保留，带生成草稿与主体连线的跨项目完整复制仍未闭合。
- 音色供应商+媒体transfer实际worker结果、混合局部媒体公开API跨项目复制和另一协作端完整可视回读，仍按此前缺口验收。

# 画布统一素材选择器回执

2026-10-29；研究10 E22、JC-052～054；设计27 §7.4.14 → PRD14 → Plan23。共享工作区增量，未提交。本回执不将隔离测试当作真实账号或源站全量一致验收。

## 正式链路

- 项目资产使用 `GET /media/library?scope=project&project_id=…` 的权限、ready/passed/未回收约束。新增创建时间半开区间 `created_from/created_before`、真实 `origin=upload|generated|system`、图片 `resolutions/ratios`，全部在完整授权集合的 count/order/page 前执行。search 复用完整 SQL 查询；前端不再遍历旧 UUID 游标全部页面后客户端过滤。
- 分辨率按真实长边 1024/2048/4096/8192，比例精确交叉乘积；缺失维度不命中。服务端与当前 UI 均支持同类 OR / 异类 AND，多选枚举按既有选项顺序发送。不会由文件名或高像素推断超清处理事实。
- 主画布一级资产/主体/画布，资产提供图片/视频/音频/文档；显式 director model 与 timeline 的允许类型保留。跨页选中保留，添加前逐项 getMediaPreview 核验项目、媒体 id/kind/revision。数量来自服务端；加载/空态/错误/重试、清除及日期范围可见。
- 主体读取当前身份/空间正式 listSubjects 的搜索、收藏、日期、排序、分页，保留旧 Bible onSubject 调用者。新增 onUnifiedSubject；确认 getSubjectVersion 再验证 scope 与固定 subject/version，创建正式 subject 节点，不转成图片或追随最新版本。
- 画布使用正式 listAssetCanvases 当前授权空间集合的 search/time/favorite 和筛选绑定游标；只浏览，明确“整张画布添加尚未接入”。
- 主画布打开 picker 时捕获 document epoch、project、scope/session。关闭、项目切换、账号/空间变化或历史恢复后拒绝迟到主体/媒体回包。integration getter 是私有读取接口，不进入持久 DTO。新弹窗外壳阻止 mouse/pointer 冒泡，筛选 portal 标记 no-zoom。

## 所有者与变更范围

后端仅 media/application/library 与 adapter/http/postgres 查询及 tests/media/library_picker*；无 DB schema/migration/收藏写入，未改主体/画布服务所有者。API 与 assistant 同轮统一生成，没有手改 generated 文件。

前端新增 components/media/project-picker.ts、project-picker-filters.tsx、project-picker-sources.tsx，修改 ProjectAssetDialog、AssetPickerModal；editor 仅 subject 早返回导入分支和 epoch prop，integration 类型与 yjs-workspace 接 getter，asset replacement 显式拒绝 subject。未改其他任务资产页。

## 验证

- Red：query/receipt 测试在 helper 缺失时失败；Go filter validation 在字段尚不存在时编译失败；实现后 green。
- 隔离 PG18 `glowloom_canvas_director_tests`、受限 `glowloom_app`：`go test -race ./tests/media -run '^TestLibraryPicker' -count=1` 通过，2.707s。真实 repository/SQL 覆盖组合过滤先于 total/page、跨页身份不同、末页、时间半开边界、来源事实、同类 OR、缺失维度排除、跨 actor/project 拒绝。仅隔离测试数据，不是生产账户/云对象存储验收。
- HTTP strict query 覆盖完整传参、重复/未知参数、非法日期区间、空/重复枚举、音频携带图片条件、项目收藏 422；非法输入不进入 repository。
- 前端 8 files / 40 tests 通过（`/tmp/canvas-picker-final-tests.log`）：query/receipt、现有选择器多页选择/版本、真实来源查询、固定主体版本、关闭取消、旧项目闭包与 epoch/session 迟到拒绝、subject state、theme policy。
- 全项目 `tsc --noEmit`、定向 ESLint `--max-warnings 0` 通过；日志 `/tmp/canvas-picker-final-tsc.log`、`/tmp/canvas-picker-eslint.log`。
- `go vet ./internal/media/... ./tests/media`、`golangci-lint run ./internal/media/... ./tests/media/...` 通过，0 issues。`govulncheck ./internal/media/...` 0 可达漏洞；另报告依赖模块中 1 项本代码不调用的漏洞，不表示该依赖模块本身无漏洞。

## 保留缺口

1. 项目资产没有当前 actor 私有收藏合同，现有 favorite 归个人库；未用共享 metadata 冒充。图片超清 process 来源事实未接入，UI 明示不可用。
2. 文档真实浏览，但解析/添加节点未实现，明确不可选；不换扩展名、假装图片或伪造本地文件。
3. 整张画布导入/引用成功语义未证，本轮只浏览。跨画布节点复制是其他正式工作流，不能冒充本 picker 整图添加。
4. 规格多选 UI 已在本回执后续补齐；非空实际账号筛选结果与源站全套交互一致仍须独立验证。
5. 非空真实项目素材筛选/添加、主体真实 Yjs 保存→刷新回读、长期权限变化和全部响应式视觉待独立浏览器验收；root 的浏览器回执单列。本地测试不代替这些证据。


## 后续增量：图片规格多选与原生 Figma

2026-10-29：依据 E22 已证的 1K/2K/4K/8K 与八个比例选项，复用 shadcn Checkbox、FieldSet/FieldLegend、Popover、Button。分辨率与比例各支持多选、取消单项、独立清空、已选数量；筛选按钮显示来源与规格总数，清除筛选恢复无条件。改动仅 project-picker model/filters、ProjectAssetDialog 的媒体类型切换与上传类型切换处，不涉及 Session/editor 或其他资产模块。

query 将 `resolutions` / `ratios` 数组转为正式 API 的逗号枚举。同组 OR 与跨组 AND 仍由服务端处理；切到非图片媒体会清除图片条件，通用来源和时间保留，切回图片不恢复旧条件。条件变化重建第一页读取，原已选素材继续保留。

Red：新增 Checkbox/多选查询测试在旧单选实现上失败 2 项（`/tmp/canvas-picker-multiselect-red.log`）。Green：9 files / 42 tests 通过，包含真实弹窗切类型后 query 不带非法规格且 page=1；替换为官方 FieldSet 后受影响的 2 files / 11 tests 再次通过。定向 ESLint 0 warnings。全项目 tsc 首次通过；后续并行 search 改动产生 Preview.renditions 与 getByRole exact 类型错误（`/tmp/canvas-picker-multiselect-tsc.log`），已反馈 root，picker 本轮无类型错误，不能把这一轮全项目 tsc 报为通过。

原生 Figma：同文件 `uLqmeRWuQuzfrp9xOYg6FT` / 最终设计稿 `1361:10`，根 `3155:54162`，位置 x243000/y40，2092×1836。四态 `3155:54166` 默认空态、`3155:54291` 四项多选、`3155:54516` 清除归零、`3155:54728` 视频规格隔离。复用已有 Canvas/AssetLibrary Empty `1118:3013` 复合结构、Checkbox `737:26391`、Button `268:2493` 与现有 surface/foreground/border 变量。Code Connect 无匹配，订阅远程库为空，设计系统搜索未返回相关组件，保留本地已采用组件体系。

写回读证据：345 个后代（TEXT 114 / FRAME 93 / INSTANCE 110 / VECTOR 28），0 IMAGE；Geist + Noto Sans SC 字体断言通过。首次组合截图暴露浮层高度固定，已修正为 AUTO，并改善外层画板对比度；最后截图通过视觉检查，未新增未完成能力为可用入口。截图 `project-picker-figma-native.png` 是设计回执，不是扁平图片交付，也不替代真实浏览器结果。

本轮依旧不包含项目收藏、超清处理筛选、文档写入或整画布添加。

# 即梦资产：可落实合同及证据边界

日期：2026-10-28。仅静态阅读下载的实际发行脚本；未执行脚本或提交即梦资产写入。REA父证据 `ev_ab139b6a843600e6060be4f985a7ec856a822b48af793f278420b065fe8b405a`：4份Asset，469774字节、115模块、0解析失败；图partial，未解析依赖不作不存在结论。新静态结论应替代旧SSR脚本“不知道操作粒度”的暂时结论，但服务端存储后果仍未观察。

可复查原文：[verified-excerpts.json](verified-excerpts.json)，区间均为零基UTF-8字节 `[start,end)`。旧历史请求字段见[shipped-static-review.json](shipped-static-review.json)。

## 发行文件身份

| 简称 | 文件 | SHA-256 |
| --- | --- | --- |
| A1 | Asset-1.03382db4a7.js | c7adb75e86d6977bc63601e3289a08b94170e19ba12187de4da0c811c1140aeb |
| A3 | Asset-3.2c061010c3.js | 34eb8bda1832e104f61da6cba1c6d4589e8ef7593b9e8816785842eb6e452e82 |
| A4 | Asset-4.c92de55cba.js | 7cf2fb03abbb39863822b7505253b7032ce9144a6b3d440475fb5754e31ffedb |

## 1. 选择对象与逐媒体动作

| 媒体 | 代码事实 | 可落实规则与限制 |
| --- | --- | --- |
| 图片 | A3 `[118354,119092)` 的 `deleteHistoryList({idList,itemIdListMap})` 按结果 `commonAttr.id` 移除itemList/imageList，仅当记录剩余结果为空才移除记录；同步移除相应favoriteItemIdList。A3 `[166786,167933)` 批量发布将 `selectedImageList.itemId` 展开为 `{record,defaultIndex}` | 支持结果级选择、收藏、删除、下载和发布。不得把一次多图任务一律当作一个不可拆对象；删除一个结果不应影响兄弟结果 |
| 视频 | A1 `[126750,130800)` 多选保存 historyRecordId，下载传被选记录；A3 `[167934,168920)` 批量发布 `selectedHistoryIds`→每记录 `{record}` | 当前分支选择粒度为视频记录。发布分单项/批量且有权限/调试开关；VideoDubbing不显示发布。多选中有VideoBGM或AudioVideoMix时禁用批量下载，单选可下载 |
| 音频 | A3 `[44871,45069)` 选择键 `uniqueKey:audioIndex`；`[41097,42899)` 单项/批量收藏、删除、下载均构造 `idMap[recordUniqueKey]=item.id[]` | 是结果级选择；实际工具栏为删除、下载、收藏/取消收藏，没有发布。批量下载有loading，完成后退出选择；确认删除成功才退出 |
| 文档 | A1 `[93400,95500)` 卡片在正常模式打开详情，传唯一记录键+itemIndex+itemId；收藏请求传 `selectedDocumentList`。`[100900,104200)` 批量按选中文档计数，只有删除、格式选择下载、收藏 | 文档不增加发布按钮；下载应提供真实格式合同。代码可确认收藏批量禁用阈值为>50，删除/下载还依赖未取得外部常量，不能编造这些常量数值 |

图片收藏页会按 favoriteItemIdList裁剪到已收藏结果，并在全部结果取消收藏时移除该记录（A3 `[122420,124016)`）。不要以任务级favorite布尔替代结果级收藏。

## 2. 删除的准确边界

- 已确认客户端可以删除指定结果，并同步刷新各列表、详情数据源、收藏状态。
- 生成历史真正发请求的共享helper：`It`（视频删除）、`RMj`（音频结果删除）尚未在已下载块中找到定义；因此尚不能从发行代码证明其HTTP载荷、底层软/硬删除或对象文件回收策略。
- 现代短剧删除确认明确提示“项目将被永久删除且无法找回”，使用 `Promise.allSettled`，有任一失败显示失败并刷新（A1 `[155450,157980)`）。A4 `[26106,26550)` 对thread对象调用delete_thread，对creative_project提交status:3。**产品不可找回不等于已证明物理字节被删除。**
- 织光采用的重建设计为个人history剔除、保留执行账本/共享引用、无回收入口。该方案由父任务依据全量对齐目标和共享账本保护提出，是织光重建设计合同；不能伪称已观察到即梦同样的数据存储实现。结果级隐藏必须按actor隔离，保留其他成员及兄弟结果。

## 3. 画布与历史画布

- `OldCanvasListManager` 是明确的类/装饰器名：A3 `GmZ` `[72421,76658)`，持有 `oldItems`，列表 `getUserDraft({workspaceId,offset,filterDraftType:Canvas,limit:50})`；删除 `deleteUserDraft`，重命名 `renameUserDraft`。
- 现代InfiniteCanvas分支 `v1m` 对比使用 `InfiniteCanvasProjectDataService.deleteProjects`、`updateInfiniteCanvasProject`（A3 `[60312,61611)`）。AI Canvas另有project批量删除/收藏接口和明确权限（A3 `[5189,7385)`）。
- A1 `[32345,32844)` 旧画布label使用 `web_mainframe_asset_tab_old_canvas`，fallback“旧画布”。结合真实页面显示“历史画布”，可以将其归为旧画布/旧草稿工作流；**不是归档、回收站或文档版本历史**。
- 单项菜单按来源及permission生成；新AI Canvas复制有独立copyPromise防重复，模板先复制后打开。旧画布不能凭通用菜单补出复制能力。批量仅删除和收藏，零项或无权限禁用。
- 画布收藏先乐观更新，失败回滚；AI Canvas用batchFavoriteProjects，旧型画布收藏type=4，InfiniteCanvas type=20，不应共用模糊资源ID合同。

## 4. 短剧是独立容器

现代短剧分支标记 `short_novel`，新建文案“新建短剧画布”；A4 `[24400,27300)` 列表type=NOVEL，新建creative_project_canvas后submit_run，收藏每批1..100，复制区分thread和project。A1 `[164350,165850)` 菜单为重命名、按权限收藏、符合条件发布、创建副本、按权限删除；模板单独标记且不当作用户对象。不能把旧story列表的处理器统一当作全部现代短剧。

## 5. 筛选数据字段

真实历史请求构造（旧 `o94.4a0ac050c1.js` SHA a6d892e37377e401f0f7d9baf8407a5b007202bdaa253c051facbd9584be9b0f，`[15300,18300)`）：

| 控件 | 客户端字段→请求字段 | 边界 |
| --- | --- | --- |
| 媒体来源 | filterTypeList→asset_type_list | 图片查询包括Image+CreationAgent，见A3 `[115006,117028)`；不可只靠已落库媒体文件筛掉失败/处理中任务 |
| 收藏 | filterStatusList含Favorited(-1)→option.only_favorited / cond.only_favorited | 图片/音频能限定结果级收藏 |
| 超清 | filterStatusList含SuperResolution(1)→show_type_list:[1] | 是处理类型，不能只按像素尺寸推断 |
| 补帧 | filterStatusList含FrameInterpolation(-4)→is_insert_frame | 视频处理类型 |
| 分辨率 | resolutionList→resolution_list | UI选项见原调研；私有枚举值尚未补证，不冒造 |
| 比例 | ratioTypeList→ratio_type_list | UI选项见原调研；私有枚举值尚未补证 |
| 生成模式 | generationModeList→generation_mode_list | 音频分支另用generateTypeFilters：Text2Song/TTSGenerateAudio/Text2Instrumental，A3 `[31494,31901)` |
| 日期 | filterEndTimeStamp→begin_time_stamp；filterBeginTimeStamp→end_time_stamp | 注意服务内部命名反向；不能据此证明竞品日期边界/时区。织光采用明确时区半开区间属于已确定设计 |
| 搜索/排序 | keyword、sort_field、sort_order；普通列表direction/orderBy | 搜索为独立请求流程；新条件重置offset/searchId和旧请求版本，不能在已加载页做假全局筛选 |

多选同组应OR，不同组应AND是织光待落实的查询设计；在未取得helper实现前，不将它冒称已穷尽验证的即梦布尔组合。

## 6. 对织光可复用能力的只读核查

- `backend/internal/media/adapter/http/library_media.go` 已有单项真实下载；项目媒体的 `document.go` 也注册 `/projects/:pid/media/:asset_id/download`。
- `media/application/library_media.go:129` 会检查原件SHA并返回真实Downloaded文件，可作为按结果授权下载基础。
- `GET /api/media/library/packages/export` / `LibraryPackageService.Export` 是整库完整ZIP导出，保留manifest和完整性验证；不能把它直接冒称“仅选中生成结果”的批量下载，需要选择范围合同。
- mediatool media-exports 是保存时间轴后渲染/审核/下载，不能当作现成视频文件的下载。
- 在当前HTTP路由中没有找到面向创作结果的社区发布接口。`PublishTransferItem` 等“publish”意为入库发布原子状态；不能当作公开作品发布。

## 7. 当时未决项与后续补证入口

仍缺共享helper定义：It/RMj（生成删除）、Qtv/Xrk/EIq（收藏）、ZBi/vHO/sdT（下载）、ej7（高级筛选归一化）、MrI（画布收藏）、t9（草稿枚举）。旧26+新4+main/$.入口及六份content-generator块未找到其定义；这只是已下载边界，不是全站不存在。需要从已观察manifest的共享块补齐，不能猜新URL。发布modal的表单/提交/取消发布及批量部分成功合同仍需父任务新页面证据。


## 8. 后续补证取代范围

本文件第1／2／7节关于删除helper、PDF／Markdown及上限常量的未知，已由[调研08第8.3节](../../08-即梦资产完整需求分析.md#83-全量对齐共享模块补证)补充；早期边界保留用于复查，不能继续当作当前缺失。具体结果删除和整记录删除已定位到remove_history客户端封装，但物理字节清除、非空竞品运行及其服务端权限仍未观察。织光的个人展示剔除属于重建设计。

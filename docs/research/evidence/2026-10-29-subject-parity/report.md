# 主体剩余差异逐项核对

日期：2026-10-29。源：已登录Chrome的即梦/ai-tool/elements；目标：localhost:1122/subjects。真实浏览器通过Chrome扩展控制；agent-browser无法接入其CDP端口，不修改浏览器配置。

| 项 | 本轮即梦证据 | 本地调整 |
| --- | --- | --- |
| 搜索 | 顶栏输入即生效，禁排序，结果显示创建时间；jimeng-search.txt/png及Elements setQuery | 去跨页CommandDialog，仅主体启用内联搜索 |
| 日期 | 预设后顶栏显示最近一周，菜单保持展开，无正文条件条 | 顶栏显示当前标签和清空 |
| 批量 | 切换时间退出选择，批量取消收藏成功退出，测试后恢复收藏 | 全成功退出，失败保留 |
| 关闭编辑 | 新建输入名称、已有主体改描述均直接关闭，不保存 | 去普通dirty确认，保留未知保存处理 |
| 删除 | 标题、不可恢复/历史编辑警告、取消/删除/关闭，无名称列表；未执行删除 | 去名称列表 |
| 补充参考 | loader的eW为右侧列，预览/删除/添加，无前移后移 | 右侧列，去重排按钮 |
| 来源选择 | 真实界面无搜索/分页，超清可进入空态；账号暂无素材 | 内部滚动累积，取消额外搜索与能力说明 |

素材选择器脚本clo.b503e73c4a.js由当前页面Debugger.scriptParsed定位、getScriptSource捕获。REA summary `ev_0146093d5090f88ab932de58ef3a22747a4c9378c3761b8cca91ec045b189fa2`，parent `ev_dfde8dd64c87a1bab8f9473d28e52061adb29bc57199d01843b9beaae749bd9f`。zI在9615，滚动阈值10473，图片loadMore25077/count30在26247，视频27993/count50在29030，maskClosable:false在22520，跨分组多选31000。5列等高虚拟行不是瀑布流；200ms滚动节流，接近容器底部加载。loadMore目标12原始记录/22转换去重项不是HTTP批量。静态分析未执行bootstrap、图关系partial；前三份同名导出误匹配已排除。

源端素材为空，真实第二批未观测；本轮不启动收费配音。音色候选数组、批量上传数量边界及源端下载格式需独立真实样本，不能因本轮列表通过宣称全部功能已一致。

## 本地实测与设计读回

实际工具观察汇总在observed-browser.json；部分AX文本是增量无变化回执，不作为完整树证明，需结合截图和操作观察记录。

- 正式登录Chrome：内联搜索不用Enter即返回，排序禁用并显示创建时间；清空恢复。时间预设和自定义日期立即应用、主菜单保留；日期清空后正文无额外状态条。
- 卡片选择进入批量，收藏筛选退出；真实批量收藏成功退出，随后恢复原未收藏状态。未执行删除。
- 真实资产选择两张图片，切到超清仍保留已选2；逐项核验后进入主参考右列。保存既有测试主体，刷新并重开仍有两张补充参考，见local-multi-reference-reopened.txt。测试主体保留此次参考供检查，没有触发生成。
- 修改描述后关闭直接返回，重开原描述未变，见local-dirty-close-reopened.txt。未知保存恢复逻辑继续保留。
- 390×844：编辑器与素材弹窗均为358px宽、左右16px，document.scrollWidth=390；确认按钮可见，手机输入及操作最小44px。素材弹窗使用两列，桌面五列。见local-mobile-editor.png、local-mobile-picker.png。
- 桌面筛选/时间/排序/账号均top14、height36，垂直中心32；正文无分页、页脚。后续并行导演台依赖缺失导致本地开发页出现mediabunny构建错误，已留local-parallel-build-block.txt/png；不将此时页面记录为正常最终态。
- Figma原位修改10个搜索状态、4个日历、5个来源稿、11个设置状态及手机变体；手机批量保留收藏星标、隐藏更多。逐项节点读回与边界见figma-alignment.json。空素材设计不作为真实多选证据。

## 工程验证

- 主体与内联搜索专项：19文件129测试通过；后续全量复测中这些测试继续通过。来源切组回到首批的测试改为等待响应状态，未放宽断言。列表也补了已加载第二批→切筛选→返回的2项回归，避免复用旧page。
- agent-browser独立Chrome执行源码fixture：真实IntersectionObserver触发1/2/3批累计45卡片，继续滚动不再请求，无分页。axe WCAG2A/AA为0违规；这是合成回执，不是正式后端或即梦第二批。见fixture-validation.json。
- 修改范围ESLint/Prettier及diff-check通过。next typegen通过；早期tsc通过，后续全量tsc命中并行generation-parameters.test.tsx:123的ByRoleOptions.exact类型错误。随后next build完成编译、类型及34页生成，退出0；共享目录持续变化，结果不等价于后续全部修改已验证。
- 全量vitest最终214文件：1404通过、4失败；失败为theme-policy、business-storage-policy、director-model、director-model-import，均不在提交范围。全量ESLint被public/__director_validation/fixture.js的62错误阻断；全量Prettier发现11个范围外文件。未覆盖或修复并行改动。

本轮收敛可观察的主体界面和既有加载合同，没有新建框架、服务或配置格式。真实豆包/TOS、来源大数据第二批、视频时长展示及批量上传上限仍需对应样本，不纳入本轮接受结论。

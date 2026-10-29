# 主体剩余差异核验（2026-10-29）

## 范围与证据

在真实 Chrome 中复核即梦 `/ai-tool/elements` 与本地 `/subjects`，保留原有测试主体。沿 reverse-engineer-anything 检查当前加载脚本，脚本身份见 source-manifest.json。REA 对 Elements/clo 的分析见 rea-summary.json；追加 subject-feature-loader 分析达到60秒超时（保留证据 ev_14297bc8b682d7cdf4a390f1f88551eb18dc917f0a1193bf68dc8effdec40f00），上传结论使用已获取脚本的人工静态检查，未将超时报告为解析成功。

agent-browser 原独立会话找不到运行中的 Chrome，doctor 显示安装正常；未迁移登录 Cookie，改用已有登录 Chrome 的浏览器工具进行真实验证。

## 逐项差异

| 项目 | 即梦证据 | 本次处理/边界 |
|---|---|---|
| 分页/布局 | 既有 Elements 源码和真实页证明等高 Grid、底部加载，不是瀑布流；单主体账号无法实测第二批 | 保留每批20条前端累积和后端page合同，无页码、每页选择器或结束文案 |
| 共享顶栏 | 筛选、时间、排序与账号处于一行 | 沿现有双槽，不新造页内工具条 |
| 收藏空态 | 收藏叠加无匹配搜索，真实页显示“当前已开启筛选，部分内容可能被隐藏”；Elements条件优先onlyFavorited | 收藏优先于普通搜索；移除额外指导 |
| 日期空态 | 2026-10-29单日无结果仅新建卡 | 去掉“此时间范围内没有主体”和指导文字 |
| 收藏激活 | 标签变“收藏”，原位清空 | 复用日期触发器结构；清空只改favorite，保留日期和排序；手机/键盘入口可达 |
| 本地添加 | subject-feature-loader 的 eu 调 et.y({multiple:true})，ec 分派主图/素材批次；ef添加补充数组；fSn 使用showOpenFilePicker及input fallback | 直接原生选文件；主图首张+其余补充、补充批量、音色/视频单选；取消不弹空表单 |
| 上传恢复 | 本地后端既有权限及幂等合同，不从竞品外观推断 | 保留显式权限确认；各文件稳定key，成功项保留，失败停止，未知恢复同一文件/key；列表限高可滚动 |
| 名称计数 | 真实输入10个😀=20/20，DOM maxlength20 | 已一致，保持UTF-16计数，不重写姓名规则 |
| 描述长度 | 即梦textarea DOM无maxlength | 本地2000服务端上限仍为项目约束；未证明竞品服务端无限，因此未盲删校验 |
| 下载格式 | Elements先水印设置、后downloadSubjects，底层打包未解析 | 现有下载保留；未声称ZIP格式或水印全对齐 |
| 视频/音色上限 | 此账号样本不足 | 不凭空补duration或数量规则，保留待证项 |
| 手机稿 | 旧稿文本新建按钮在后 | 原位修正首位新建、等高单列，补390px空态/上传状态 |
| TOS/豆包 | 用户后续填.env | 无新增.local/YML，未伪造配置或供应商成功；不阻止其他实现与测试 |

## 编译阻断

画布曾引用缺失的DirectorRecovery导出，当前并行API生成已恢复；未手改生成文件。另一个实际TS错误来自并行未跟踪 `generation/agent-workspace.tsx`：可选链比较不能保证current非空，已加显式current守卫，保留revision比较。精确补丁见canvas-local-compile.patch；未将整个并行未跟踪模块纳入主体提交。当前工作区构建曾通过，且真实画布已展示4个节点、已保存/已连接，见local-canvas-restored截图。

## 设计稿

沿既有 Figma 主体区编辑，见[设计核验](figma-summary.md)及readback；不把原型状态视为真实上传或供应商回执。

## 验证记录

最终命令结果、浏览器状态和提交范围在 validation.md 记录。真实上传产物使用合成网格与几何测试图，不包含用户私人素材；当前存储链路上传成功不等于后续TOS配置已验收。
